package ui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/query"
)

// The table indexes series rather than materializing a row for every range sample.
type Table struct {
	Rows         []query.Series
	Row, Back, X int // Back counts from the newest sample in the selected series.
	Expanded     bool
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
		if row := min(max(t.Row+n, 0), len(t.Rows)-1); row != t.Row {
			t.Row, t.Back, t.X, t.Expanded = row, 0, 0, false
		}
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
	t.Row, t.Back, t.X, t.Expanded = 0, 0, 0, false
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
	if h < 1 || len(t.Rows) == 0 {
		return ""
	}
	t.Pan(0, w)
	var lines []string
	line := func(prefix, text string, selected bool) {
		s := prefix + ansi.Cut(Plain(text), t.X, t.X+max(1, w-2))
		if selected {
			s = lipgloss.NewStyle().Reverse(true).Width(w).Render(s)
		}
		lines = append(lines, s)
	}
	room := h
	if h >= 2 {
		line("  ", t.columns(w, "Value", "Series", "Sample", "UTC"), false)
		room--
	}
	detail := 0
	if t.Expanded {
		detail = min(room-1, 4, 1+len(t.Rows[t.Row].Samples))
	}
	room -= detail
	first := max(0, t.Row-room+1)
	for i := first; i < min(first+room, len(t.Rows)); i++ {
		prefix := "  "
		if i == t.Row {
			prefix = "› "
		}
		line(prefix, t.rowText(w, i), i == t.Row)
		if i == t.Row && detail > 0 {
			line("  ", "Labels: "+t.Rows[i].Labels, false)
			samples := t.Rows[i].Samples
			copy := t
			for n := 0; n < detail-1; n++ {
				copy.Back = min(t.Back, len(samples)-detail+1) + n
				ts, v, idx := copy.Fields(i)
				mark := "  "
				if copy.Back == t.Back {
					mark = "· "
				}
				line(mark, fmt.Sprintf("%s  %s  %s", v, idx, ts), false)
			}
		}
	}
	return strings.Join(lines, "\n")
}

func (t *Table) Pan(delta, w int) {
	if len(t.Rows) == 0 {
		return
	}
	extent := ansi.StringWidth(Plain(t.rowText(w, t.Row)))
	if t.Expanded {
		extent = max(extent, ansi.StringWidth(Plain("Labels: "+t.Rows[t.Row].Labels)))
	}
	t.X = min(max(0, t.X+delta), max(0, extent-w+2))
}

func (t Table) rowText(w, i int) string {
	ts, value, sample := t.Fields(i)
	label := t.Rows[i].Labels
	if label == "" || label == "{}" {
		label = t.Rows[i].Name
	}
	return t.columns(w, value, label, sample, ts)
}

func (t Table) columns(w int, value, label, sample, ts string) string {
	pad := func(s string, n int) string {
		s = ansi.Truncate(Plain(s), n, "…")
		return s + strings.Repeat(" ", n-ansi.StringWidth(s))
	}
	valueWidth := min(12, max(6, w/4))
	labelWidth := max(12, w-valueWidth-36)
	return pad(value, valueWidth) + "  " + pad(label, labelWidth) + "  " + pad(sample, 8) + "  " + ts
}
