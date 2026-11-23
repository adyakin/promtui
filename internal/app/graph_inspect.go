package app

import (
	"fmt"
	"image"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/prometheus/common/model"
	"promtui/internal/graph"
	"promtui/internal/query"
)

func (m *Model) inspectionReserve(w workspace) int {
	if !m.opt.Mouse {
		return 0
	}
	// Preserve the five-row minimum raster. In a tight terminal the
	// inspection can share the legend row instead.
	return min(5, max(0, w.bottomY-w.bodyY-1-5))
}

func (m *Model) canInspectGraph() bool {
	return m.opt.Mouse && m.activeTab == promQLTab && m.queryPhase == querySucceeded &&
		m.panel == 0 && !m.popup && !m.searching && !m.pickingConnection &&
		m.parameterDialog == nil && !m.quitting && m.chart.err == ""
}

func (p placement) plotCells(b graph.Bounds, width, height int) image.Rectangle {
	// Select cells that intersect the raster's actual plot, not the axes
	// or borders. Kitty positions are one-based; mouse coordinates are not.
	return image.Rect(
		int(math.Floor(b.Left*float64(p.cols)/float64(width))),
		int(math.Floor(b.Top*float64(p.rows)/float64(height))),
		int(math.Ceil(b.Right*float64(p.cols)/float64(width))),
		int(math.Ceil(b.Bottom*float64(p.rows)/float64(height))),
	).Add(image.Pt(p.col-1, p.row-1))
}

func inspectionTime(r query.Result, x, cols int) time.Time {
	span := r.End.Sub(r.Start)
	delta := time.Duration(math.Round(float64(span) * float64(x) / float64(max(1, cols-1))))
	if r.Step > 0 {
		// The backend evaluates at Start + n*Step; End need not be on that grid.
		delta = min(delta.Round(r.Step), span-span%r.Step)
	}
	return r.Start.Add(delta)
}

func inspectionValue(samples []model.SamplePair, at time.Time, step time.Duration) string {
	// Range API samples are in timestamp order. Don't bridge missing steps
	// or skip non-finite samples in favor of older numbers.
	i := sort.Search(len(samples), func(i int) bool { return !samples[i].Timestamp.Time().Before(at) })
	if i > 0 && (i == len(samples) || at.Sub(samples[i-1].Timestamp.Time()) <= samples[i].Timestamp.Time().Sub(at)) {
		i--
	}
	if i == len(samples) || !samples[i].Timestamp.Time().Equal(at) &&
		at.Sub(samples[i].Timestamp.Time()).Abs() > max(time.Millisecond, step/2) {
		return "no sample"
	}
	v := samples[i].Value
	if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
		return "no sample"
	}
	return strconv.FormatFloat(float64(v), 'g', -1, 64)
}

func (m *Model) renderInspection(rows []string) {
	g := &m.chart
	at := g.hover.UTC().Format(time.RFC3339Nano)
	rows[0] = at
	visible, first := 0, -1
	total := m.graphSeriesCount()
	selected := m.graphRow()
	for n := 0; n < total; n++ {
		i := m.graphIndex((selected + n) % total)
		if !g.visible[i] {
			continue
		}
		visible++
		if first < 0 {
			first = i
		}
		if visible < len(rows) {
			rows[visible] = m.inspectionEntry(i, m.width-2)
		}
	}
	if len(rows) == 1 && first >= 0 {
		// Share one text row in a tight terminal, keeping the timestamp and
		// the first visible value as well as a count of omitted series.
		at := g.hover.UTC().Format("15:04:05Z")
		suffix := ""
		if more := visible - 1; more > 0 {
			suffix = fmt.Sprintf(" +%d", more)
		}
		rows[0] = at + "  " + m.inspectionEntry(first, m.width-4-len(at)-len(suffix)) + suffix
		return
	}
	if more := visible - (len(rows) - 1); more > 0 {
		rows[0] += fmt.Sprintf("  +%d more", more)
	}
}

func (m *Model) inspectionEntry(i, width int) string {
	g := &m.chart
	value := inspectionValue(g.result.Series[i].Samples, g.hover, g.result.Step)
	name := ansi.Truncate(g.names[i], max(1, width-5-ansi.StringWidth(value)), "…")
	marker := lipgloss.NewStyle().Foreground(lipgloss.Color(graph.Color(i))).Render("●")
	return strings.Join([]string{marker, name, value}, "  ")
}
