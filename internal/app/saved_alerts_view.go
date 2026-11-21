package app

import (
	"errors"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/saveditems"
	"promtui/internal/ui"
)

func (m *Model) alertSavedWidth() int {
	maxWidth := 56
	switch m.alerts.saved.mode {
	case alertSavedManual:
		maxWidth = 72
	case alertSavedTemplate:
		maxWidth = 76
	case alertSavedName, alertSavedConfirm:
		maxWidth = 48
	}
	return min(maxWidth, m.width-2)
}

func matcherWidths(width int) (label, value int) {
	label = max(4, (width-11)/3)
	return label, max(1, width-label-11)
}

func (m *Model) sizeAlertSavedInputs() {
	d := &m.alerts.saved
	width := max(minWidth-2, m.alertSavedWidth())
	switch d.mode {
	case alertSavedName, alertSavedSearch:
		d.input.SetWidth(max(1, width-5-ansi.StringWidth(d.input.Prompt)))
		d.input.SetCursor(d.input.Position())
	case alertSavedManual:
		label, value := matcherWidths(width)
		for i := range d.editors {
			r := &d.editors[i]
			r.label.SetWidth(label)
			r.value.SetWidth(value)
			r.label.SetCursor(r.label.Position())
			r.value.SetCursor(r.value.Position())
		}
	case alertSavedTemplate:
		d.template.resize(width, m.height)
	}
}

func (m *Model) alertSavedFooter() string {
	d := &m.alerts.saved
	hints, back := []string{}, "Esc Cancel"
	switch d.mode {
	case alertSavedBrowse:
		back = "Esc Close"
		if d.openErr == nil {
			if d.selected() >= 0 {
				hints = append(hints, "Enter Load", "↑↓ Select", "e Rename", "u Update", "t Template", "d Delete")
			}
			if len(d.items) != 0 {
				hints = append(hints, "/ Search")
			}
		}
	case alertSavedSearch:
		hints = []string{"Enter Done", "↑↓ Select"}
	case alertSavedName:
		if d.input.Focused() {
			hints = []string{"Enter " + d.action.label()}
		}
	case alertSavedConfirm:
		hints = []string{"Enter " + d.action.label()}
	case alertSavedManual:
		hints = []string{"Ctrl+Enter Apply", "Tab Field", "←→ Operator", "Ctrl+N Add", "Ctrl+D Remove"}
		if m.width < 30 {
			hints[0] = "^Enter OK"
		}
	case alertSavedTemplate:
		hints = []string{"Ctrl+Enter Save"}
		if m.width < 30 {
			hints[0] = "^Enter OK"
		}
	}
	return m.formatFooter(hints, back, "  ")
}

func alertSavedError(err error) string {
	var pathErr *os.PathError
	var linkErr *os.LinkError
	switch {
	case errors.Is(err, saveditems.ErrChanged):
		return "File changed. Reopen Saved Alerts."
	case errors.As(err, &pathErr):
		return fmt.Sprintf("alerts.yaml: %s: %v", pathErr.Op, pathErr.Err)
	case errors.As(err, &linkErr):
		return fmt.Sprintf("alerts.yaml: %s: %v", linkErr.Op, linkErr.Err)
	default:
		return ui.Plain(err.Error())
	}
}

