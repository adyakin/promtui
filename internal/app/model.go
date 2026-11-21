package app

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/api"
	"promtui/internal/config"
	"promtui/internal/editor"
	"promtui/internal/history"
	"promtui/internal/kitty"
	"promtui/internal/programstatus"
	"promtui/internal/query"
	"promtui/internal/ui"
)

type Options struct {
	Graphics              bool
	ProbeGraphics         bool
	ProgramStatus         bool
	Connection            config.Connection
	Connections           []config.Connection
	Timeout, Window, Step time.Duration
	End                   time.Time
	Ranged                bool
	Limit, HistoryLimit   int
	HistoryPath           string
	SavedQueriesPath      string
	SilenceCreator        string
}
type completed struct {
	id      int
	result  query.Result
	elapsed time.Duration
	err     error
}
type saved struct{ err error }
type copied struct{ err error }
type Model struct {
	shellState
	promQLState
	alerts        alertsState
	silences      silencesState
	targets       targetsState
	routing       routingState
	httpClients   map[bool]*http.Client
	programStatus terminalStatus
}

type promQLState struct {
	chart graphState
	assist
	opt                                   Options
	editor                                textarea.Model
	table                                 ui.Table
	resultFilter                          resultFilterState
	entries                               []history.Entry
	cursor                                int
	draft                                 string
	status, summary, notice, historyError string
	queryPhase                            queryPhase
	queryError                            queryError
	id                                    int
	requestLabel                          string
	cancel                                context.CancelFunc
	saving, dirty                         bool
	panels
	savedQueries    savedQueriesState
	savedFeedback   string
	savedFeedbackID int
}

func New(o Options, entries []history.Entry) *Model {
	e := textarea.New()
	e.Placeholder = "Enter a PromQL expression, e.g. up"
	e.Prompt = "│ "
	e.CharLimit = 65536
	e.MaxWidth = 0
	m := &Model{alerts: newAlertsState(o.Connection.Alerts), targets: newTargetsState(o.Connection), silences: newSilencesState(), routing: newRoutingState(), promQLState: promQLState{
		opt: o, entries: entries, cursor: -1, editor: e, resultFilter: newResultFilter(), status: "Ready", summary: "No query executed",
	}}
	m.resize(80, 24)
	return m
}
func (m *Model) resize(w, h int) {
	m.width, m.height = w, h
	m.editor.SetWidth(max(8, w-2))
	m.editor.SetHeight(max(1, min(6, h/4, h-9)))
	m.alerts.input.SetWidth(max(1, w-13))
	m.targets.input.SetWidth(max(1, w-13))
	m.resultFilter.input.SetWidth(max(1, w-13))
	m.sizeSavedInput()
	m.sizeSilenceInputs()
	m.sizeRoutingInput()
}
func (m *Model) Init() tea.Cmd {
	if m.opt.ProbeGraphics {
		return tea.Raw(kitty.Probe())
	}
	return nil
}
func (m *Model) HistoryError() string { return m.historyError }
func (m *Model) save() tea.Cmd {
	if m.opt.HistoryLimit == 0 {
		return nil
	}
	if m.saving {
		m.dirty = true
		return nil
	}
	m.saving, m.dirty = true, false
	entries, path := m.entries, m.opt.HistoryPath
	return func() tea.Msg { return saved{history.Save(path, entries)} }
}
func (m *Model) stop() bool {
	if m.cancel == nil {
		return false
	}
	m.cancel()
	m.cancel = nil
	m.id++
	return true
}

func (m *Model) setQueryMode(ranged bool) {
	m.dismiss()
	m.opt.Ranged = ranged
	if ranged && m.opt.Window <= 0 {
		m.opt.Window = time.Hour
	}
}

func (m *Model) runMode(ranged bool) tea.Cmd {
	m.setQueryMode(ranged)
	if m.queryPhase == queryFailed {
		// Let users switch the view without discarding the error. Run and
		// confirming a new time range remain explicit executions.
		return nil
	}
	return m.run()
}

