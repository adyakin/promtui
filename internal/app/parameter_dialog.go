package app

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/parameters"
	"promtui/internal/ui"
)

type parameterField struct {
	definition parameters.Parameter
	input      textinput.Model
	edited     bool
}

type parameterDialog struct {
	name   string
	fields []parameterField
	row    int
	err    error
	apply  func(parameters.Values) (tea.Cmd, error)
}

type parameterClipboard struct {
	dialog  *parameterDialog
	row     int
	content string
	err     error
}

// The caller keeps its browser open underneath. Cancel simply removes the
// dialog; on success apply may close the browser or update the consuming view.
func (m *Model) openParameters(name string, definitions []parameters.Parameter, apply func(parameters.Values) (tea.Cmd, error)) tea.Cmd {
	d := &parameterDialog{name: name, apply: apply}
	m.parameterDialog = d
	for _, p := range definitions {
		input := textinput.New()
		input.SetVirtualCursor(false)
		input.Prompt = ""
		if p.Default != nil {
			input.SetValue(*p.Default)
		} else {
			input.Placeholder = "Required"
		}
		d.fields = append(d.fields, parameterField{definition: p, input: input})
	}
	d.resize(m.width)
	if len(d.fields) != 0 {
		return d.fields[0].input.Focus()
	}
	return nil
}

func (d *parameterDialog) resize(width int) {
	if d == nil {
		return
	}
	width = min(60, width-2)
	for i := range d.fields {
		input := &d.fields[i].input
		input.SetWidth(max(1, width-width/3-7))
		input.SetCursor(input.Position())
	}
}

func (d *parameterDialog) edit(msg tea.Msg) tea.Cmd {
	if len(d.fields) == 0 {
		return nil
	}
	f := &d.fields[d.row]
	if paste, ok := msg.(tea.PasteMsg); ok {
		// A single-line input cannot faithfully represent these characters.
		// Do not silently collapse them to spaces before raw substitution.
		probe := textinput.New()
		probe.SetValue(paste.Content)
		if probe.Value() != paste.Content {
			d.err = fmt.Errorf("paste a single-line value for %s", f.definition.Name)
			return nil
		}
	}
	before := f.input.Value()
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	if f.input.Value() != before {
		f.edited, d.err = true, nil
		f.input.Placeholder = "(empty)"
	}
	return cmd
}

func (d *parameterDialog) focus(row int) tea.Cmd {
	if len(d.fields) == 0 {
		return nil
	}
	d.fields[d.row].input.Blur()
	d.row = row
	return d.fields[row].input.Focus()
}

func (m *Model) parameterKey(k tea.KeyPressMsg) tea.Cmd {
	d := m.parameterDialog
	switch k.Keystroke() {
	case "esc":
		m.parameterDialog = nil
	case "tab", "down", "shift+tab", "up":
		if n := len(d.fields); n != 0 {
			delta := 1
			if k.Keystroke() == "shift+tab" || k.Keystroke() == "up" {
				delta = -1
			}
			return d.focus((d.row + n + delta) % n)
		}
	case "ctrl+space", "ctrl+@":
		if len(d.fields) != 0 {
			f := &d.fields[d.row]
			f.input.SetValue("")
			f.input.Placeholder = "(empty)"
			f.edited, d.err = true, nil
		}
	case "ctrl+v":
		row := d.row
		return func() tea.Msg {
			value, err := clipboard.ReadAll()
			return parameterClipboard{d, row, value, err}
		}
	case "ctrl+enter", "ctrl+g":
		values := make(parameters.Values, len(d.fields))
		for i, f := range d.fields {
			p := f.definition
			switch {
			case f.edited:
				values[p.Name] = f.input.Value()
			case p.Default != nil:
				values[p.Name] = *p.Default
			default:
				d.err = fmt.Errorf("%s is required (Ctrl+Space for empty)", p.Name)
				return d.focus(i)
			}
		}
		cmd, err := d.apply(values)
		if err == nil {
			m.parameterDialog = nil
		} else {
			d.err = err
		}
		return cmd
	default:
		return d.edit(k)
	}
	return nil
}

func (m *Model) parameterLayer() (*lipgloss.Layer, *tea.Cursor) {
	d := m.parameterDialog
	width := min(60, m.width-2)
	maxRows := m.height - 5
	var rows []string
	var errors []string
	if d.err != nil {
		errors = strings.Split(ansi.Hardwrap(ui.Plain(d.err.Error()), width-4, false), "\n")
		limit := min(len(errors), 3, maxRows-2)
		if limit < len(errors) {
			errors[limit-1] = ansi.Truncate(errors[limit-1], width-5, "") + "…"
		}
		errors = errors[:limit]
	}
	room := maxRows - len(errors)
	first := dialogStart(d.row, len(d.fields), room)
	labelWidth := width / 3
	for i := first; i < min(first+room, len(d.fields)); i++ {
		f := &d.fields[i]
		label := ansi.Truncate(f.definition.Name, labelWidth, "…")
		row := fmt.Sprintf("%-*s", labelWidth, label) + " " + ui.Plain(ansi.Strip(f.input.View()))
		rows = append(rows, dialogRow(row, "", width-2, i == d.row))
	}
	for _, line := range errors {
		rows = append(rows, dialogSurface.Foreground(lipgloss.Color("11")).Render(" "+line+" "))
	}
	x, y := (m.width-width)/2, 1+(m.height-5-len(rows))/2
	meta := ""
	if len(d.fields) > 1 {
		meta = fmt.Sprintf("%d/%d", d.row+1, len(d.fields))
	}
	layer := lipgloss.NewLayer(renderFrame("Parameters: "+ui.Plain(d.name), meta, rows, width, true, dialogSurface)).X(x).Y(y).Z(2)
	if len(d.fields) != 0 && !m.quitting {
		if c := d.fields[d.row].input.Cursor(); c != nil {
			return layer, nativeCursor(min(x+width-2, x+5+labelWidth+c.X), y+1+d.row-first)
		}
	}
	return layer, nil
}
