package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
)

type targetLabelsCopied struct {
	dialogID int
	err      error
}

func (m *Model) targetsKey(k tea.KeyPressMsg) tea.Cmd {
	a := &m.targets
	key := k.String()
	if k.Keystroke() == "alt+s" && a.dialog != targetSources {
		if a.dialog == targetStatus {
			a.dialog = noTargetDialog
		} else {
			a.dialog, a.dialogRow = targetStatus, 0
		}
		return nil
	}
	if a.input.Focused() && a.dialog == noTargetDialog {
		switch key {
		case "enter":
			a.filter = a.input.Value()
			a.input.Blur()
		case "esc":
			a.input.Blur()
			a.rebuild(a.anchor, a.anchorRow)
		default:
			return a.edit(k)
		}
		return nil
	}
	switch a.dialog {
	case targetSources:
		switch key {
		case "esc":
			a.dialog = noTargetDialog
		case "enter":
			id := a.selection()
			a.source, a.dialog = a.dialogRow, noTargetDialog
			a.errorOffset = 0
			a.rebuild(id, 0)
		default:
			a.dialogRow = targetNavigate(key, a.dialogRow, len(a.sources)+1, m.targetSourcePage())
		}
		return nil
	case targetDetails, targetHelp, targetStatus:
		switch key {
		case "esc":
			a.dialog = noTargetDialog
		case "?":
			if a.dialog == targetHelp {
				a.dialog = noTargetDialog
			} else {
				a.dialog, a.dialogRow = targetHelp, 0
			}
		case "y":
			if a.dialog == targetDetails && len(a.detail.Labels) > 0 {
				text, id := strings.Join(targetLabels(a.detail.Labels), "\n"), a.dialogID
				return func() tea.Msg { return targetLabelsCopied{id, clipboard.WriteAll(text)} }
			}
		case "up", "down", "pgup", "pgdown", "home", "end":
			_, rows, _, room := m.targetTextDialog()
			a.dialogRow = targetNavigate(key, min(a.dialogRow, max(0, len(rows)-room)), len(rows)-room+1, room)
		}
		return nil
	}
	if a.hasError() {
		switch key {
		case "up", "down", "pgup", "pgdown", "home", "end":
			a.errorOffset = scrollPaneError(key, a.errorOffset, len(m.targetErrorRows()), max(1, m.height-5))
		case "f":
			a.dialog, a.dialogRow = targetSources, a.source
		case "r":
			return m.refreshTargets()
		case "?":
			a.dialog, a.dialogRow = targetHelp, 0
		case "q":
			return m.act("quit", "", 0)
		}
		return nil
	}
	id := a.selection()
	switch key {
	case "enter":
		if len(a.visible) > 0 {
			a.dialog, a.dialogRow, a.detail, a.copyStatus = targetDetails, 0, *a.visible[a.row], ""
			a.dialogID++
		}
	case "/":
		if len(a.sources) > 0 {
			a.anchor, a.anchorRow = id, a.row
			a.input.SetValue(a.filter)
			a.input.CursorEnd()
			return a.input.Focus()
		}
	case "f":
		if len(a.sources) > 0 {
			a.dialog, a.dialogRow = targetSources, a.source
		}
	case "h":
		a.health = (a.health + 1) % len(targetHealthOrder)
		if targetHealthOrder[a.health] != targetDropped {
			a.activeHealth = a.health
		}
		a.rebuild(id, 0)
	case "d":
		if targetHealthOrder[a.health] == targetDropped {
			a.health = a.activeHealth
		} else {
			a.activeHealth, a.health = a.health, len(targetHealthOrder)-1
		}
		a.rebuild(id, 0)
	case "s":
		a.sort = (a.sort + 1) % targetSort(len(targetSortNames))
		a.rebuild(id, a.row)
	case "r":
		return m.refreshTargets()
	case "?":
		a.dialog, a.dialogRow = targetHelp, 0
	case "q":
		return m.act("quit", "", 0)
	default:
		a.row = targetNavigate(key, a.row, len(a.visible), m.targetPage())
	}
	return nil
}

func targetNavigate(key string, row, length, page int) int {
	switch key {
	case "up":
		row--
	case "down":
		row++
	case "pgup":
		row -= page
	case "pgdown":
		row += page
	case "home":
		row = 0
	case "end":
		row = length - 1
	}
	return max(0, min(row, length-1))
}
