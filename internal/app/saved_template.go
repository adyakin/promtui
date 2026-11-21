package app

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
	"gopkg.in/yaml.v3"
	"promtui/internal/ui"
)

type savedYAML struct{ text textarea.Model }

func newSavedYAML(fields any) (*savedYAML, error) {
	raw, err := yaml.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("show template: %w", err)
	}
	text := textarea.New()
	text.Prompt, text.ShowLineNumbers = "", false
	text.CharLimit, text.MaxHeight, text.MaxWidth = 0, 0, 0
	text.SetVirtualCursor(false)
	text.SetValue(string(raw))
	if text.Value() != string(raw) {
		return nil, errors.New("template does not fit the editor")
	}
	text.MoveToBegin()
	return &savedYAML{text}, nil
}

func (t *savedYAML) resize(width, height int) {
	if t != nil {
		t.text.SetWidth(max(1, width-4))
		t.text.SetHeight(max(1, min(height-8, 14)))
	}
}

type templateClipboard struct {
	editor *savedYAML
	value  string
	err    error
}

func (t *savedYAML) edit(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok && k.Keystroke() == "ctrl+v" {
		return func() tea.Msg {
			value, err := clipboard.ReadAll()
			return templateClipboard{t, value, err}
		}
	}
	var cmd tea.Cmd
	t.text, cmd = t.text.Update(msg)
	return cmd
}

func (t *savedYAML) decode(fields any) error {
	dec := yaml.NewDecoder(bytes.NewBufferString(t.text.Value()))
	dec.KnownFields(true)
	if err := dec.Decode(fields); err != nil {
		return fmt.Errorf("invalid template YAML: %w", err)
	}
	if err := dec.Decode(new(yaml.Node)); err != io.EOF {
		if err == nil {
			err = errors.New("expected one YAML document")
		}
		return fmt.Errorf("invalid template YAML: %w", err)
	}
	return nil
}

func (m *Model) savedTemplateOverlay(t *savedYAML, title, name string, err error, width int) (*lipgloss.Layer, *tea.Cursor) {
	maxRows := m.height - 5
	var rows, errorRows []string
	if err != nil {
		errorRows = strings.Split(ansi.Hardwrap(ui.Plain(err.Error()), width-4, false), "\n")
		limit := min(3, len(errorRows))
		if limit < len(errorRows) {
			errorRows[limit-1] = ansi.Truncate(errorRows[limit-1], width-5, "") + "…"
		}
		errorRows = errorRows[:limit]
	}
	room := maxRows - len(errorRows)
	text := func(s string) {
		rows = append(rows, dialogSurface.Render(" "+ansi.Truncate(ui.Plain(s), width-4, "…")+" "))
	}
	if room > 2 {
		text(name + " · YAML")
	}
	lines := strings.Split(ansi.Strip(t.text.View()), "\n")
	caret := t.text.Cursor()
	available := room - len(rows)
	first := 0
	if caret != nil {
		first = max(0, caret.Y-available+1)
	}
	first = min(first, max(0, len(lines)-available))
	editorY := len(rows) + 1
	for _, line := range lines[first:min(first+available, len(lines))] {
		text(line)
	}
	for _, line := range errorRows {
		rows = append(rows, dialogSurface.Foreground(lipgloss.Color("11")).Render(" "+line+" "))
	}
	x, y := (m.width-width)/2, 1+(maxRows-len(rows))/2
	layer := lipgloss.NewLayer(renderFrame(title, "", rows, width, true, dialogSurface)).X(x).Y(y).Z(1)
	if caret != nil && !m.quitting && caret.Y >= first && caret.Y < first+available {
		return layer, nativeCursor(min(x+width-2, x+2+caret.X), y+editorY+caret.Y-first)
	}
	return layer, nil
}
