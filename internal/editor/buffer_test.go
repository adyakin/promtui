package editor

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

func TestExactBufferPreservesControlsAndLiteralMarkers(t *testing.T) {
	b := Buffer{Model: textarea.New()}
	const raw = "up\t+\r\nmetric{job=\"�\U000f0000\"}"
	b.SetExactValue(raw)
	b.Focus()
	if b.Value() != raw || Position(b) != len(raw) {
		t.Fatalf("exact set = %q, cursor=%d", b.Value(), Position(b))
	}
	rows, _, _ := View(b, 80)
	if rendered := strings.Join(rows, "\n"); strings.ContainsAny(rendered, "\t\r\U000f0001\U000f0002") {
		t.Fatalf("internal representation leaked into view: %q", rendered)
	}
	for marker := range b.encoded {
		char := string(marker)
		b, _ = b.Update(tea.KeyPressMsg{Code: marker, Text: char})
		if b.Value() != raw+char {
			t.Fatalf("typing a private-use rune was interpreted as a control: %q", b.Value())
		}
		break
	}
}
