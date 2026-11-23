package app

import (
	"image"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/prometheus/common/model"
	"promtui/internal/query"
)

func TestInspectionTime(t *testing.T) {
	start := time.Unix(100, 0)
	r := query.Result{Start: start, End: start.Add(10500 * time.Millisecond), Step: time.Second}
	for x, want := range []time.Duration{0, 3, 5, 8, 10} {
		if got := inspectionTime(r, x, 5).Sub(start); got != want*time.Second {
			t.Errorf("column %d: got %v, want %ds", x, got, want)
		}
	}
}

func TestInspectionValue(t *testing.T) {
	// A non-finite sample and a missing step must not reuse earlier values.
	samples := []model.SamplePair{
		{Timestamp: 1000, Value: 11},
		{Timestamp: 2100, Value: 22},
		{Timestamp: 3000, Value: model.SampleValue(math.NaN())},
		{Timestamp: 5000, Value: 55},
	}
	for ms, want := range map[int64]string{0: "no sample", 1000: "11", 2000: "22", 3000: "no sample", 4000: "no sample", 5000: "55", 6000: "no sample"} {
		if got := inspectionValue(samples, time.UnixMilli(ms), time.Second); got != want {
			t.Errorf("at %dms: got %s, want %s", ms, got, want)
		}
	}
	if got := inspectionValue(nil, time.Unix(1, 0), time.Second); got != "no sample" {
		t.Fatal(got)
	}
}

func TestGraphMouseInspection(t *testing.T) {
	m := New(Options{Graphics: true, Mouse: true}, nil)
	start := time.Unix(100, 0)
	r := query.Result{Start: start, End: start.Add(2 * time.Second), Step: time.Second}
	for _, name := range []string{"visible", "hidden", "gap", "filtered"} {
		r.Series = append(r.Series, query.Series{Metric: model.Metric{"instance": model.LabelValue(name)},
			Samples: []model.SamplePair{{Timestamp: 101000, Value: 42}, {Timestamp: 102000, Value: 43}}})
	}
	r.Series[2].Samples = r.Series[2].Samples[1:]
	m.setGraphResult(r)
	m.queryPhase = querySucceeded
	m.chart.visible[1] = false
	m.resultFilter.matches = []int{0, 1, 2}
	upload := func(render tea.Cmd) graphFrame {
		t.Helper()
		frame := render().(graphFrame)
		if frame.err != nil || frame.plot.Empty() {
			t.Fatalf("bad frame: %v %v", frame.plot, frame.err)
		}
		_, send := m.Update(frame)
		m.Update(send())
		return frame
	}
	frame := upload(m.syncGraph(nil))
	hover := func(p image.Point) tea.Cmd {
		_, cmd := m.Update(tea.MouseMotionMsg{X: p.X, Y: p.Y})
		return cmd
	}
	view := m.View()
	if view.MouseMode != tea.MouseModeAllMotion || !view.ReportFocus {
		t.Fatal("graph does not request hover and focus events")
	}
	inside := image.Pt((frame.plot.Min.X+frame.plot.Max.X)/2, frame.plot.Min.Y)
	if cmd := hover(inside); cmd != nil || m.chart.displayed != frame.key {
		t.Fatal("hover requested a Kitty operation")
	}
	text := ansi.Strip(m.screen())
	for _, want := range []string{"1970-01-01T00:01:41Z", "●  visible  42", "●  gap  no sample"} {
		if !strings.Contains(text, want) {
			t.Errorf("inspection missing %q", want)
		}
	}
	if strings.Contains(text, "●  hidden") || strings.Contains(text, "filtered") {
		t.Fatal("inspection included a non-visible series")
	}
	_, wheel := m.Update(tea.MouseWheelMsg{X: inside.X, Y: inside.Y, Button: tea.MouseWheelDown})
	if wheel != nil || m.chart.selected != 1 || m.chart.hover.IsZero() {
		t.Fatal("wheel stopped selecting series or disturbed the plot")
	}
	if cmd := hover(image.Pt(frame.plot.Min.X-1, inside.Y)); cmd != nil || !m.chart.hover.IsZero() ||
		strings.Contains(m.screen(), "no sample") {
		t.Fatal("leaving the plot didn't clear inspection")
	}
	hover(inside)
	if _, cmd := m.Update(tea.BlurMsg{}); cmd != nil || !m.chart.hover.IsZero() {
		t.Fatal("focus loss didn't clear inspection")
	}
	hover(inside)
	_, clear := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if !m.chart.hover.IsZero() {
		t.Fatal("resize kept a stale hover")
	}
	_, render := m.Update(clear())
	resized := upload(render)
	if resized.plot == frame.plot || hover(resized.plot.Min) != nil || !m.chart.hover.Equal(start) {
		t.Fatal("hover did not remap to the resized plot")
	}
	press(m, 's')
	if !m.chart.hover.IsZero() || hover(resized.plot.Min) != nil || !m.chart.hover.IsZero() {
		t.Fatal("hover leaked through the selector overlay")
	}
}

func TestGraphInspectionHeightAndDisabledMouse(t *testing.T) {
	r := sampleResult(true)
	for range 6 {
		r.Series = append(r.Series, r.Series[0])
	}
	m := New(Options{Graphics: true, Mouse: true}, nil)
	m.setGraphResult(r)
	m.chart.hover = r.Start
	rows := make([]string, m.inspectionReserve(m.workspace()))
	m.renderInspection(rows)
	if len(rows) != 5 || !strings.HasSuffix(rows[0], "+4 more") {
		t.Fatalf("expected bounded inspection: %v", rows)
	}
	m.opt.Mouse = false
	if m.View().MouseMode != tea.MouseModeNone || m.inspectionReserve(m.workspace()) != 0 {
		t.Fatal("disabled mouse reserved inspection space or requested mouse events")
	}
}

func TestGraphSeriesNames(t *testing.T) {
	r := []query.Series{
		{Metric: model.Metric{"instance": "pgsql01:9100", "servicename": "postgres", "mountpoint": "/"}},
		{Labels: `{instance="pgsql01:9100",mountpoint="/var/lib/postgresql"}`},
		{Metric: model.Metric{"instance": "pgsql02:9100", "mountpoint": "/"}},
		{Metric: model.Metric{"servicename": "postgres", "job": "job-a"}},
		{Metric: model.Metric{"job": "job-a"}},
		{Metric: model.Metric{"region": "east"}},
		{Metric: model.Metric{"region": "east"}},
		{Metric: model.Metric{"instance": "region=east #2"}},
	}
	want := []string{"pgsql01:9100 · /", "pgsql01:9100 · /var/lib/postgresql", "pgsql02:9100",
		"postgres", "job-a", "region=east", "region=east #3", "region=east #2"}
	if got := graphSeriesNames(r); !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
