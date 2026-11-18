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
	"promtui/internal/query"
	"promtui/internal/ui"
)

type Options struct {
	Graphics              bool
	ProbeGraphics         bool
	Connection            config.Connection
	Timeout, Window, Step time.Duration
	End                   time.Time
	Ranged                bool
	Limit, HistoryLimit   int
	HistoryPath           string
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
	chart graphState
	assist
	opt                                   Options
	editor                                textarea.Model
	table                                 ui.Table
	entries                               []history.Entry
	cursor                                int
	draft                                 string
	width, height                         int
	status, summary, notice, historyError string
	id                                    int
	requestLabel                          string
	cancel                                context.CancelFunc
	saving, dirty, quitting               bool
	panels
}

func New(o Options, entries []history.Entry) *Model {
	e := textarea.New()
	e.Placeholder = "Enter a PromQL expression, e.g. up"
	e.Prompt = "│ "
	e.CharLimit = 65536
	e.MaxWidth = 0
	e.Focus()
	m := &Model{opt: o, entries: entries, cursor: -1, editor: e, status: "Ready", summary: "No query executed"}
	m.resize(80, 24)
	return m
}
func (m *Model) resize(w, h int) {
	m.width, m.height = w, h
	m.editor.SetWidth(max(8, w))
	m.editor.SetHeight(max(1, min(6, h/4)))
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
func (m *Model) stop() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
		m.id++
	}
}
func (m *Model) run() tea.Cmd {
	expr := strings.TrimSpace(m.editor.Value())
	if expr == "" {
		m.status = "Enter a query first"
		return nil
	}
	m.stop()
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
	m.status = "Running…"
	m.entries = history.Add(m.entries, history.Entry{At: now.UTC(), Connection: o.Connection.Name, Query: expr}, o.HistoryLimit)
	m.cursor = -1
	return tea.Batch(m.save(), func() tea.Msg {
		defer cancel()
		res, err := query.Execute(ctx, api.Client{Base: o.Connection.URL, HTTP: http.DefaultClient}, r)
		return completed{id, res, time.Since(now), err}
	})
}
func (m *Model) recall(delta int) {
	for i := m.cursor + delta; i < len(m.entries) && i >= -1; i += delta {
		if i == -1 {
			m.cursor = i
			m.editor.SetValue(m.draft)
			return
		}
		if m.entries[i].Connection != m.opt.Connection.Name {
			continue
		}
		if m.cursor == -1 {
			m.draft = m.editor.Value()
		}
		m.cursor = i
		m.editor.SetValue(m.entries[i].Query)
		return
	}
}

var presets = []time.Duration{15 * time.Minute, time.Hour, 6 * time.Hour, 24 * time.Hour, 7 * 24 * time.Hour}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.update(msg)
	draw := m.syncGraph(msg)
	if m.quitting && !m.saving && !m.chart.sending && m.chart.displayed.cols == 0 {
		return model, tea.Quit
	}
	return model, tea.Batch(cmd, draw)
}
func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case uv.KittyGraphicsEvent:
		if m.opt.ProbeGraphics && v.Options.ID == kitty.ProbeID {
			m.opt.ProbeGraphics = false
			if string(v.Payload) == "OK" {
				m.opt.Graphics = true
				m.chart.cw, m.chart.ch = kitty.CellSize()
				m.chart.enabled = !m.chart.result.Start.IsZero() && !m.chart.preferTable
			}
		}
		return m, nil
	case graphFrame, tea.RawMsg:
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
			m.status = fmt.Sprintf("Error (%s): %v", duration, v.err)
			return m, nil
		}
		m.status = fmt.Sprintf("Done in %s", duration)
		m.table = ui.Table{Rows: v.result.Series}
		// Sorting table rows must not reorder the legend/visibility map.
		m.table.Rows = append([]query.Series(nil), m.table.Rows...)
		m.setGraphResult(v.result)
		m.summary = fmt.Sprintf("%d series · %s", len(m.table.Rows), m.requestLabel)
		m.notice = strings.Join(v.result.Notices, "; ")
		if m.panel == seriesKeys {
			m.refreshSeries()
		}
	case copied:
		if v.err != nil {
			m.status = "Clipboard: " + v.err.Error()
		} else {
			m.status = "Copied"
		}
	case tea.PasteMsg:
		if scope := m.scope(); scope == historyKeys || scope == filterKeys {
			m.editFilter(v.Content, false)
			return m, nil
		} else if m.quitting || scope != editKeys && scope != completionKeys {
			return m, nil
		}
	case tea.KeyPressMsg:
		if m.quitting {
			return m, nil
		}
		if cmd, handled := m.key(v); handled {
			return m, cmd
		}
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
func (m *Model) resultTop() int {
	rows := 3 + min(m.editor.LineCount(), m.editor.Height())
	if m.notice != "" {
		rows++
	}
	if m.showDiagnostic() {
		rows++
	}
	return rows
}
func (m *Model) page() int {
	return max(1, m.height-m.resultTop()-3)
}
func (m *Model) showDiagnostic() bool {
	return m.diagnostic != "" && !(m.popup && m.context.Kind == "value")
}

