package app

import (
	"errors"
	"fmt"
	"os"
	"strconv"
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
	m.mouseModal()
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
		if d.selected().kind != alertPickerNone {
			hints = append(hints, "Enter Apply", "↑↓ Select")
		}
		if len(d.items) != 0 || len(d.views) != 0 {
			hints = append(hints, "/ Search")
		}
		if d.selected().kind == alertPickerFilter {
			hints = append(hints, "e Rename", "u Update", "t Template", "d Delete")
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
		return "File changed. Reopen with Ctrl+O."
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
	title, meta := "Open — Alerts", ""
	var rows []string
	first, end, listY := 0, 0, 0
	var cursor *tea.Cursor
	text := func(s string) {
		rows = append(rows, dialogSurface.Render(" "+ansi.Truncate(ui.Plain(s), inner-2, "…")+" "))
	}
	input := func() {
		value, col := dialogInput(d.input, inner-2)
		if d.input.Focused() {
			cursor = nativeCursor(min(width-2, 2+col), 1+len(rows))
		}
		text(value)
	}
	var errorRows []string
	err := d.err
	if err == nil {
		err = d.openErr
	}
	if err != nil {
		errorRows = strings.Split(ansi.Hardwrap(alertSavedError(err), inner-2, false), "\n")
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
		case len(d.items) != 0 || len(d.views) != 0:
			text("/ Search filters and views")
		}
		if len(d.matches) == 0 && len(rows) < room {
			if len(d.items) == 0 && len(d.views) == 0 && err == nil {
				text("No saved alert filters yet")
				text("m Edit · Ctrl+S Save")
			} else if len(d.items) != 0 || len(d.views) != 0 {
				text("No matching filters or views")
			}
		}
		room = min(10, room-len(rows))
		first = dialogStart(d.row, len(d.matches), room)
		end, listY = min(first+room, len(d.matches)), len(rows)
		if len(d.matches) > 0 {
			meta = fmt.Sprintf("%d/%d", d.row+1, len(d.matches))
		}
		for i := first; i < min(first+room, len(d.matches)); i++ {
			item := d.matches[i]
			var label string
			switch item.kind {
			case alertPickerFilter:
				label = "FILTER  " + ui.Plain(d.items[item.filterIndex].Name)
			case alertPickerView:
				label = "VIEW    " + ui.Plain(item.viewName)
			}
			rows = append(rows, dialogRow(label, "", inner, i == d.row))
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
		first = dialogStart(d.row, len(d.editors), room)
		end, listY = min(first+room, len(d.editors)), len(rows)
		meta = fmt.Sprintf("%d/%d", d.row+1, len(d.editors))
		for i := first; i < min(first+room, len(d.editors)); i++ {
			r := &d.editors[i]
			l, lx := dialogInput(r.label, label)
			v, vx := dialogInput(r.value, value)
			op := fmt.Sprintf(" %-2s ", alertOperators[r.operator])
			if i == d.row {
				pos := 4
				switch d.field {
				case 0:
					cursor = nativeCursor(pos+lx, 1+len(rows))
				case 1:
					op = fmt.Sprintf("[%2s]", alertOperators[r.operator])
				case 2:
					cursor = nativeCursor(pos+label+5+vx, 1+len(rows))
				}
			}
			rows = append(rows, dialogRow(fixedMatcherColumn(l, label)+" "+op+fixedMatcherColumn(v, value), "", inner, i == d.row))
		}
	}
	for _, line := range errorRows {
		rows = append(rows, dialogSurface.Foreground(lipgloss.Color("11")).Render(" "+line+" "))
	}
	x, y := (m.width-width)/2, 1+(maxRows-len(rows))/2
	m.mouseDialog(x, y, width, len(rows), m.paneKey)
	switch d.mode {
	case alertSavedBrowse, alertSavedSearch:
		m.mouseRows(x+1, y+1+listY, inner, first, end, "alert picker", strconv.Itoa,
			func(i int) tea.Cmd { d.row = i; return nil }, nil)
	case alertSavedManual:
		m.mouseRows(x+1, y+1+listY, inner, first, end, "matchers", strconv.Itoa,
			func(i int) tea.Cmd { return d.focusMatcher(i, d.field) }, nil)
	}
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
