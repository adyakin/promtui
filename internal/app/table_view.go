package app

import (
	"cmp"
	"image/color"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"promtui/internal/config"
	"promtui/internal/ui"
)

type viewSort struct {
	field, title string
}

func columnTitle(kind config.ViewKind, col config.Column) string {
	if col.Title != "" {
		return col.Title
	}
	return kind.FieldTitle(col.Field)
}

func viewSorts(kind config.ViewKind, defaults []viewSort, cols []config.Column) []viewSort {
	if len(cols) == 0 {
		return defaults
	}
	modes := slices.Clone(defaults)
	seen := make(map[string]bool, len(defaults)+len(cols))
	for _, s := range defaults {
		seen[s.field] = true
	}
	for _, col := range cols {
		if !seen[col.Field] {
			modes = append(modes, viewSort{col.Field, columnTitle(kind, col)})
			seen[col.Field] = true
		}
	}
	return modes
}

func nextViewSort(modes []viewSort, field string) string {
	i := slices.IndexFunc(modes, func(s viewSort) bool { return s.field == field })
	return modes[(i+1)%len(modes)].field
}

func viewSortTitle(modes []viewSort, field string) string {
	if i := slices.IndexFunc(modes, func(s viewSort) bool { return s.field == field }); i >= 0 {
		return ui.Plain(modes[i].title)
	}
	return ""
}

func matchesFields(f fuzzyFilter, fields []string, value func(string) string) bool {
	if len(f) == 0 {
		return true
	}
	for _, field := range fields {
		if f.matches(value(field)) {
			return true
		}
	}
	return false
}

func missingLast(a, b bool) int {
	if a == b {
		return 0
	}
	if a {
		return 1
	}
	return -1
}

func compareFieldText(a, b string) int {
	return cmp.Or(missingLast(a == "", b == ""), cmp.Compare(a, b))
}

type viewColumn struct {
	config.Column
	width, headingWidth int
}

type viewTable struct {
	width int
	gap   string
	cols  []viewColumn
}

func layoutViewTable(kind config.ViewKind, columns []config.Column, width int) viewTable {
	// Keep the leftmost columns when there aren't enough cells for even one
	// cell per column. Otherwise reduce spacing before clipping any columns.
	room := max(0, width-2)
	columns = columns[:min(room, len(columns))]
	table := viewTable{width: width}
	if len(columns) == 0 {
		return table
	}
	headingRoom := 0
	for _, c := range columns {
		c.Title = columnTitle(kind, c)
		n := max(1, ansi.StringWidth(ui.Plain(c.Title)))
		headingRoom += n
		table.cols = append(table.cols, viewColumn{Column: c, width: 1, headingWidth: n})
	}
	// Prefer two spaces when headings fit, otherwise reserve at most one.
	gap := min(2, max(0, room-headingRoom)/max(1, len(columns)-1))
	if gap == 0 && room >= 2*len(columns)-1 {
		gap = 1
	}
	table.gap = strings.Repeat(" ", gap)
	room -= gap*(len(columns)-1) + len(columns)
	// Share space fairly until headings fit, then share the surplus among values.
	for room > 0 {
		start := room
		for i := range table.cols {
			c := &table.cols[i]
			if room > 0 && c.width < c.headingWidth {
				c.width++
				room--
			}
		}
		if start == room {
			break
		}
	}
	for i := range table.cols {
		table.cols[i].width += room / len(columns)
		if i < room%len(columns) {
			table.cols[i].width++
		}
	}
	return table
}

func (t viewTable) header() string {
	var line strings.Builder
	line.WriteString("  ")
	for i, c := range t.cols {
		if i > 0 {
			line.WriteString(t.gap)
		}
		line.WriteString(alertColumn(c.Title, c.width))
	}
	return title.Render(line.String())
}

func (t viewTable) row(selected bool, resolve func(string, int) (string, color.Color)) string {
	var before, after strings.Builder
	part := &before
	var cell string
	var tint color.Color
	for i, c := range t.cols {
		if i > 0 {
			part.WriteString(t.gap)
		}
		value, fg := resolve(c.Field, c.width)
		text := alertColumn(alertValue(value), c.width)
		if fg != nil && tint == nil {
			cell, tint, part = text, fg, &after
		} else {
			part.WriteString(text)
		}
	}
	return renderStatusRow(t.width, selected, before.String(), cell, after.String(), tint)
}
