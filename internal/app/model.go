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
	"github.com/atotto/clipboard"
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
	details                               bool
	offset                                int
	cancel                                context.CancelFunc
	saving, dirty, quitting, help         bool
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
		m.table = ui.Table{Rows: v.result.Series, Labels: true}
		// Sorting table rows must not reorder the legend/visibility map.
		m.table.Rows = append([]query.Series(nil), m.table.Rows...)
		m.setGraphResult(v.result)
		m.summary = fmt.Sprintf("%d series · %s", len(m.table.Rows), m.requestLabel)
		m.notice = strings.Join(v.result.Notices, "; ")
	case copied:
		if v.err != nil {
			m.status = "Clipboard: " + v.err.Error()
		} else {
			m.status = "Copied"
		}
	case tea.PasteMsg:
		if m.help || m.details || m.quitting {
			return m, nil
		}
		if m.searching {
			m.searchText += v.Content
			m.searchHistory()
			return m, nil
		}
	case tea.KeyPressMsg:
		if m.quitting {
			return m, nil
		}
		switch v.String() {
		case "ctrl+c", "ctrl+q":
			m.dismiss()
			running := m.cancel != nil
			m.stop()
			if running && v.String() == "ctrl+c" {
				m.status = "Cancelled"
				return m, nil
			}
			m.quitting = true
			return m, nil
		case "f1":
			m.dismiss()
			m.help = !m.help
			m.details, m.offset = false, 0
			return m, nil
		case "f2":
			m.dismiss()
			m.details = !m.details
			m.help, m.offset = false, 0
			return m, nil
		}
		if m.help || m.details {
			switch v.String() {
			case "down", "j", "pgdown":
				m.offset = min(m.offset+1, max(0, len(m.overlay())-m.height+1))
			case "up", "k", "pgup":
				m.offset = max(0, m.offset-1)
			default:
				m.help, m.details = false, false
			}
			return m, nil
		}
		if cmd, handled := m.assistKey(v); handled {
			return m, cmd
		}
		if v.String() == "f4" {
			m.graphKey("f4")
			return m, nil
		}
		switch v.String() {
		case "ctrl+enter", "ctrl+g", "f5":
			m.dismiss()
			return m, m.run()
		case "ctrl+t":
			m.opt.Ranged = !m.opt.Ranged
			return m, nil
		case "ctrl+w":
			next := presets[0]
			for _, d := range presets {
				if d > m.opt.Window {
					next = d
					break
				}
			}
			m.opt.Window = next
			m.opt.Ranged = true
			return m, nil
		case "esc":
			m.dismiss()
			if m.editor.Focused() {
				m.editor.Blur()
				return m, nil
			}
			return m, m.editor.Focus()
		}
		if m.editor.Focused() {
			switch v.String() {
			case "alt+up", "ctrl+p":
				m.recall(1)
				return m, m.schedule(false)
			case "alt+down", "ctrl+n":
				m.recall(-1)
				return m, m.schedule(false)
			}
		} else {
			if m.graphKey(v.String()) {
				return m, nil
			}
			switch v.String() {
			case "up", "k":
				m.table.Move(-1)
			case "down", "j":
				m.table.Move(1)
			case "pgup":
				m.table.Move(-m.page())
			case "pgdown":
				m.table.Move(m.page())
			case "left":
				m.table.X = max(0, m.table.X-8)
			case "right":
				m.table.X += 8
			case "[":
				m.table.Sample(1)
			case "]":
				m.table.Sample(-1)
			case "l":
				m.table.Labels = !m.table.Labels
			case "n", "v":
				m.table.Sort(v.String() == "v")
			case "y", "Y":
				if len(m.table.Rows) > 0 {
					_, s, _ := m.table.Fields(m.table.Row)
					if v.String() == "Y" {
						s = m.table.Rows[m.table.Row].Labels
					}
					return m, func() tea.Msg { return copied{clipboard.WriteAll(s)} }
				}
			}
			return m, nil
		}
	}
	before, pos := m.editor.Value(), editor.Position(m.editor)
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	if m.editor.Focused() && (before != m.editor.Value() || pos != editor.Position(m.editor)) {
		return m, tea.Batch(cmd, m.schedule(false))
	}
	return m, cmd
}
func (m *Model) page() int {
	rows := m.height - min(m.editor.LineCount(), m.editor.Height()) - 6
	if m.notice != "" {
		rows--
	}
	if m.showDiagnostic() {
		rows--
	}
	return max(1, rows)
}
func (m *Model) showDiagnostic() bool {
	return m.diagnostic != "" && !(m.popup && m.context.Kind == "value")
}

var title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))