func (m *Model) clearQueryResults() {
	m.table = ui.Table{}
	m.resultFilter.input.Blur()
	m.resultFilter.input.Reset()
	m.resultFilter.anchor = ui.Table{}
	m.resultFilter.allRows, m.resultFilter.matches, m.resultFilter.before = nil, nil, ""
	m.setGraphResult(query.Result{})
	m.notice = ""
	m.queryError = queryError{}
	m.labelText, m.labelFields = "", nil
	m.seriesRow, m.isolatedIndex = 0, 0
	m.filtered, m.filtering = nil, false
	if m.panel == labelKeys || m.panel == seriesKeys || m.panel == queryErrorKeys {
		m.panel, m.offset = 0, 0
	}
}

func (m *Model) run() tea.Cmd {
	expr := strings.TrimSpace(m.editor.Value())
	if expr == "" {
		m.status = "Enter a query first"
		return nil
	}
	m.stop()
	m.clearQueryResults()
	m.id++
	id, o := m.id, m.opt
	now := time.Now()
	end := o.End
	if end.IsZero() {
		end = now
	}
	r := query.Request{Expression: expr, End: end, Step: o.Step, Limit: o.Limit}
	if o.Ranged {
		r.Window = o.Window
	}
	mode := "instant"
	if o.Ranged {
		mode = "range " + o.Window.String()
	}
	m.requestLabel = mode + " · " + ui.Plain(expr)
	ctx, cancel := context.WithTimeout(context.Background(), o.Timeout)
	m.cancel = cancel
	m.queryPhase = queryRunning
	m.status, m.summary = executingQuery, executingQuery
	m.reportProgramStatus(programstatus.Working, "Executing PromQL query")
	m.entries = history.Add(m.entries, history.Entry{At: now.UTC(), Connection: o.Connection.Name, Query: expr}, o.HistoryLimit)
	m.cursor = -1
	client := api.Client{Base: o.Connection.URL, HTTP: m.connectionHTTP()}
	return tea.Batch(m.save(), func() tea.Msg {
		defer cancel()
		res, err := query.Execute(ctx, client, r)
		return completed{id, res, time.Since(now), err}
	})
}
func (m *Model) recall(delta int) {
	i := m.cursor + delta
	if i < -1 || i >= len(m.entries) {
		return
	}
	if i == -1 {
		m.editor.SetValue(m.draft)
	} else {
		if m.cursor == -1 {
			m.draft = m.editor.Value()
		}
		m.editor.SetValue(m.entries[i].Query)
	}
	m.cursor = i
}

