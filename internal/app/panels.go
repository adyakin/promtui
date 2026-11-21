package app

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/graph"
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
	labelFields               []labelField
	isolated                  []bool
	isolatedIndex             int
}

const dialogKeys = rangeKeys | seriesKeys | labelKeys | savedQueryKeys | queryErrorKeys

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
	filter := newFuzzyFilter(m.seriesFilter)
	m.filtered = m.filtered[:0]
	if m.chart.result.Start.IsZero() {
		m.seriesRow = 0
		return
	}
	for i, s := range m.chart.result.Series {
		if matchesSeries(filter, s) {
			m.filtered = append(m.filtered, i)
		}
	}
	m.seriesRow = max(0, slices.Index(m.filtered, m.chart.selected))
}

func (m *Model) willRestoreSeries() bool {
	return m.isolated != nil && len(m.filtered) > 0 && m.filtered[m.seriesRow] == m.isolatedIndex
}

func (m *Model) isolateSeries() {
	index := m.filtered[m.seriesRow]
	m.chart.selected = index
	if m.willRestoreSeries() {
		copy(m.chart.visible, m.isolated)
		m.isolated = nil
	} else {
		if m.isolated == nil {
			m.isolated = slices.Clone(m.chart.visible)
		}
		m.isolatedIndex = index
		for i := range m.chart.visible {
			m.chart.visible[i] = i == index
		}
	}
	m.chart.revision++
}

func (m *Model) selectedSeries() query.Series {
	if m.chart.enabled {
		return m.chart.result.Series[m.graphIndex(m.graphRow())]
	}
	return m.table.Rows[m.table.Row]
}

type labelField struct{ name, value string }

func (m *Model) openLabels() {
	m.openPanel(labelKeys)
	s := m.selectedSeries()
	m.labelText = s.Labels
	m.labelFields = m.labelFields[:0]
	for name, value := range s.Metric {
		m.labelFields = append(m.labelFields, labelField{ui.Plain(string(name)), ui.Plain(string(value))})
	}
	slices.SortFunc(m.labelFields, func(a, b labelField) int { return strings.Compare(a.name, b.name) })
	// Allow a formatted-only result to be inspected as well.
	if len(m.labelFields) == 0 && s.Labels != "" && s.Labels != "{}" {
		m.labelFields = append(m.labelFields, labelField{value: ui.Plain(s.Labels)})
	}
}

func (m *Model) canCopyLabels() bool {
	return m.labelText != "" && m.labelText != "{}"
}

func (m *Model) helpBody() []string {
	scope := m.mainScope()
	lines := []string{"Alt+0–5  ·  Switch app tabs (0–5 outside text inputs)"}
	if len(m.opt.Connections) > 0 {
		lines = append(lines, "c / Alt+C  ·  Switch connection (Alt+C while typing)")
	}
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
		case "filterResults":
			description = "Filter by metric name or labels"
		case "recall":
			description = "Previous or next query"
		case "newline":
			description = "Insert newline"
		case "mode":
			description = "Switch between instant and range and rerun"
		case "graph":
			description = "Run a range query and show Graph"
			if m.queryPhase == queryFailed {
				description = "Switch to Graph without rerunning"
			}
		case "table":
			description = "Run an instant query and show Table"
			if m.queryPhase == queryFailed {
				description = "Switch to Table without rerunning"
			}
		case "cancelOrQuit":
			description = "Cancel active query, or quit if idle"
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
		for _, s := range []string{m.status, "Last result: " + m.summary, m.notice, m.diagnostic, m.historyError, m.chart.err} {
			if s != "" {
				lines = append(lines, ui.Plain(s))
			}
		}
	}
	return strings.Split(ansi.Hardwrap(strings.Join(lines, "\n"), m.width, false), "\n")
}