func (m *Model) View() tea.View {
	v := tea.NewView(m.screen())
	v.AltScreen = true
	return v
}
func (m *Model) screen() string {
	if m.width < 24 || m.height < 10 {
		return "Enlarge terminal (24×10)"
	}
	if m.help || m.details {
		lines := m.overlay()
		first := min(m.offset, max(0, len(lines)-m.height+1))
		return strings.Join(lines[first:min(len(lines), first+m.height-1)], "\n") + "\n↑↓ scroll · Esc close"
	}
	mode := "instant"
	if m.opt.Ranged {
		mode = "range " + m.opt.Window.String()
	}
	if !m.opt.End.IsZero() {
		mode += " @ " + m.opt.End.Format(time.RFC3339)
	}
	clip := func(s string) string { return ansi.Truncate(ui.Plain(s), m.width, "…") }
	section := func(name string, active bool) string {
		mark := "  "
		if active {
			mark = "› "
		}
		s := clip(mark + name + " ")
		s += strings.Repeat("─", max(0, m.width-ansi.StringWidth(s)))
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
	lines := []string{head.Render(clip(" PromTUI  │  " + m.opt.Connection.Name + "  ·  " + mode)), section("Query", m.editor.Focused())}
	rows, x, y := editor.View(m.editor, m.width)
	lines = append(lines, rows...)
	if m.showDiagnostic() {
		lines = append(lines, clip("PromQL: "+m.diagnostic))
	}
	name := "Results"
	if m.chart.enabled {
		name = "Graph · UTC"
	}
	if m.summary != "No query executed" {
		name += " · " + m.summary
	}
	lines = append(lines, section(name, !m.editor.Focused()))
	if m.notice != "" {
		lines = append(lines, clip("Notice: "+m.notice))
	}
	room := m.height - len(lines) - 2
	if room > 0 {
		switch {
		case m.summary == "No query executed":
			lines = append(lines, clip("  Run a query with Ctrl+Enter."))
		case len(m.table.Rows) == 0:
			lines = append(lines, clip("  No matching series."))
		case m.chart.enabled:
			lines = append(lines, m.legend())
			if m.chart.err != "" {
				lines = append(lines, clip("  "+m.chart.err))
			} else if !m.popup && !m.searching && m.graphPlace().cols == 0 {
				lines = append(lines, clip("  Enlarge graph or enable a series (Space)."))
			}
		default:
			lines = append(lines, strings.Split(m.table.View(m.width, room), "\n")...)
		}
	}
	for len(lines) < m.height-2 {
		lines = append(lines, "")
	}
	keys := "Ctrl+Enter Run   Ctrl+Space Complete   Ctrl+R History   Esc Results   ? Help"
	if !m.editor.Focused() {
		keys = "g Graph   ↑↓ Series   ←→ Pan   [ ] Samples   Esc Edit   ? Help"
	}
	if m.chart.enabled {
		keys = "F4 Table   Esc Focus   Ctrl+W Range   ? Help"
		if !m.editor.Focused() {
			keys = "g Table   ↑↓ Series   Space Hide/show   Esc Edit   ? Help"
		}
	}
	if m.popup {
		keys = "Tab Accept   ↑↓ Select   Esc Dismiss"
	}
	if m.searching {
		keys = "Enter Restore   ↑↓ Select   Esc Cancel"
	}
	lines = append(lines, clip(" "+status), lipgloss.NewStyle().Width(m.width).Reverse(true).Render(clip(" "+keys)))
	base := strings.Join(lines, "\n")
	if m.popup || m.searching {
		return lipgloss.NewCompositor(lipgloss.NewLayer(base), m.menu(x, 3+y)).Render()
	}
	return base
}

func (m *Model) overlay() []string {
	text := "PromTUI · Keys\nCtrl+Enter  execute (fallback Ctrl+G / F5)\nEnter  newline\nCtrl+Space  suggestions · ↑↓ select · Tab accept\nCtrl+R  fuzzy history · Enter restore\nAlt+Up/Down  older/newer query\nEsc  dismiss popup / switch focus\n?  help (F1 anywhere)\nCtrl+T  instant/range · Ctrl+W next range\nResults: ↑↓ PgUp/Dn · ←→ pan · l labels\n[ / ] older/newer sample · n/v sort name/value\ny/Y copy value/labels\nF2 full status and warnings\nCtrl+C cancel or quit · Ctrl+Q quit"
	if m.details {
		text = strings.Join([]string{"Request details", ui.Plain(m.status), ui.Plain(m.diagnostic), ui.Plain(m.historyError), "Last result: " + ui.Plain(m.summary), ui.Plain(m.notice)}, "\n")
	}
	if !m.details {
		text += "\nF4 table/graph · g in results · graph ↑↓ select · Space hide/show"
	}
	return strings.Split(ansi.Hardwrap(text, max(1, m.width), false), "\n")
}
