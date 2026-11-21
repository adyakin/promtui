package app

import (
	"charm.land/bubbles/v2/textinput"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/ui"
)

// Render the value and caret from the same display-cell viewport. Bubbles'
// native Cursor reports an absolute rune index even when its View scrolls.
func dialogInput(input textinput.Model, width int) (string, int) {
	prompt := ui.Plain(input.Prompt)
	room := max(1, width-ansi.StringWidth(prompt))
	value := []rune(ui.Plain(input.Value()))
	pos := min(input.Position(), len(value))
	if len(value) == 0 {
		return ansi.Truncate(prompt+ui.Plain(input.Placeholder), width, "…"), ansi.StringWidth(prompt)
	}
	first := 0
	col := ansi.StringWidth(string(value[:pos]))
	if col >= room {
		// Leave a cursor cell on the right and start on a rune boundary
		// rather than clipping half of a wide character.
		first = pos
		col = 0
		for first > 0 {
			w := ansi.StringWidth(string(value[first-1 : first]))
			if col+w >= room {
				break
			}
			first--
			col += w
		}
		col = ansi.StringWidth(string(value[first:pos]))
	}
	return prompt + ansi.Truncate(string(value[first:]), room, ""), ansi.StringWidth(prompt) + col
}