var presets = []time.Duration{15 * time.Minute, time.Hour, 6 * time.Hour, 24 * time.Hour, 7 * 24 * time.Hour}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.update(msg)
	draw := m.syncGraph(msg)
	raw := m.flushProgramStatus(msg)
	if m.quitting && !m.saving && !m.programStatus.sending && !m.chart.sending && m.chart.displayed.cols == 0 {
		return model, tea.Quit
	}
	if raw != nil {
		if next := tea.Batch(cmd, draw); next != nil {
			return model, tea.Sequence(raw, next)
		}
		return model, raw
	}
	return model, tea.Batch(cmd, draw)
}
func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case routingLoaded:
		m.routing.loaded(v)
		return m, nil
	case silencesLoaded:
		return m, m.onSilencesLoaded(v, time.Now())
	case silencesTick:
		return m, m.onSilencesTick(time.Time(v))
	case silenceDetailLoaded:
		return m, m.onSilenceDetailLoaded(v)
	case silenceMatchesLoaded:
		m.onSilenceMatchesLoaded(v)
		return m, nil
	case silenceClipboard:
		if m.pickingConnection || m.quitting {
			return m, nil
		}
		return m, m.editSilenceInput(v)
	case silenceWritten:
		return m, m.onSilenceWritten(v, time.Now())
	case targetsCompleted:
		return m, m.onTargetsCompleted(v, time.Now())
	case targetsTick:
		return m, m.onTargetsTick(time.Time(v))
	case targetLabelsCopied:
		if m.targets.dialog == targetDetails && m.targets.dialogID == v.dialogID {
			m.targets.copyStatus = "Copied labels"
			if v.err != nil {
				m.targets.copyStatus = "Could not copy labels"
			}
		}
		return m, nil
	case alertsCompleted:
		return m, m.onAlertsCompleted(v, time.Now())
	case alertsTick:
		return m, m.onAlertsTick(time.Time(v))
	case alertLabelsCopied:
		m.alerts.detailStatus = "Copied labels"
		if v.err != nil {
			m.alerts.detailStatus = "Could not copy labels"
		}
		return m, nil
	case uv.KittyGraphicsEvent:
		if m.opt.ProbeGraphics && v.Options.ID == kitty.ProbeID {
			m.opt.ProbeGraphics = false
			if string(v.Payload) == "OK" {
				m.opt.Graphics = true
				m.chart.cw, m.chart.ch = kitty.CellSize()
				m.chart.enabled = !m.chart.result.Start.IsZero() && len(m.chart.result.Series) > 0
			}
		}
		return m, nil
	case graphFrame, tea.RawMsg:
		return m, nil
	case savedFeedbackExpired:
		if int(v) == m.savedFeedbackID {
			m.savedFeedback = ""
		}
		return m, nil
	case analysisMsg:
		return m, m.analyzed(v)
	case suggestionsMsg:
		m.suggested(v)
		return m, nil
	case tea.WindowSizeMsg:
		m.resize(v.Width, v.Height)
		if m.opt.Graphics {
			m.chart.cw, m.chart.ch = kitty.CellSize()
		}
	case saved:
		m.saving = false
		m.historyError = ""
		if v.err != nil {
			m.historyError = "History: " + v.err.Error()
		}
		if m.dirty {
			return m, m.save()
		}
	case completed:
		if m.id != v.id {
			return m, nil
		}
		m.cancel = nil
		duration := v.elapsed.Round(time.Millisecond)
		if v.err != nil {
			m.clearQueryResults()
			m.queryPhase = queryFailed
			m.queryError = newQueryError(v.err, v.elapsed)
			m.status, m.summary = m.queryError.heading(), "Query failed"
			m.reportProgramStatus(programstatus.Error, "Query failed")
			return m, nil
		}
		m.queryPhase = querySucceeded
		m.queryError = queryError{}
		m.status = fmt.Sprintf("Done in %s", duration)
		m.reportProgramStatus(programstatus.Done, "Query completed")
		// Sorting table rows must not reorder the legend/visibility map.
		m.resultFilter.allRows = append([]query.Series(nil), v.result.Series...)
		m.table = ui.Table{Rows: m.resultFilter.allRows}
		m.setGraphResult(v.result)
		m.summary = "No data"
		if len(m.table.Rows) > 0 {
			m.summary = fmt.Sprintf("%d series · %s", len(m.table.Rows), m.requestLabel)
		}
		m.notice = strings.Join(v.result.Notices, "; ")
		if m.panel == seriesKeys {
			m.refreshSeries()
		}
	case copyQueryError:
		return m, copyText(string(v))
	case copied:
		if v.err != nil {
			m.status = "Clipboard: " + v.err.Error()
		} else {
			m.status = "Copied"
		}
	case tea.PasteMsg:
		if m.pickingConnection {
			return m, nil
		}
		if !m.quitting && (m.activeTab == silencesTab || m.silences.editor != nil) {
			return m, m.editSilenceInput(v)
		}
		if !m.quitting && m.activeTab == alertsTab && m.alerts.input.Focused() {
			return m, m.alerts.edit(v)
		}
		if !m.quitting && m.activeTab == targetsTab && m.targets.input.Focused() {
			return m, m.targets.edit(v)
		}
		if !m.quitting && m.activeTab == routingTab && m.routing.search.Focused() {
			return m, m.editRoutingSearch(v)
		}
		if m.quitting || m.activeTab != promQLTab {
			return m, nil
		} else if m.panel&savedQueryKeys != 0 {
			return m, m.editSavedInput(v)
		} else if m.resultFilter.input.Focused() {
			return m, m.editResultFilter(v)
		} else if scope := m.scope(); scope == historyKeys || scope == filterKeys {
			m.editFilter(v.Content, false)
			return m, nil
		} else if scope != editKeys && scope != completionKeys {
			return m, nil
		}
	case tea.KeyPressMsg:
		if m.quitting {
			return m, nil
		}
		if cmd, handled := m.shellKey(v); handled {
			return m, cmd
		}
		if cmd, handled := m.key(v); handled {
			return m, cmd
		}
	}
	if m.activeTab != promQLTab {
		return m, nil
	}
	before, pos := m.editor.Value(), editor.Position(m.editor)
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	if m.editor.Focused() && (before != m.editor.Value() || pos != editor.Position(m.editor)) {
		if before != m.editor.Value() {
			m.cursor = -1
		}
		return m, tea.Batch(cmd, m.schedule(false))
	}
	return m, cmd
}

