package ui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"promtui/internal/query"
)

// The table indexes series rather than materializing a row for every range sample.
type Table struct {
	Rows         []query.Series
	Row, Back, X int // Back counts from the newest sample in the selected series.
	Labels       bool
}

func Plain(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}

func (t *Table) Move(n int) {
	if len(t.Rows) > 0 {
		t.Row = min(max(t.Row+n, 0), len(t.Rows)-1)
		t.Back = 0
	}
}
func (t *Table) Sample(n int) {
	if len(t.Rows) > 0 {
		t.Back = min(max(t.Back+n, 0), len(t.Rows[t.Row].Samples)-1)
	}
}
func (t *Table) Sort(value bool) {
	sort.SliceStable(t.Rows, func(i, j int) bool {
		a, b := t.Rows[i], t.Rows[j]
		if value {
			x, y := float64(a.Samples[len(a.Samples)-1].Value), float64(b.Samples[len(b.Samples)-1].Value)
			if math.IsNaN(x) != math.IsNaN(y) {
				return math.IsNaN(y)
			}
			if x != y && !math.IsNaN(x) {
				return x > y
			}
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Labels < b.Labels
	})
	t.Row, t.Back = 0, 0
}
func (t Table) Fields(i int) (string, string, string) {
	s := t.Rows[i]
	j := len(s.Samples) - 1
	if i == t.Row {
		j -= t.Back
	}
	p := s.Samples[j]
	v := p.Value.String()
	if s.Text != nil {
		v = *s.Text
	}
	return p.Timestamp.Time().UTC().Format("2006-01-02 15:04:05.000"), v, fmt.Sprintf("%d/%d", j+1, len(s.Samples))
}
func (t Table) View(w, h int) string {
	var lines []string
	line := func(prefix, text string) { lines = append(lines, prefix+ansi.Cut(Plain(text), t.X, t.X+max(1, w-2))) }
	line("  ", fmt.Sprintf("%-23s  %-12s  %-11s  %s", "UTC", "Value", "Sample", "Series"))
	first := max(0, t.Row-h+2)
	for i := first; i < min(first+h-1, len(t.Rows)); i++ {
		ts, v, idx := t.Fields(i)
		label := t.Rows[i].Name
		if t.Labels {
			label = t.Rows[i].Labels
		}
		prefix := "  "
		if i == t.Row {
			prefix = "› "
		}
		line(prefix, fmt.Sprintf("%s  %-12s  %-11s  %s", ts, v, idx, label))
	}
	return strings.Join(lines, "\n")
}
