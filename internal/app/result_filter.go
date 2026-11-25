package app

import (
	"slices"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"promtui/internal/query"
	"promtui/internal/ui"
)

type resultFilterState struct {
	input   textinput.Model
	before  string
	anchor  ui.Table
	allRows []query.Series
	matches []int // Graph indices, in legend order; nil when unfiltered.
}

func newResultFilter() resultFilterState {
	input := textinput.New()
	input.Prompt = "  Filter: "
	input.Placeholder = "type a metric name or label"
	input.CharLimit = 4096
	input.SetVirtualCursor(false)
	return resultFilterState{input: input}
}

func matchesSeries(f fuzzyFilter, s query.Series) bool {
	if f.matches(s.Name) || matchLabels(f, s.Metric) {
		return true
	}
	if len(s.Metric) == 0 && s.Labels != "" && s.Labels != "{}" {
		// Some results have formatted labels but no original metric map.
		metric, err := promQLParser.ParseMetric(s.Labels)
		if err == nil {
			return matchLabels(f, metric.Map())
		}
	}
	return false
}

func (m *Model) refreshResultFilter() {
	r := &m.resultFilter
	f := newFuzzyFilter(r.input.Value())
	if len(f) == 0 {
		m.table = ui.Table{Rows: r.allRows}
		r.matches = nil
	} else {
		var rows []query.Series
		for _, s := range r.allRows {
			if matchesSeries(f, s) {
				rows = append(rows, s)
			}
		}
		m.table = ui.Table{Rows: rows}
		if m.chart.enabled {
			r.matches = make([]int, 0)
			for i, s := range m.chart.result.Series {
				if matchesSeries(f, s) {
					r.matches = append(r.matches, i)
				}
			}
		}
	}
	m.chart.revision++
}

func (m *Model) editResultFilter(msg tea.Msg) tea.Cmd {
	r := &m.resultFilter
	before := r.input.Value()
	var cmd tea.Cmd
	r.input, cmd = r.input.Update(msg)
	if r.input.Value() != before {
		m.refreshResultFilter()
	}
	return cmd
}

func (m *Model) sortResults(value bool) {
	if len(newFuzzyFilter(m.resultFilter.input.Value())) == 0 {
		m.table.Sort(value)
		return
	}
	t := ui.Table{Rows: m.resultFilter.allRows}
	t.Sort(value)
	m.refreshResultFilter()
}

func (m *Model) graphSeriesCount() int {
	if indices := m.resultFilter.matches; indices != nil {
		return len(indices)
	}
	return len(m.chart.visible)
}

func (m *Model) graphIndex(row int) int {
	if indices := m.resultFilter.matches; indices != nil {
		return indices[row]
	}
	return row
}

func (m *Model) graphRow() int {
	if indices := m.resultFilter.matches; indices != nil {
		i, _ := slices.BinarySearch(indices, m.chart.selected)
		if i < len(indices) && indices[i] == m.chart.selected {
			return i
		}
		return 0
	}
	return m.chart.selected
}