type workspace struct {
	resultY, bodyY, bottomY int
	diagnostic, notice      bool
	filter                  bool
}

func (m *Model) workspace() workspace {
	editorRows := min(m.editor.LineCount(), m.editor.Height())
	// Reserve one body row for Results, in addition to the two frames,
	// their gutter, the header, status and footer.
	spare := m.height - 9 - editorRows
	w := workspace{resultY: 4 + editorRows, bottomY: m.height - 3}
	w.filter = m.resultFilter.input.Focused() || m.resultFilter.input.Value() != ""
	if w.filter {
		spare--
	}
	if spare > 0 && m.showDiagnostic() {
		w.diagnostic = true
		w.resultY++
		spare--
	}
	w.bodyY = w.resultY + 1
	if spare > 0 && m.notice != "" {
		w.notice = true
		w.bodyY++
	}
	if w.filter {
		w.bodyY++
	}
	return w
}

func (m *Model) page() int {
	w := m.workspace()
	// Match the table viewport, excluding its column-heading row.
	room := w.bottomY - w.bodyY
	return max(1, room-1)
}
func (m *Model) showDiagnostic() bool {
	return m.diagnostic != "" && !(m.popup && m.context.Kind == "value")
}

var title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))

// A frame occupies exactly width cells, including its two side borders.
func frame(name, meta string, rows []string, width int, active bool) string {
	return renderFrame(name, meta, rows, width, active, lipgloss.NewStyle())
}

func renderFrame(name, meta string, rows []string, width int, active bool, surface lipgloss.Style) string {
	border := surface.Foreground(lipgloss.Color("8"))
	heading := border
	if active {
		border = surface.Foreground(lipgloss.Color("6"))
		heading = border.Bold(true)
		name = "› " + name
	}
	label := ansi.Truncate(" "+name, width-4, "…")
	if ansi.StringWidth(label) < width-4 {
		label += " "
	}
	rule := width - 3 - ansi.StringWidth(label)
	top := border.Render("╭─") + heading.Render(label)
	if meta != "" && ansi.StringWidth(meta)+3 <= rule {
		meta = " " + meta + " "
		rule -= ansi.StringWidth(meta)
	} else {
		meta = ""
	}
	top += border.Render(strings.Repeat("─", rule)) + heading.Render(meta) + border.Render("╮")
	lines := []string{top}
	edge := border.Render("│")
	for _, row := range rows {
		row = ansi.Truncate(row, width-2, "…")
		lines = append(lines, edge+row+surface.Render(strings.Repeat(" ", width-2-ansi.StringWidth(row)))+edge)
	}
	lines = append(lines, border.Render("╰"+strings.Repeat("─", width-2)+"╯"))
	return strings.Join(lines, "\n")
}

const minWidth, minHeight = 24, 10

func (m *Model) View() tea.View {
	content, cursor := m.renderScreen()
	v := tea.NewView(content)
	v.AltScreen = true
	v.Cursor = cursor
	return v
}
func (m *Model) screen() string {
	content, _ := m.renderScreen()
	return content
}

// Bubble Tea v2.0.6 encodes Shape -1 with Blink false as style 0, the
// same value it uses for a hidden cursor. Switching focus or exiting
// therefore emits no cursor-style escape, preserving the terminal's setting.
func nativeCursor(x, y int) *tea.Cursor {
	return &tea.Cursor{Position: tea.Position{X: x, Y: y}, Shape: -1}
}

