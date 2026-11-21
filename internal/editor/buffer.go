package editor

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

// Buffer uses the standard textarea for editing. Exact saved templates may
// contain tabs, CRs or other runes that the textarea sanitizes. Store those
// as distinct, private-use runes internally and decode on read; the app's
// own renderer never draws the internal representation.
type Buffer struct {
	textarea.Model
	encoded map[rune]rune
}

func (b *Buffer) SetValue(value string) {
	b.encoded = nil
	b.Model.SetValue(value)
}

func (b *Buffer) SetExactValue(value string) {
	needsEncoding := func(r rune) bool { return r != '\n' && (r == utf8.RuneError || unicode.IsControl(r)) }
	if strings.IndexFunc(value, needsEncoding) < 0 {
		b.SetValue(value)
		return
	}
	b.encoded = make(map[rune]rune)
	used := make(map[rune]bool)
	for _, r := range value {
		used[r] = true
	}
	replacements := map[rune]rune{}
	next := rune(0xf0000)
	buffer := strings.Map(func(r rune) rune {
		if !needsEncoding(r) {
			return r
		}
		if substitute, ok := replacements[r]; ok {
			return substitute
		}
		for used[next] {
			next++
		}
		replacements[r], b.encoded[next] = next, r
		next++
		return next - 1
	}, value)
	b.Model.SetValue(buffer)
}

func (b Buffer) Value() string {
	value := b.Model.Value()
	if len(b.encoded) == 0 {
		return value
	}
	return strings.Map(func(r rune) rune {
		if decoded, ok := b.encoded[r]; ok {
			return decoded
		}
		return r
	}, value)
}

func (b Buffer) Update(msg tea.Msg) (Buffer, tea.Cmd) {
	// Encode literal copies of any internal marker as themselves before
	// inserting them. Other text follows the existing textarea behavior.
	switch v := msg.(type) {
	case tea.KeyPressMsg:
		changed := b.encodeInput(v.Text)
		if changed != v.Text {
			msg = tea.PasteMsg{Content: changed}
		}
	case tea.PasteMsg:
		v.Content = b.encodeInput(v.Content)
		msg = v
	}
	var cmd tea.Cmd
	b.Model, cmd = b.Model.Update(msg)
	return b, cmd
}

func (b *Buffer) encodeInput(input string) string {
	if len(b.encoded) == 0 {
		return input
	}
	var used map[rune]bool
	next := rune(0xf0000)
	return strings.Map(func(r rune) rune {
		decoded, ok := b.encoded[r]
		if !ok || decoded == r {
			return r
		}
		if used == nil {
			used = make(map[rune]bool)
			for _, s := range []string{b.Model.Value(), input} {
				for _, occupied := range s {
					used[occupied] = true
				}
			}
		}
		for used[next] {
			next++
		}
		used[next] = true
		b.encoded[next] = r
		next++
		return next - 1
	}, input)
}
