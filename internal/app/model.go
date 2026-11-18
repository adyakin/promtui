package app

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/api"
	"promtui/internal/config"
	"promtui/internal/history"
	"promtui/internal/query"
	"promtui/internal/ui"
)

type Options struct {
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
func (m *Model) Init() tea.Cmd        { return textarea.Blink }
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
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(v.Width, v.Height)
	case saved:
		m.saving = false
		m.historyError = ""
		if v.err != nil {
			m.historyError = "History: " + v.err.Error()
		}
		if m.dirty {
			return m, m.save()
		}
		if m.quitting {
			return m, tea.Quit
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
		m.summary = fmt.Sprintf("%d series · %s", len(m.table.Rows), m.requestLabel)
		m.notice = strings.Join(v.result.Notices, "; ")
	case copied:
		if v.err != nil {
			m.status = "Clipboard: " + v.err.Error()
		} else {
			m.status = "Copied"
		}
	case tea.KeyMsg:
		if m.quitting {
			return m, nil
		}
		switch v.String() {
		case "ctrl+c", "ctrl+q":
			running := m.cancel != nil
			m.stop()
			if running && v.String() == "ctrl+c" {
				m.status = "Cancelled"
				return m, nil
			}
			if m.saving {
				m.quitting = true
				return m, nil
			}
			return m, tea.Quit
		case "f1":
			m.help = !m.help
			m.details, m.offset = false, 0
			return m, nil
		case "f2":
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
		switch v.String() {
		case "ctrl+g", "f5":
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
		case "esc", "tab":
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
				return m, nil
			case "alt+down", "ctrl+n":
				m.recall(-1)
				return m, nil
			}
		} else {
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
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	return m, cmd
}
func (m *Model) page() int { return max(2, m.height-m.editor.Height()-5) }

var title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))

func (m *Model) View() string {
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
	focus := "editor"
	if !m.editor.Focused() {
		focus = "results"
	}
	clip := func(s string) string { return ansi.Truncate(ui.Plain(s), m.width, "…") }
	status := m.status
	if m.historyError != "" {
		status = m.historyError + " · " + status
	}
	lines := []string{title.Render(clip("PromTUI · " + m.opt.Connection.Name + " · " + mode + " · " + focus)), m.editor.View(), clip("Last result · " + m.summary)}
	if m.notice != "" {
		lines = append(lines, clip("Notice: "+m.notice))
	}
	lines = append(lines, m.table.View(m.width, m.page()), clip(status), clip("Ctrl+G run · Ctrl+T mode · Tab focus · F1 keys · Ctrl+Q quit"))
	return strings.Join(lines, "\n")
}

func (m *Model) overlay() []string {
	text := "PromTUI · Keys\nCtrl+G / F5  execute\nEnter  newline\nCtrl+T  instant/range\nCtrl+W  next range\nCtrl+P/N  recall older/newer\nTab / Esc  editor/results\nResults: ↑↓ PgUp/Dn · ←→ pan · l labels\n[ / ] older/newer sample · n/v sort name/value\ny/Y copy value/labels\nF2 full status and warnings\nCtrl+C cancel or quit · Ctrl+Q quit"
	if m.details {
		text = strings.Join([]string{"Request details", ui.Plain(m.status), ui.Plain(m.historyError), "Last result: " + ui.Plain(m.summary), ui.Plain(m.notice)}, "\n")
	}
	return strings.Split(ansi.Hardwrap(text, max(1, m.width), false), "\n")
}