func (m *Model) promQLScreen() (string, *tea.Cursor) {
	if m.panel != 0 && m.panel&dialogKeys == 0 {
		return m.panelView(), nil
	}
	w := m.workspace()
	clip := func(s string) string { return ansi.Truncate(ui.Plain(s), m.width, "…") }
	inner := m.width - 2
	status := m.status
	if m.savedFeedback != "" {
		status = m.savedFeedback + " · " + status
	}
	if m.historyError != "" {
		status = m.historyError + " · " + status
	}
	queryRows, x, y := editor.View(m.editor, inner)
	if w.diagnostic {
		queryRows = append(queryRows, " PromQL: "+ui.Plain(m.diagnostic))
	}
	name, meta := "Results · Table", ""
	if m.graphView() {
		name = "Results · Graph"
	}
	count := len(m.table.Rows)
	if m.chart.enabled {
		count = m.graphSeriesCount()
	}
	if m.queryPhase == querySucceeded {
		meta = fmt.Sprintf("%d series", count)
	}
	var resultRows []string
	if w.notice {
		resultRows = append(resultRows, " Notice: "+ui.Plain(m.notice))
	}
	var filterCursor *tea.Cursor
	if w.filter {
		line := "  Filter: " + ui.Plain(m.resultFilter.input.Value())
		if m.resultFilter.input.Focused() {
			line = m.resultFilter.input.View()
			if c := m.resultFilter.input.Cursor(); c != nil && !m.quitting {
				filterCursor = nativeCursor(min(m.width-2, 1+c.X), w.bodyY-1)
			}
		}
		resultRows = append(resultRows, line)
	}
	room := w.bottomY - w.bodyY
	body := make([]string, room)
	switch {
	case room == 0:
	case m.queryPhase == queryRunning:
		body[0] = "  " + executingQuery
	case m.queryPhase == queryCancelled:
		body[0] = "  Query cancelled"
	case m.queryPhase == queryFailed:
		m.queryError.renderInline(body, inner-2, m.editor.Focused())
	case m.queryPhase == queryReady:
		if !m.editor.Focused() && m.editor.Value() == "" {
			body[0] = "  Press i to edit query."
		} else {
			body[0] = "  Run with Ctrl+Enter."
		}
	case count == 0 && len(m.resultFilter.allRows) > 0:
		body[0] = "  No series matching the current filter"
	case count == 0:
		body[0] = "  No data"
	case m.chart.enabled && m.chart.err != "":
		if room == 1 {
			body[0] = "  Graph error: " + ui.Plain(m.chart.err)
		} else {
			renderPaneError(body, paneErrorRows(inner-2, "Graph rendering failed", m.chart.err), 0)
		}
	case m.chart.enabled:
		if room > 1 {
			switch {
			case !m.hasVisibleSeries():
				body[0] = "  No visible series. Use the series selector."
			case m.activeTab == promQLTab && m.panel == 0 && !m.popup && !m.searching && m.graphPlace().cols == 0:
				body[0] = "  Enlarge terminal to see the graph."
			}
		}
		body[room-1] = m.legend()
	default:
		copy(body, strings.Split(m.table.View(inner, room), "\n"))
	}
	base := strings.Join([]string{m.header(promQLTab, m.queryStatus()...),
		frame("Query", runHint, queryRows, m.width, m.scope() == editKeys), "",
		frame(name, meta, append(resultRows, body...), m.width, m.scope()&resultKeys != 0 || m.resultFilter.input.Focused()),
		clip(" " + status), m.footer()}, "\n")
	if m.popup || m.searching {
		return lipgloss.NewCompositor(lipgloss.NewLayer(base), m.menu(1+x, 3+y)).Render(), nil
	}
	if !m.pickingConnection && m.panel&savedQueryKeys != 0 {
		dialog, cursor := m.savedQueryLayer()
		return lipgloss.NewCompositor(lipgloss.NewLayer(base), dialog).Render(), cursor
	}
	if !m.pickingConnection && m.panel&dialogKeys != 0 {
		if dialog := m.dialogLayer(w); dialog != nil {
			return lipgloss.NewCompositor(lipgloss.NewLayer(base), dialog).Render(), nil
		}
	}
	if m.scope() == editKeys && !m.quitting {
		return base, nativeCursor(1+x, 2+y) // left border, then header and top border
	}
	return base, filterCursor
}
