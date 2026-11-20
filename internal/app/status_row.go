package app

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	statusRed        = lipgloss.Color("1")
	statusYellow     = lipgloss.Color("3")
	selectedRowStyle = lipgloss.NewStyle().Reverse(true)
)

func renderStatusRow(width int, selected bool, before, cell, after string, tint color.Color) string {
	prefix := "  "
	if selected {
		prefix = "› "
	}
	before = prefix + before
	if tint == nil {
		line := before + cell + after
		if selected {
			return selectedRowStyle.Width(width).Render(line)
		}
		return line
	}

	label := strings.TrimRight(cell, " ")
	after = cell[len(label):] + after
	if selected {
		after += strings.Repeat(" ", max(0, width-ansi.StringWidth(before+label+after)))
		// Reverse video swaps the foreground and background. Set the logical
		// background on the label so only its visible foreground changes.
		return selectedRowStyle.Render(before) + selectedRowStyle.Background(tint).Render(label) + selectedRowStyle.Render(after)
	}
	return before + lipgloss.NewStyle().Foreground(tint).Render(label) + after
}
