package app

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	"github.com/charmbracelet/x/ansi"
)

func TestDialogInputViewportUsesDisplayCells(t *testing.T) {
	input := textinput.New()
	input.Prompt = ""
	input.SetValue(strings.Repeat("x", 25) + "雪ab" + strings.Repeat("y", 25))
	input.SetCursor(26)
	value, column := dialogInput(input, 8)
	if column != 7 || !strings.HasPrefix(value, "xxxxx雪a") || ansi.StringWidth(value) != 8 {
		t.Fatalf("viewport %q at %d", value, column)
	}
	input.SetCursor(input.Position() - 1)
	earlier, column := dialogInput(input, 8)
	if column != 7 || earlier != "xxxxxxx" {
		t.Fatalf("caret moved to the wrong cell: %q at %d", earlier, column)
	}
	input.SetCursor(26)
	narrow, column := dialogInput(input, 4)
	if narrow != "x雪a" || column != 3 {
		t.Fatalf("resized viewport %q at %d", narrow, column)
	}
}