func (m *Model) alertSavedLayer() (*lipgloss.Layer, *tea.Cursor) {
	d := &m.alerts.saved
	width := m.alertSavedWidth()
	if d.mode == alertSavedTemplate {
		return m.savedTemplateOverlay(d.template, "Edit Alert Filter Template", d.items[d.target].Name, d.err, width)
	}
	inner, maxRows := width-2, m.height-5
	title, meta := "Saved Alerts", ""
	var rows []string
	var cursor *tea.Cursor
	text := func(s string) {
		rows = append(rows, dialogSurface.Render(" "+ansi.Truncate(ui.Plain(s), inner-2, "…")+" "))
	}
	input := func() {
		if c := d.input.Cursor(); c != nil {
			cursor = nativeCursor(min(width-2, 2+c.X), 1+len(rows))
		}
		text(ansi.Strip(d.input.View()))
	}
	var errorRows []string
	if d.err != nil {
		errorRows = strings.Split(ansi.Hardwrap(alertSavedError(d.err), inner-2, false), "\n")
		limit := min(3, len(errorRows))
		if limit < len(errorRows) {
			errorRows[limit-1] = ansi.Truncate(errorRows[limit-1], inner-3, "") + "…"
		}
		errorRows = errorRows[:limit]
	}
	room := maxRows - len(errorRows)
	switch d.mode {
	case alertSavedBrowse, alertSavedSearch:
		switch {
		case d.mode == alertSavedSearch:
			input()
		case d.filter != "":
			text("Search: " + d.filter)
		}
		if d.err == nil && len(d.matches) == 0 {
			if len(d.items) == 0 {
				text("No saved alert filters yet")
				text("m Edit · Ctrl+S Save")
			} else {
				text("No matching filters")
			}
		}
		room = min(10, room-len(rows))
		first := dialogStart(d.row, len(d.matches), room)
		if len(d.matches) > 0 {
			meta = fmt.Sprintf("%d/%d", d.row+1, len(d.matches))
		}
		for i := first; i < min(first+room, len(d.matches)); i++ {
			rows = append(rows, dialogRow(ui.Plain(d.items[d.matches[i]].Name), "", inner, i == d.row))
		}
	case alertSavedName:
		title = d.action.label() + " Alert Filter"
		if d.input.Focused() {
			input()
		}
	case alertSavedConfirm:
		title = d.action.label() + " Alert Filter"
		message := "Delete this saved filter?"
		if d.action == updateSaved {
			message = "Replace with active label matchers?"
		}
		text(message)
		text(fmt.Sprintf("%q", d.items[d.target].Name))
	case alertSavedManual:
		title = "Alert label matchers"
		label, value := matcherWidths(width)
		if room >= 3 {
			text("Tab: field · Ctrl+N: add · Ctrl+D: remove")
		}
		room -= len(rows)
		first := dialogStart(d.row, len(d.editors), room)
		meta = fmt.Sprintf("%d/%d", d.row+1, len(d.editors))
		for i := first; i < min(first+room, len(d.editors)); i++ {
			r := &d.editors[i]
			l := fixedMatcherColumn(r.label.View(), label)
			v := fixedMatcherColumn(r.value.View(), value)
			op := fmt.Sprintf(" %-2s ", alertOperators[r.operator])
			if i == d.row {
				pos := 4
				switch d.field {
				case 0:
					if c := r.label.Cursor(); c != nil {
						cursor = nativeCursor(pos+min(c.X, label-1), 1+len(rows))
					}
				case 1:
					op = fmt.Sprintf("[%2s]", alertOperators[r.operator])
				case 2:
					if c := r.value.Cursor(); c != nil {
						cursor = nativeCursor(pos+label+5+min(c.X, value-1), 1+len(rows))
					}
				}
			}
			rows = append(rows, dialogRow(l+" "+op+v, "", inner, i == d.row))
		}
	}
	for _, line := range errorRows {
		rows = append(rows, dialogSurface.Foreground(lipgloss.Color("11")).Render(" "+line+" "))
	}
	x, y := (m.width-width)/2, 1+(maxRows-len(rows))/2
	layer := lipgloss.NewLayer(renderFrame(title, meta, rows, width, true, dialogSurface)).X(x).Y(y).Z(1)
	if cursor != nil && !m.quitting {
		cursor.X += x
		cursor.Y += y
		return layer, cursor
	}
	return layer, nil
}

func fixedMatcherColumn(s string, width int) string {
	s = ansi.Truncate(ui.Plain(ansi.Strip(s)), width, "…")
	return s + strings.Repeat(" ", width-ansi.StringWidth(s))
}
