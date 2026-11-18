package app

import (
	"context"
	"errors"
	"fmt"
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

type placement struct{ revision, row, cols, rows, cw, ch int }
type graphFrame struct {
	key    placement
	packet string
	err    error
}
type graphPacket string
type graphState struct {
	result                     query.Result
	enabled, preferTable       bool
	selected, revision, cw, ch int
	visible                    []bool
	wanted, displayed, failed  placement
	renderCancel               context.CancelFunc
	sending, everSent          bool
	buffer                     int
	err                        string
}

func (m *Model) setGraphResult(r query.Result) {
	g := &m.chart
	g.revision++
	g.result = r
	g.selected = 0
	g.enabled = m.opt.Graphics && !r.Start.IsZero() && !g.preferTable
	g.visible = make([]bool, len(r.Series))
	for i := range g.visible {
		g.visible[i] = i < 8
	}
}
func (m *Model) hasVisibleSeries() bool {
	for _, on := range m.chart.visible {
		if on {
			return true
		}
	}
	return false
}
func (m *Model) graphPlace() placement {
	g := &m.chart
	if !g.enabled || m.quitting || m.scope()&mainKeys == 0 || !m.hasVisibleSeries() {
		return placement{}
	}
	// The image starts on the first result row (Kitty uses 1-based positions)
	// and leaves the legend, status and shortcuts beneath it.
	row := m.resultTop() + 1
	rows := m.height - m.resultTop() - 3
	if rows < 5 || m.width < 32 {
		return placement{}
	}
	return placement{g.revision, row, m.width, rows, g.cw, g.ch}
}

func (m *Model) legend() string {
	g := &m.chart
	if len(g.visible) == 0 {
		return ""
	}
	var entries []string
	width := 0
	// Keep the selected series first; further entries form a compact legend.
	for n := 0; n < len(g.visible); n++ {
		i := (g.selected + n) % len(g.visible)
		state := "●"
		if !g.visible[i] {
			state = "○"
		}
		entry := lipgloss.NewStyle().Foreground(lipgloss.Color(graph.Color(i))).Render(state) + " " + seriesName(g.result.Series[i])
		if n == 0 {
			entry = fmt.Sprintf("› %d/%d %s", i+1, len(g.visible), entry)
		}
		suffix := ""
		if remaining := len(g.visible) - n - 1; remaining > 0 {
			suffix = fmt.Sprintf("  +%d more", remaining)
		}
		room := m.width - ansi.StringWidth(suffix) - width
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
	if remaining := len(g.visible) - len(entries); remaining > 0 {
		legend += fmt.Sprintf("  +%d more", remaining)
	}
	return ansi.Truncate(legend, m.width, "…")
}

// One render and one raw write at a time. RawMsg comes back through Update
// after Tea queues the write; do not race a later delete against an upload.
func (m *Model) syncGraph(msg tea.Msg) tea.Cmd {
	g := &m.chart
	k := m.graphPlace()
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
				g.everSent = true
				g.sending = true
				return tea.Raw(graphPacket(v.packet))
			}
		}
	}
	if g.sending {
		return nil
	}
	if g.displayed.cols != 0 && (k.cols == 0 || g.failed == k || k.row != g.displayed.row || k.rows != g.displayed.rows || k.cols != g.displayed.cols) {
		g.displayed = placement{}
		g.sending = true
		return tea.Raw(graphPacket(kitty.Clear()))
	}
	if k.cols == 0 || k == g.displayed || k == g.failed || g.renderCancel != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	g.renderCancel = cancel
	p := graph.Plot{Start: g.result.Start, End: g.result.End, Step: g.result.Step}
	for i, on := range g.visible {
		if on {
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
		data, err := p.Render(ctx, int(w*scale), int(h*scale), float64(ch)*.75*scale)
		f := graphFrame{key: k, err: err}
		if err == nil {
			f.packet = kitty.Frame(data, id, k.row, k.cols, k.rows)
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
