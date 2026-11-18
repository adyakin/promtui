package app

import (
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/query"
	"promtui/internal/ui"
)

type panels struct {
	panel                     keyScope
	offset                    int
	ranges                    []time.Duration
	rangeRow, seriesRow       int
	filtered                  []int
	filtering                 bool
	seriesFilter, filterDraft string
	labelText                 string
}

func (m *Model) openPanel(scope keyScope) {
	m.dismiss()
	m.panel, m.offset = scope, 0
}

func windowLabel(d time.Duration) string {
	s := d.String()
	if d != 0 && d%time.Minute == 0 {
		s = strings.TrimSuffix(s, "0s")
	}
	if d != 0 && d%time.Hour == 0 {
		s = strings.TrimSuffix(s, "0m")
	}
	return "Last " + s
}

func (m *Model) openRange() {
	m.openPanel(rangeKeys)
	m.ranges = slices.Clone(presets)
	if m.opt.Window > 0 && !slices.Contains(m.ranges, m.opt.Window) {
		m.ranges = append(m.ranges, m.opt.Window)
		slices.Sort(m.ranges)
	}
	m.rangeRow = max(0, slices.Index(m.ranges, m.opt.Window))
}

func (m *Model) settingsChanged() {
	m.status = "Query settings updated · F5 to run"
	if m.cancel != nil {
		m.status = "Running previous settings… · F5 to rerun"
	}
}

func seriesName(s query.Series) string {
	if s.Labels == "" || s.Labels == "{}" {
		return ui.Plain(s.Name)
	}
	return ui.Plain(s.Labels)
}

func (m *Model) openSeries() {
	m.openPanel(seriesKeys)
	m.seriesFilter, m.filtering = "", false
	m.refreshSeries()
}

func (m *Model) refreshSeries() {
	text := strings.ToLower(m.seriesFilter)
	m.filtered = m.filtered[:0]
	if m.chart.result.Start.IsZero() {
		m.seriesRow = 0
		return
	}
	for i, s := range m.chart.result.Series {
		if strings.Contains(strings.ToLower(seriesName(s)), text) {
			m.filtered = append(m.filtered, i)
		}
	}
	m.seriesRow = max(0, slices.Index(m.filtered, m.chart.selected))
	if len(m.filtered) > 0 {
		m.chart.selected = m.filtered[m.seriesRow]
	}
}

func (m *Model) selectedLabels() string {
	if m.chart.enabled {
		return m.chart.result.Series[m.chart.selected].Labels
	}
	return m.table.Rows[m.table.Row].Labels
}

func (m *Model) helpBody() []string {
	scope := m.mainScope()
	var lines []string
	for _, b := range bindings {
		if b.scope&scope == 0 || !m.available(b, scope) {
			continue
		}
		_, description, _ := strings.Cut(b.hint, " ")
		switch b.id {
		case "results":
			description = "Focus results"
		case "edit":
			description = "Focus query editor"
		case "page":
			description = "Select one page up or down"
		case "edge":
			description = "Select first or last row"
		case "pan":
			description = "Scroll horizontally"
		case "sample":
			description = "Older or newer sample"
		case "copyLabels":
			description = "Copy selected labels"
		case "recall":
			description = "Previous or next query"
		case "newline":
			description = "Insert newline"
		case "legacyView":
			description = "Toggle table/graph"
		case "mode":
			description = "Toggle instant/range"
		case "quit":
			description = "Quit PromTUI"
		}
		if description == "" {
			description = "Open time-range selector"
		}
		lines = append(lines, b.label+"  ·  "+description)
	}
	return lines
}

func (m *Model) panelBody() []string {
	var lines []string
	switch m.panel {
	case helpKeys:
		lines = m.helpBody()
	case detailKeys:
		for _, s := range []string{m.status, "Last result: " + m.summary, m.notice, m.diagnostic, m.historyError} {
			if s != "" {
				lines = append(lines, ui.Plain(s))
			}
		}
	case labelKeys:
		text := m.labelText
		if text == "" || text == "{}" {
			text = "No labels"
		}
		lines = []string{ui.Plain(text)}
	}
	return strings.Split(ansi.Hardwrap(strings.Join(lines, "\n"), m.width, false), "\n")
}

func (m *Model) panelView() string {
	names := map[keyScope]string{
		helpKeys: "Help · Query editor", detailKeys: "Request status",
		labelKeys: "Series labels", rangeKeys: "Time range", seriesKeys: "Series selector",
	}
	name := names[m.panel]
	if m.panel == helpKeys {
		switch m.mainScope() {
		case tableKeys:
			name = "Help · Table"
		case graphKeys:
			name = "Help · Graph"
		}
	}
	header := lipgloss.NewStyle().Bold(true).Reverse(true).Width(m.width)
	clip := func(s string) string { return ansi.Truncate(ui.Plain(s), m.width, "…") }
	lines := []string{header.Render(clip(" " + name))}
	row := func(text string, selected bool) {
		prefix := "  "
		if selected {
			prefix = "› "
		}
		line := clip(prefix + text)
		if selected {
			line = lipgloss.NewStyle().Reverse(true).Width(m.width).Render(line)
		}
		lines = append(lines, line)
	}
	switch m.panel {
	case rangeKeys:
		for i, d := range m.ranges {
			text := windowLabel(d)
			if m.opt.Ranged && d == m.opt.Window {
				text += " (current)"
			}
			row(text, i == m.rangeRow)
		}
		lines = append(lines, clip("  Apply sets range mode. F5 runs the query."))
	case seriesKeys:
		if m.filtering || m.seriesFilter != "" {
			input := "  Filter: " + m.seriesFilter
			if m.filtering {
				input += " ▏"
			}
			lines = append(lines, clip(input))
		}
		if len(m.filtered) == 0 {
			lines = append(lines, "  No matching series")
		}
		room := m.height - len(lines) - 1
		first := max(0, m.seriesRow-room+1)
		for i := first; i < min(first+room, len(m.filtered)); i++ {
			index := m.filtered[i]
			state := "○ Hidden"
			if m.chart.visible[index] {
				state = "● Shown "
			}
			row(state+"  "+seriesName(m.chart.result.Series[index]), i == m.seriesRow)
		}
	default:
		body := m.panelBody()
		start := min(m.offset, max(0, len(body)-m.height+2))
		lines = append(lines, body[start:min(len(body), start+m.height-2)]...)
	}
	for len(lines) < m.height-1 {
		lines = append(lines, "")
	}
	return strings.Join(append(lines, m.footer()), "\n")
}
