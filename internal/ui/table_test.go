package ui

import (
	"strings"
	"testing"

	"github.com/prometheus/common/model"
	"promtui/internal/query"
)

func TestSelectedSeriesAndSamples(t *testing.T) {
	table := Table{Rows: []query.Series{
		{Name: "z", Samples: []model.SamplePair{{Value: 11}, {Value: 7}}},
		{Name: "a", Samples: []model.SamplePair{{Value: 2}}},
	}}
	table.Sample(9)
	_, v, index := table.Fields(0)
	if v != "11" || index != "1/2" {
		t.Fatal(v, index)
	}
	table.Move(99)
	if !strings.Contains(table.View(80, 2), "a") || strings.Contains(table.View(80, 2), "1/2") {
		t.Fatal(table.View(80, 2))
	}
	table.Sort(false)
	if table.Rows[0].Name != "a" {
		t.Fatal(table.Rows)
	}
	table.Sort(true)
	if table.Rows[0].Name != "z" || table.Back != 0 {
		t.Fatal(table)
	}
}
