package app

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"
	"promtui/internal/api"
	"promtui/internal/editor"
	"promtui/internal/ui"
)

type cacheKey struct{ kind, metric, label, prefix string }
type cached struct {
	until  time.Time
	values []string
}
type assist struct {
	generation                   int
	stopAnalysis                 context.CancelFunc
	context                      editor.Context
	cache                        map[cacheKey]cached
	options                      []string
	selected                     int
	popup, searching             bool
	searchText, diagnostic, hint string
}
type analysisMsg struct {
	id         int
	ctx        context.Context
	at         editor.Context
	diagnostic string
	explicit   bool
}
type suggestionsMsg struct {
	id     int
	key    cacheKey
	values []string
	err    error
}

func (m *Model) dismiss() {
	m.generation++
	if m.stopAnalysis != nil {
		m.stopAnalysis()
		m.stopAnalysis = nil
	}
	m.options = nil
	m.selected = 0
	m.popup = false
	m.searching = false
	m.hint = ""
}
func (m *Model) schedule(explicit bool) tea.Cmd {
	m.dismiss()
	s, pos, id := m.editor.Value(), editor.Position(m.editor), m.generation
	m.diagnostic = ""
	ctx, cancel := context.WithCancel(context.Background())
	m.stopAnalysis = cancel
	return func() tea.Msg {
		if !explicit {
			timer := time.NewTimer(180 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return nil
			case <-timer.C:
			}
		}
		return analysisMsg{id, ctx, editor.At(s, pos), editor.Diagnostic(s), explicit}
	}
}
func (m *Model) analyzed(a analysisMsg) tea.Cmd {
	if a.id != m.generation {
		return nil
	}
	c := a.at
	m.diagnostic = a.diagnostic
	scopedValue := c.Kind == "value" && c.Metric != "" && c.Label != ""
	if c.Kind == "" || !a.explicit && c.Prefix == "" && !scopedValue {
		return nil
	}
	m.context = c
	m.options = c.Local()
	m.popup = a.explicit || scopedValue || len(m.options) > 0
	if c.Kind == "duration" || c.Kind == "expr" && c.Prefix == "" || c.Kind == "value" && c.Metric == "" && c.Prefix == "" {
		return nil
	}
	k := cacheKey{c.Kind, c.Metric, c.Label, c.Prefix}
	if hit, ok := m.cache[k]; ok && time.Now().Before(hit.until) {
		m.options = c.Filter(append(m.options, hit.values...))
		m.popup = m.popup || len(m.options) > 0
		return nil
	}
	id := m.generation
	timeout := m.opt.Timeout
	if timeout <= 0 || timeout > 3*time.Second {
		timeout = 3 * time.Second
	}
	client := api.Client{Base: m.opt.Connection.URL, HTTP: http.DefaultClient}
	m.hint = "Loading suggestions…"
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(a.ctx, timeout)
		defer cancel()
		values, err := client.Suggestions(ctx, c.Kind, c.Metric, c.Label, c.Prefix)
		return suggestionsMsg{id, k, values, err}
	}
}
func (m *Model) suggested(s suggestionsMsg) {
	if m.generation != s.id {
		return
	}
	m.hint = ""
	if s.err != nil {
		m.hint = s.err.Error()
		return
	}
	if m.cache == nil {
		m.cache = map[cacheKey]cached{}
	}
	if len(m.cache) >= 64 {
		var oldest cacheKey
		var t time.Time
		for k, v := range m.cache {
			if t.IsZero() || v.until.Before(t) {
				oldest, t = k, v.until
			}
		}
		delete(m.cache, oldest)
	}
	m.cache[s.key] = cached{time.Now().Add(time.Minute), s.values}
	selected := ""
	if len(m.options) > 0 {
		selected = m.options[m.selected]
	}
	m.options = m.context.Filter(append(m.options, s.values...))
	for i, v := range m.options {
		if v == selected {
			m.selected = i
			break
		}
	}
	m.popup = m.popup || len(m.options) > 0
}
func (m *Model) accept() {
	if len(m.options) == 0 {
		return
	}
	c := m.context
	s := m.editor.Value()
	v := c.Insert(m.options[m.selected])
	// Replace just this token, including its suffix if completing mid-word.
	lineStart := strings.LastIndexByte(s[:c.Start], '\n') + 1
	col := utf8.RuneCountInString(s[lineStart:c.Start])
	line := m.editor.Line()
	m.editor.SetValue(s[:c.Start] + v + s[c.End:])
	for m.editor.Line() > line {
		m.editor.CursorStart()
		m.editor.CursorUp()
	}
	m.editor.SetCursorColumn(col + utf8.RuneCountInString(v))
}
func (m *Model) searchHistory() {
	var items []string
	for _, e := range m.entries {
		if e.Connection == m.opt.Connection.Name {
			items = append(items, e.Query)
		}
	}
	m.options = items
	if m.searchText != "" {
		m.options = nil
		for _, match := range fuzzy.Find(m.searchText, items) {
			m.options = append(m.options, items[match.Index])
		}
	}
	m.selected = 0
}
func (m *Model) assistKey(k tea.KeyPressMsg) (tea.Cmd, bool) {
	key := k.String()
	if key == "ctrl+r" {
		m.dismiss()
		m.searchText = ""
		m.searching = true
		m.searchHistory()
		return nil, true
	}
	if m.searching || m.popup {
		switch key {
		case "esc":
			m.dismiss()
			return nil, true
		case "down", "up":
			if key == "down" {
				m.selected = min(m.selected+1, max(0, len(m.options)-1))
			} else {
				m.selected = max(0, m.selected-1)
			}
			return nil, true
		}
	}
	if m.searching {
		switch key {
		case "enter":
			if len(m.options) > 0 {
				m.editor.SetValue(m.options[m.selected])
				m.cursor = -1
				m.editor.Focus()
				return m.schedule(false), true
			}
		case "backspace":
			_, n := utf8.DecodeLastRuneInString(m.searchText)
			m.searchText = m.searchText[:len(m.searchText)-n]
			m.searchHistory()
		default:
			if k.Text != "" {
				m.searchText += k.Text
				m.searchHistory()
			}
		}
		return nil, true
	}
	if key == "?" && (!m.editor.Focused() || !editor.LiteralAt(m.editor.Value(), editor.Position(m.editor))) {
		m.dismiss()
		m.help = true
		return nil, true
	}
	if m.editor.Focused() {
		switch key {
		case "ctrl+space", "ctrl+@":
			return m.schedule(true), true
		case "tab":
			if m.popup && len(m.options) > 0 {
				m.accept()
				m.dismiss()
				m.diagnostic = editor.Diagnostic(m.editor.Value())
				return nil, true
			}
			return m.schedule(true), true
		}
	}
	return nil, false
}
func (m *Model) menu(x, y int) *lipgloss.Layer {
	label := "Suggestions"
	if m.context.Kind == "value" {
		label = m.context.Label + " values"
	}
	if m.searching {
		label = "History · " + m.searchText + " ▏"
	}
	w := min(52, m.width-2)
	n := max(1, min(6, m.height-2-y-3))
	out := []string{label}
	if len(m.options) == 0 {
		msg := m.hint
		if msg == "" {
			msg = "No matches"
		}
		out = append(out, msg)
	}
	first := max(0, m.selected-n+1)
	for i := first; i < min(first+n, len(m.options)); i++ {
		mark := " "
		if i == m.selected {
			mark = "›"
		}
		out = append(out, fmt.Sprintf("%s %s", mark, m.options[i]))
	}
	contentWidth := min(23, w)
	for _, s := range out {
		contentWidth = max(contentWidth, min(w, ansi.StringWidth(ui.Plain(s))))
	}
	for i, s := range out {
		row := lipgloss.NewStyle().Width(contentWidth)
		if i == 0 {
			row = row.Bold(true)
		} else if len(m.options) > 0 && i == m.selected-first+1 {
			row = row.Reverse(true)
		}
		out[i] = row.Render(ansi.Truncate(ui.Plain(s), contentWidth, "…"))
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("6")).Render(strings.Join(out, "\n"))
	return lipgloss.NewLayer(box).X(min(x, m.width-contentWidth-2)).Y(y).Z(1)
}