func (m *Model) panelView() string {
	names := map[keyScope]string{
		helpKeys: "Help · Query editor", detailKeys: "Request status",
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
	lines := []string{m.header(promQLTab, name)}
	body := m.panelBody()
	start := min(m.offset, max(0, len(body)-m.height+2))
	lines = append(lines, body[start:min(len(body), start+m.height-2)]...)
	for len(lines) < m.height-1 {
		lines = append(lines, "")
	}
	return strings.Join(append(lines, m.footer()), "\n")
}

type dialogBounds struct{ x, y, width, height int }

func (m *Model) dialogBounds(w workspace) dialogBounds {
	if m.width < minWidth || m.height < minHeight {
		return dialogBounds{}
	}
	resultHeight := w.bottomY - w.resultY + 1
	centered := func(width, height int) dialogBounds {
		width = min(width, m.width-2)
		height = min(height, resultHeight)
		return dialogBounds{(m.width - width) / 2, w.resultY + (resultHeight-height)/2, width, height}
	}
	if m.pickingConnection {
		width := 28
		for _, c := range m.opt.Connections {
			width = max(width, ansi.StringWidth(ui.Plain(c.Name))+16)
		}
		return centered(min(60, width), len(m.opt.Connections)+2)
	}
	switch m.panel {
	case queryErrorKeys:
		width := min(72, m.width-2)
		height := min(max(4, resultHeight), len(m.queryError.lines(width-4))+2)
		y := w.resultY + (resultHeight-height)/2
		return dialogBounds{(m.width - width) / 2, min(y, w.bottomY-height+1), width, height}
	case rangeKeys:
		width := 30
		for _, d := range m.ranges {
			width = max(width, ansi.StringWidth(windowLabel(d))+17)
		}
		return centered(min(40, width), len(m.ranges)+2)
	case seriesKeys:
		width := min(m.width-2, max(22, (m.width*2+4)/5))
		height := max(1, len(m.filtered)) + 2
		minHeight := 3
		if m.filtering || m.seriesFilter != "" {
			height++
			minHeight++
		}
		// On short terminals move the dialog up enough for its frame,
		// filter, and at least one selectable item.
		maxHeight := max(minHeight, (resultHeight*7+5)/10)
		height = min(height, maxHeight)
		y := min(w.resultY+1, w.bottomY-height+1)
		return dialogBounds{m.width - width - 1, y, width, height}
	case labelKeys:
		name, value := 0, 0
		for _, f := range m.labelFields {
			name = max(name, ansi.StringWidth(f.name))
			value = max(value, ansi.StringWidth(f.value))
		}
		width := min(72, max(22, name+value+8))
		height := min(max(3, (resultHeight*7+5)/10), max(1, len(m.labelFields))+2)
		return centered(width, height)
	}
	return dialogBounds{}
}

func (m *Model) errorViewport() (lines []string, room, limit int) {
	d := m.dialogBounds(m.workspace())
	if d.width == 0 {
		return nil, 0, 0
	}
	lines = m.queryError.lines(d.width - 4)
	room = d.height - 2
	return lines, room, max(0, len(lines)-room)
}

var dialogSurface = lipgloss.NewStyle().
	Background(lipgloss.Color("#303844")).Foreground(lipgloss.Color("#e0e8f0"))

func (m *Model) dialogLayer(w workspace) *lipgloss.Layer {
	d := m.dialogBounds(w)
	if d.width == 0 {
		return nil
	}
	inner := d.width - 2
	var name, meta string
	var lines []string
	row := func(text, swatch string, selected bool) {
		lines = append(lines, dialogRow(text, swatch, inner, selected))
	}

	if m.pickingConnection {
		room := d.height - 2
		first := dialogStart(m.connectionRow, len(m.opt.Connections), room)
		for i := first; i < min(first+room, len(m.opt.Connections)); i++ {
			text := ui.Plain(m.opt.Connections[i].Name)
			if m.opt.Connections[i].Name == m.opt.Connection.Name {
				text += " (current)"
			}
			row(text, "", i == m.connectionRow)
		}
		return lipgloss.NewLayer(renderFrame("Connections", "", lines, d.width, true, dialogSurface)).X(d.x).Y(d.y).Z(1)
	}
	switch m.panel {
	case queryErrorKeys:
		name = "Query Error"
		text, room, limit := m.errorViewport()
		first := min(m.offset, limit)
		if limit > 0 {
			meta = fmt.Sprintf("%d–%d/%d", first+1, first+room, len(text))
		}
		for _, line := range text[first:min(first+room, len(text))] {
			lines = append(lines, dialogSurface.Render(" "+line))
		}
	case rangeKeys:
		name = "Time range"
		room := d.height - 2
		first := dialogStart(m.rangeRow, len(m.ranges), room)
		for i := first; i < min(first+room, len(m.ranges)); i++ {
			window := m.ranges[i]
			text := windowLabel(window)
			if m.opt.Ranged && window == m.opt.Window {
				text += " (current)"
			}
			row(text, "", i == m.rangeRow)
		}
	case seriesKeys:
		name = "Series selector"
		if m.filtering || m.seriesFilter != "" {
			input := "  Filter: " + m.seriesFilter
			if m.filtering {
				input += " ▏"
			}
			lines = append(lines, dialogSurface.Render(input))
		}
		if len(m.filtered) == 0 {
			lines = append(lines, dialogSurface.Render("  No matching series"))
		}
		room := d.height - len(lines) - 2
		first := dialogStart(m.seriesRow, len(m.filtered), room)
		for i := first; i < min(first+room, len(m.filtered)); i++ {
			index := m.filtered[i]
			state := "[ ]"
			if m.chart.visible[index] {
				state = "[x]"
			}
			row(state+" "+seriesName(m.chart.result.Series[index]), graph.Color(index), i == m.seriesRow)
		}
	case labelKeys:
		name = "Series labels"
		if len(m.labelFields) == 0 {
			row("No labels", "", false)
			break
		}
		room := d.height - 2
		first := dialogStart(m.offset, len(m.labelFields), room)
		nameWidth := 0
		for _, f := range m.labelFields {
			nameWidth = max(nameWidth, min(inner/3, ansi.StringWidth(f.name)))
		}
		for i := first; i < min(first+room, len(m.labelFields)); i++ {
			f := m.labelFields[i]
			text := f.value
			if nameWidth > 0 {
				key := ansi.Truncate(f.name, nameWidth, "…")
				text = key + strings.Repeat(" ", nameWidth-ansi.StringWidth(key)+2) + text
			}
			row(text, "", i == m.offset)
		}
	}
	return lipgloss.NewLayer(renderFrame(name, meta, lines, d.width, true, dialogSurface)).X(d.x).Y(d.y).Z(1)
}

func dialogStart(selected, count, room int) int {
	return min(max(0, selected-room/2), max(0, count-room))
}

func dialogRow(text, swatch string, inner int, selected bool) string {
	prefix := "   "
	style := dialogSurface
	if selected {
		prefix = " › "
		style = style.Bold(true).Background(lipgloss.Color("#465972")).Foreground(lipgloss.Color("15"))
	}
	prefix = style.Render(prefix)
	if swatch != "" {
		prefix += style.Foreground(lipgloss.Color(swatch)).Render("■") + style.Render(" ")
	}
	room := inner - ansi.StringWidth(prefix) - 1
	return prefix + style.Width(room).Render(ansi.Truncate(text, room, "…")) + style.Render(" ")
}
