package app

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/graph"
	"promtui/internal/kitty"
	"promtui/internal/query"
)

type placement struct{ revision, row, col, cols, rows, cw, ch int }
type graphFrame struct {
	key    placement
	plot   image.Rectangle
	packet string
	err    error
}
type graphPacket string
type graphState struct {
	result                     query.Result
	enabled                    bool
	selected, revision, cw, ch int
	visible                    []bool
	names                      []string
	plot                       image.Rectangle // zero-based screen cells for the displayed plot
	hover                      time.Time
	wanted, displayed, failed  placement
	renderCancel               context.CancelFunc
	sending, everSent          bool
	buffer                     int
	err                        string
}

func (m *Model) setGraphResult(r query.Result) {
	g := &m.chart
	m.isolated = nil
	g.revision++
	g.result = r
	g.names = graphSeriesNames(r.Series)
	g.hover = time.Time{}
	g.selected = 0
	g.enabled = m.opt.Graphics && !r.Start.IsZero() && len(r.Series) > 0
	g.visible = make([]bool, len(r.Series))
	for i := range g.visible {
		g.visible[i] = i < 8
	}
}

func (m *Model) graphView() bool {
	return m.chart.enabled || m.opt.Graphics && m.opt.Ranged && len(m.table.Rows) == 0 && len(m.resultFilter.allRows) == 0
}
func (m *Model) hasVisibleSeries() bool {
	for row := range m.graphSeriesCount() {
		if m.chart.visible[m.graphIndex(row)] {
			return true
		}
	}
	return false
}
func (m *Model) graphPlace() placement {
	g := &m.chart
	if m.activeTab != promQLTab || !g.enabled || m.quitting || m.panel&(helpKeys|detailKeys) != 0 || m.popup || m.searching ||
		!m.hasVisibleSeries() || m.width < minWidth || m.height < minHeight {
		return placement{}
	}
	w := m.workspace()
	// Keep the raster inside the frame and above the legend. Floating
	// dialogs paint opaque cells above the image without moving it.
	rows, cols := w.bottomY-w.bodyY-1-m.inspectionReserve(w), m.width-2
	if rows < 5 || cols < 32 {
		return placement{}
	}
	return placement{g.revision, w.bodyY + 1, 2, cols, rows, g.cw, g.ch}
}

func (m *Model) legend() string {
	g := &m.chart
	count := m.graphSeriesCount()
	if count == 0 {
		return ""
	}
	var entries []string
	width := 0
	selected := m.graphRow()
	// Keep the selected series first; further entries form a compact legend.
	for n := 0; n < count; n++ {
		row := (selected + n) % count
		i := m.graphIndex(row)
		state := "●"
		if !g.visible[i] {
			state = "○"
		}
		entry := lipgloss.NewStyle().Foreground(lipgloss.Color(graph.Color(i))).Render(state) + " " + g.names[i]
		if n == 0 {
			entry = fmt.Sprintf("› %d/%d %s", row+1, count, entry)
		}
		suffix := ""
		if remaining := count - n - 1; remaining > 0 {
			suffix = fmt.Sprintf("  +%d more", remaining)
		}
		room := m.width - 2 - ansi.StringWidth(suffix) - width
		if n > 0 {
			entry = "  " + entry
		}
		if n > 0 && ansi.StringWidth(entry) > room {
			break
		}
		entry = ansi.Truncate(entry, max(1, room), "…")
		entries = append(entries, entry)
		width += ansi.StringWidth(entry)
	}
	legend := strings.Join(entries, "")
	if remaining := count - len(entries); remaining > 0 {
		legend += fmt.Sprintf("  +%d more", remaining)
	}
	return ansi.Truncate(legend, m.width-2, "…")
}

// One render and one raw write at a time. RawMsg comes back through Update
// after Tea queues the write; do not race a later delete against an upload.
func (m *Model) syncGraph(msg tea.Msg) tea.Cmd {
	g := &m.chart
	k := m.graphPlace()
	if k != g.displayed || !m.canInspectGraph() {
		g.hover = time.Time{}
	}
	if k != g.wanted {
		if g.renderCancel != nil {
			g.renderCancel()
		}
		g.wanted = k
		g.failed = placement{}
		g.err = ""
	}
	switch v := msg.(type) {
	case tea.RawMsg:
		if _, ok := v.Msg.(graphPacket); ok {
			g.sending = false
		}
	case graphFrame:
		if g.renderCancel != nil {
			g.renderCancel()
			g.renderCancel = nil
		}
		if v.key == k && k.cols != 0 && !errors.Is(v.err, context.Canceled) {
			if v.err != nil {
				g.failed = k
				g.err = v.err.Error()
			} else {
				g.displayed = k
				g.plot = v.plot
				g.everSent = true
				g.sending = true
				return tea.Raw(graphPacket(v.packet))
			}
		}
	}
	if g.sending {
		return nil
	}
	if g.displayed.cols != 0 && (k.cols == 0 || g.failed == k || k.row != g.displayed.row ||
		k.col != g.displayed.col || k.rows != g.displayed.rows || k.cols != g.displayed.cols) {
		g.displayed = placement{}
		g.plot = image.Rectangle{}
		g.sending = true
		return tea.Raw(graphPacket(kitty.Clear()))
	}
	if k.cols == 0 || k == g.displayed || k == g.failed || g.renderCancel != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	g.renderCancel = cancel
	p := graph.Plot{Start: g.result.Start, End: g.result.End, Step: g.result.Step}
	for row := range m.graphSeriesCount() {
		i := m.graphIndex(row)
		if g.visible[i] {
			p.Lines = append(p.Lines, graph.Line{Index: i, Samples: g.result.Series[i].Samples})
		}
	}
	g.buffer = 1 - g.buffer
	id := kitty.BaseID + g.buffer
	return func() tea.Msg {
		// Coalesce rapid window changes and visibility toggles before rasterizing.
		timer := time.NewTimer(35 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return graphFrame{key: k, err: ctx.Err()}
		case <-timer.C:
		}
		cw, ch := k.cw, k.ch
		if cw == 0 || ch == 0 {
			cw, ch = 8, 16
		}
		w, h := float64(k.cols*cw), float64(k.rows*ch)
		scale := math.Min(1, math.Min(3200/w, 2000/h))
		pw, ph := int(w*scale), int(h*scale)
		data, bounds, err := p.Render(ctx, pw, ph, float64(ch)*.75*scale)
		f := graphFrame{key: k, err: err}
		if err == nil {
			f.packet = kitty.Frame(data, id, k.row, k.col, k.cols, k.rows)
			f.plot = k.plotCells(bounds, pw, ph)
		}
		return f
	}
}
func (m *Model) CloseGraphics() string {
	if m.chart.renderCancel != nil {
		m.chart.renderCancel()
	}
	if m.chart.everSent {
		return kitty.Clear()
	}
	return ""
}
