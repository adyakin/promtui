package app

import (
	"cmp"
	"image/color"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"promtui/internal/config"
	"promtui/internal/ui"
)

const (
	alertTablePrefix   = 2 // Selection marker and padding inside the frame.
	alertTableSamples  = 64
	alertTableWidthCap = 32 // Maximum influence of any sampled value.
)

type alertTableColumn struct {
	config.Column
	width, minimum, preferred int
	weight                    float64
}

type alertTable struct {
	width, trailing int
	clipped         bool
	gap             string
	cols            []alertTableColumn
}

// Re-sample on resize, view change or refresh, not on every repaint or scroll.
func (a *alertsState) tableForWidth(width int, now time.Time) alertTable {
	if a.cachedTable.width != width {
		a.cachedTable = a.tableLayout(width, now)
	}
	return a.cachedTable
}

// width is the frame's interior width, excluding borders. Alerts uses a
// position counter in the frame header rather than an in-body scrollbar.
func (a *alertsState) tableLayout(width int, now time.Time) alertTable {
	t := alertTable{width: max(0, width)}
	room := max(0, width-alertTablePrefix)
	columns := a.view.Columns
	defaultView := len(columns) == 0
	narrowDefault := defaultView && width < 43
	if defaultView {
		var defaults [len(defaultAlertFields)]config.Column
		for i, field := range defaultAlertFields {
			defaults[i].Field = field
		}
		columns = defaults[:]
		if narrowDefault {
			// Keep State and Alert identity legible instead of squeezing five
			// columns into the smallest supported terminals.
			columns = []config.Column{defaults[0], defaults[2]}
		}
	}
	if len(columns) > 1 && room < 4*len(columns)-1 {
		// Preserve at least three cells per retained column and room for a
		// separator. Mark omitted columns without squeezing every value to ….
		if room >= 5 {
			t.clipped = true
			room -= 2
		}
		columns = columns[:min(len(columns), max(1, (room+1)/4))]
	}
	columns = columns[:min(len(columns), room)]
	if len(columns) == 0 {
		t.trailing = room
		return t
	}
	t.cols = make([]alertTableColumn, len(columns))
	minimum := 0
	for i, column := range columns {
		c := &t.cols[i]
		*c = sizeAlertTableColumn(column, a.rows, now)
		if t.clipped {
			floor := 3
			if c.Width > 0 {
				floor = min(floor, c.Width)
			}
			c.minimum = max(c.minimum, floor)
		}
		if defaultView && width < 76 && column.Field == "state" && c.preferred > 11 {
			c.preferred = 11
		}
		minimum += c.minimum
	}

	// Drop separator padding before dropping columns, then share whatever
	// remains among the leftmost columns if their minimums cannot all fit.
	gap := 0
	if gaps := len(columns) - 1; gaps > 0 {
		gap = min(2, (room-len(columns))/gaps)
		for gap > 0 && minimum+gap*gaps > room {
			gap--
		}
		// Even when headings must truncate, keep cells distinguishable if
		// that does not reduce them below two cells apiece.
		if gap == 0 && room >= 2*len(columns)+gaps {
			gap = 1
		}
		room -= gap * gaps
	}
	t.gap = strings.Repeat(" ", gap)
	room -= len(columns) // Every retained column starts with one cell.

	// Give every column a chance at a usable width first. Fixed and compact
	// columns then get their preferred widths before flexible columns grow.
	room = growAlertTableColumns(t.cols, room, func(c alertTableColumn) int { return c.minimum })
	room = growAlertTableColumns(t.cols, room, func(c alertTableColumn) int { return c.preferred })
	t.trailing = shareAlertTableSpace(t.cols, room)
	return t
}

func sizeAlertTableColumn(column config.Column, rows []alertRow, now time.Time) alertTableColumn {
	column.Title = ui.Plain(columnTitle(config.AlertsView, column))
	if column.Title == "" {
		_, name, _ := strings.Cut(column.Field, ".")
		column.Title = ui.Plain(strings.ToUpper(name))
	}
	heading := max(1, ansi.StringWidth(column.Title))
	c := alertTableColumn{Column: column, width: 1, minimum: min(heading, 8)}
	if c.Width > 0 {
		c.minimum = min(c.minimum, c.Width)
		c.preferred = c.Width
		return c
	}
	if c.Weight > 0 {
		c.weight = float64(c.Weight)
		return c
	}
	switch c.Field {
	case "state":
		// Reserve enough space for the full combined state on roomy tables,
		// even if that state did not occur in the sample.
		c.preferred = max(heading, len("Silenced + Inhibited"))
	case "duration":
		// Do not let a ticking elapsed time change the layout.
		c.preferred = max(heading, 8)
	case "severity", "labels.severity":
		c.preferred = max(heading, min(12, alertTableContentWidth(c.Column, rows, now)))
	default:
		// Automatic columns compete for flexible space by typical content
		// width. Keep these weights in the same range as common explicit
		// weights, so even a very long typical value cannot take over.
		c.weight = float64(max(8, min(alertTableWidthCap, max(heading, alertTableContentWidth(c.Column, rows, now))))) / 8
	}
	return c
}

