package app

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/prometheus/common/model"
)

func TestTableResultsFilter(t *testing.T) {
	m := New(Options{}, nil)
	r := sampleResult(false)
	r.Series[0].Name = "machine_cpu_seconds"
	r.Series[0].Metric = model.Metric{"host": "alpha", "cluster_zone": "west"}
	r.Series[0].Samples[0].Value = 987654
	r.Series[0].Samples[0].Timestamp = 432109876
	text := "needleval"
	r.Series[0].Text = &text
	m.Update(completed{result: r})

	for _, tc := range []struct {
		filter string
		count  int
	}{
		{"MCNCPU", 1}, {"CLSTRZN", 1}, {"hST=ap", 1}, {"BtA", 1},
		{"alphawest", 0}, {"hstalp", 0}, {"987654", 0}, {"1970", 0}, {"needleval", 0},
	} {
		press(m, '/')
		m.Update(tea.PasteMsg{Content: tc.filter})
		if len(m.table.Rows) != tc.count || m.View().Cursor == nil {
			t.Fatalf("filter %q: got %d rows, want %d\n%s", tc.filter, len(m.table.Rows), tc.count, m.screen())
		}
		if tc.count == 0 && !strings.Contains(m.screen(), "No series matching the current filter") {
			t.Fatal(m.screen())
		}
		press(m, tea.KeyEsc)
	}
	press(m, '/')
	m.Update(tea.PasteMsg{Content: "ALP"})
	press(m, tea.KeyEnter, 'v', '/')
	m.Update(tea.PasteMsg{Content: "not-found"})
	press(m, tea.KeyEsc)
	if len(m.table.Rows) != 1 || m.resultFilter.input.Value() != "ALP" || m.View().Cursor != nil {
		t.Fatal("Esc did not restore the applied filter")
	}
	press(m, '/', tea.KeyBackspace, tea.KeyBackspace, tea.KeyBackspace, tea.KeyEnter)
	if len(m.table.Rows) != 2 || m.table.Rows[0].Name != "machine_cpu_seconds" {
		t.Fatal("clearing did not restore rows in the current sort order")
	}
	m.resize(minWidth, minHeight)
	press(m, '/')
	screen, cursor := m.renderScreen()
	if len(strings.Split(screen, "\n")) != minHeight || cursor == nil || cursor.Position.Y != m.workspace().bodyY-1 {
		t.Fatal("filter did not fit the smallest results pane")
	}
}

func TestGraphResultsFilter(t *testing.T) {
	m := New(Options{Graphics: true}, nil)
	m.Update(completed{result: sampleResult(true)})
	press(m, tea.KeyDown, tea.KeySpace)
	before := slices.Clone(m.chart.visible)
	press(m, '/')
	m.Update(tea.PasteMsg{Content: "APh"})
	press(m, tea.KeyEnter)
	if !slices.Equal(m.resultFilter.matches, []int{0}) || !m.hasVisibleSeries() ||
		strings.Contains(m.legend(), "beta") || !slices.Equal(m.chart.visible, before) {
		t.Fatal("graph filter did not mask the legend and plot independently of manual visibility")
	}
	press(m, 'l')
	if !strings.Contains(m.labelText, "alpha") {
		t.Fatal("label inspection targeted a filtered-out series")
	}
	press(m, tea.KeyEsc, '/')
	m.Update(tea.PasteMsg{Content: "not-found"})
	if m.graphSeriesCount() != 0 || m.graphPlace().cols != 0 ||
		!strings.Contains(m.screen(), "No series matching the current filter") {
		t.Fatal("empty graph filter did not clear the plot and show the empty state")
	}
	press(m, tea.KeyEsc)
	press(m, '/', tea.KeyBackspace, tea.KeyBackspace, tea.KeyBackspace)
	m.Update(tea.PasteMsg{Content: "bt"})
	if !slices.Equal(m.resultFilter.matches, []int{1}) || m.hasVisibleSeries() {
		t.Fatal("filter showed a matching series that was manually hidden")
	}
	press(m, tea.KeyBackspace, tea.KeyBackspace, tea.KeyEnter)
	if m.resultFilter.matches != nil || !slices.Equal(m.chart.visible, before) ||
		!strings.Contains(m.legend(), "beta") {
		t.Fatal("clearing did not restore the previous graph visibility")
	}
	press(m, 's', '/')
	m.Update(tea.PasteMsg{Content: "ht=bt"})
	if !slices.Equal(m.filtered, []int{1}) {
		t.Fatal("series picker did not apply the same label matching")
	}
}