var title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))

const minWidth, minHeight = 24, 10

func (m *Model) View() tea.View {
	v := tea.NewView(m.screen())
	v.AltScreen = true
	return v
}
func (m *Model) screen() string {
	if m.width < minWidth || m.height < minHeight {
		lines := make([]string, max(1, m.height))
		lines[0] = ansi.Truncate("Enlarge terminal (24×10)", m.width, "…")
		if len(lines) > 1 {
			lines[len(lines)-1] = m.footer()
		}
		return strings.Join(lines, "\n")
	}
	if m.panel != 0 {
		return m.panelView()
	}
	mode := "Instant"
	if m.opt.Ranged {
		mode = "Range · " + windowLabel(m.opt.Window)
	}
	mode += " · UTC"
	if !m.opt.End.IsZero() {
		end := " │ " + m.opt.End.UTC().Format("2006-01-02 15:04")
		if ansi.StringWidth(" PromTUI  "+mode+end) <= m.width {
			mode += end
		}
	}
	if m.opt.Ranged && ansi.StringWidth(" PromTUI  "+mode) > m.width {
		mode = "Range · " + strings.TrimPrefix(windowLabel(m.opt.Window), "Last ") + " UTC"
	}
	clip := func(s string) string { return ansi.Truncate(ui.Plain(s), m.width, "…") }
	section := func(name, meta string, active bool) string {
		mark, rule := "  ", "─"
		if active {
			mark, rule = "› ", "━"
		}
		s := clip(mark + name + " ")
		if meta != "" && ansi.StringWidth(s)+ansi.StringWidth(meta)+3 <= m.width {
			s += strings.Repeat(rule, m.width-ansi.StringWidth(s)-ansi.StringWidth(meta)-2) + " " + meta + " "
		} else {
			s += strings.Repeat(rule, max(0, m.width-ansi.StringWidth(s)))
		}
		if active {
			return title.Render(s)
		}
		return s
	}
	status := m.status
	if m.historyError != "" {
		status = m.historyError + " · " + status
	}
	head := lipgloss.NewStyle().Bold(true).Reverse(true).Width(m.width)
	left := " PromTUI"
	if m.opt.Connection.Name != "" {
		left += " │ " + ui.Plain(m.opt.Connection.Name)
	}
	gap := m.width - ansi.StringWidth(left) - ansi.StringWidth(mode)
	header := left + strings.Repeat(" ", max(2, gap-1)) + mode
	if gap < 2 {
		header = ansi.Truncate(left, max(1, m.width-ansi.StringWidth(mode)-2), "…") + "  " + mode
	}
	lines := []string{head.Render(clip(header)), section("Query", "F5 Run", m.editor.Focused() && !m.popup && !m.searching)}
	rows, x, y := editor.View(m.editor, m.width)
	lines = append(lines, rows...)
	if m.showDiagnostic() {
		lines = append(lines, clip("PromQL: "+m.diagnostic))
	}
	name, meta := "Results · Table", ""
	if m.chart.enabled {
		name = "Results · Graph"
	}
	if m.summary != "No query executed" {
		meta = fmt.Sprintf("%d series", len(m.table.Rows))
	}
	lines = append(lines, section(name, meta, !m.editor.Focused()))
	if m.notice != "" {
		lines = append(lines, clip("Notice: "+m.notice))
	}
	room := m.height - len(lines) - 2
	if room > 0 {
		switch {
		case m.summary == "No query executed":
			lines = append(lines, clip("  Run a query with F5."))
		case len(m.table.Rows) == 0:
			lines = append(lines, clip("  No matching series."))
		case m.chart.enabled:
			if m.chart.err != "" && len(lines) < m.height-3 {
				lines = append(lines, clip("  "+m.chart.err))
			} else if !m.popup && !m.searching && len(lines) < m.height-3 {
				switch {
				case !m.hasVisibleSeries():
					lines = append(lines, clip("  No visible series. Use the series selector."))
				case m.graphPlace().cols == 0:
					lines = append(lines, clip("  Enlarge terminal to see the graph."))
				}
			}
			for len(lines) < m.height-3 {
				lines = append(lines, "")
			}
			lines = append(lines, m.legend())
		default:
			lines = append(lines, strings.Split(m.table.View(m.width, room), "\n")...)
		}
	}
	for len(lines) < m.height-2 {
		lines = append(lines, "")
	}
	lines = append(lines, clip(" "+status), m.footer())
	base := strings.Join(lines, "\n")
	if m.popup || m.searching {
		return lipgloss.NewCompositor(lipgloss.NewLayer(base), m.menu(x, 3+y)).Render()
	}
	return base
}