func alertTableContentWidth(column config.Column, rows []alertRow, now time.Time) int {
	var widths [alertTableSamples]int
	n := min(len(rows), len(widths))
	if n == 0 {
		return 0
	}
	for i := range n {
		index := 0
		if n > 1 {
			// Sample evenly across the loaded snapshot, including both ends.
			index = i * (len(rows) - 1) / (n - 1)
		}
		value := rows[index].columnValue(column, now)
		// Bound work spent measuring oversized strings (after extraction).
		// Back up to a rune boundary before measuring the prefix.
		end := min(len(value), 256)
		for end < len(value) && end > 0 && !utf8.RuneStart(value[end]) {
			end--
		}
		widths[i] = min(alertTableWidthCap, ansi.StringWidth(ui.Plain(value[:end])))
	}
	slices.Sort(widths[:n])
	return widths[(n-1)*3/4]
}

// Grow equally towards the given target, returning the unassigned cells.
func growAlertTableColumns(cols []alertTableColumn, room int, target func(alertTableColumn) int) int {
	for room > 0 {
		start := room
		for i := range cols {
			if room == 0 {
				break
			}
			if c := &cols[i]; c.width < target(*c) {
				c.width++
				room--
			}
		}
		if start == room {
			break
		}
	}
	return room
}

func shareAlertTableSpace(cols []alertTableColumn, room int) int {
	if room == 0 {
		return 0
	}
	total := 0.0
	for _, c := range cols {
		total += c.weight
	}
	if total == 0 {
		return room // No flexible columns; leave fixed and compact widths alone.
	}
	type remainder struct {
		index int
		part  float64
	}
	var remainders []remainder
	left := room
	for i := range cols {
		if c := &cols[i]; c.weight > 0 {
			share := float64(room) * (c.weight / total)
			cells := min(left, int(share))
			c.width += cells
			left -= cells
			remainders = append(remainders, remainder{i, share - float64(cells)})
		}
	}
	// Resolve ties by column order for a deterministic result.
	slices.SortStableFunc(remainders, func(a, b remainder) int {
		return cmp.Compare(b.part, a.part)
	})
	for _, r := range remainders[:min(left, len(remainders))] {
		cols[r.index].width++
		left--
	}
	return left
}

func (t alertTable) header() string {
	if t.width < alertTablePrefix {
		return strings.Repeat(" ", t.width)
	}
	var line strings.Builder
	line.WriteString("  ")
	for i, c := range t.cols {
		if i > 0 {
			line.WriteString(t.gap)
		}
		line.WriteString(alertColumn(c.Title, c.width))
	}
	line.WriteString(strings.Repeat(" ", t.trailing))
	if t.clipped {
		line.WriteString(" …")
	}
	return title.Render(line.String())
}

func (t alertTable) row(selected bool, row alertRow, now time.Time) string {
	if t.width < alertTablePrefix {
		return ansi.Truncate(renderStatusRow(alertTablePrefix, selected, "", "", "", nil), t.width, "")
	}
	var before, after strings.Builder
	part := &before
	var cell string
	var tint color.Color
	for i, c := range t.cols {
		if i > 0 {
			part.WriteString(t.gap)
		}
		value := row.columnValue(c.Column, now)
		if c.Field == "state" && value == "Silenced + Inhibited" && c.width < len(value) {
			value = "Sil.+Inhib."
		}
		text := alertColumn(alertValue(value), c.width)
		if tint == nil && value != "" && (c.Field == "severity" || c.Field == "labels.severity") {
			if tint = alertSeverityTint(row.Labels["severity"]); tint != nil {
				cell, part = text, &after
				continue
			}
		}
		part.WriteString(text)
	}
	part.WriteString(strings.Repeat(" ", t.trailing))
	if t.clipped {
		part.WriteString("  ")
	}
	return renderStatusRow(t.width, selected, before.String(), cell, after.String(), tint)
}
