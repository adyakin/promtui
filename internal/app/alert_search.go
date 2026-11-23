package app

import (
	"fmt"
	"maps"
	"slices"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"promtui/internal/ui"
)

const alertSuggestionLimit = 6

type alertSearchSuggestions struct {
	names, matches []string
	selected       int // One-based; zero means no explicit selection.
}

func (s *alertSearchSuggestions) selectedName() string {
	if s.selected == 0 {
		return ""
	}
	return s.matches[s.selected-1]
}

// refreshAlertNames indexes all loaded rows, regardless of active filters.
func (a *alertsState) refreshAlertNames() {
	s := &a.suggestions
	selected := s.selectedName()
	names := make(map[string]struct{})
	for _, row := range a.rows {
		if name := row.Labels["alertname"]; name != "" {
			names[name] = struct{}{}
		}
	}
	s.names = slices.Sorted(maps.Keys(names))
	if !a.input.Focused() {
		s.matches = s.matches[:0]
		s.selected = 0
		return
	}
	a.refreshSearchSuggestions()
	if selected != "" {
		s.selected = slices.Index(s.matches, selected) + 1
	}
}

// refreshSearchSuggestions starts a fresh match after opening or editing search.
func (a *alertsState) refreshSearchSuggestions() {
	s := &a.suggestions
	s.matches = s.matches[:0]
	s.selected = 0
	filter := newFuzzyFilter(a.input.Value())
	if len(filter) == 0 {
		return
	}
	for _, name := range s.names {
		if filter.matches(name) {
			s.matches = append(s.matches, name)
		}
	}
}

// searchSuggestionKey reports whether it handled navigation or filled the
// input. Enter still commits through the caller's ordinary fuzzy-search path.
func (a *alertsState) searchSuggestionKey(key string) bool {
	s := &a.suggestions
	if !a.input.Focused() || len(s.matches) == 0 {
		return false
	}
	switch key {
	case "up":
		if s.selected == 0 {
			s.selected = len(s.matches)
		} else {
			s.selected = max(1, s.selected-1)
		}
	case "down":
		s.selected = min(len(s.matches), s.selected+1)
	case "enter":
		if name := s.selectedName(); name != "" {
			a.input.SetValue(name)
			a.input.CursorEnd()
			return true
		}
		return false
	default:
		return false
	}
	return true
}

func (m *Model) alertSuggestionsFit() bool {
	return m.width >= minWidth && m.height >= minHeight
}

func (m *Model) alertSuggestionsLayer() *lipgloss.Layer {
	a := &m.alerts
	s := &a.suggestions
	if !a.input.Focused() || len(s.matches) == 0 || !m.alertSuggestionsFit() {
		return nil
	}
	const x, y = 3, 4 // Directly below the input at screen row 3.
	width := min(48, m.width-x-1)
	// Leave the pane's bottom border, status line, and footer visible.
	room := min(alertSuggestionLimit, len(s.matches), m.height-y-5)
	first := dialogStart(s.selected-1, len(s.matches), room)
	meta := ""
	if room < len(s.matches) {
		meta = fmt.Sprintf("%d–%d/%d", first+1, first+room, len(s.matches))
	}
	rows := make([]string, room)
	for i, name := range s.matches[first : first+room] {
		rows[i] = dialogRow(ui.Plain(name), "", width-2, first+i+1 == s.selected)
	}
	m.mouseDialog(x, y, width, room, m.paneKey)
	m.mouseRows(x+1, y+1, width-2, first, first+room, "alert names",
		func(i int) string { return s.matches[i] }, func(i int) tea.Cmd { s.selected = i + 1; return nil }, nil)
	return lipgloss.NewLayer(renderFrame("Alert names", meta, rows, width, true, dialogSurface)).X(x).Y(y).Z(1)
}
