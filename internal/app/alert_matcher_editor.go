package app

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"gopkg.in/yaml.v3"
	"promtui/internal/parameters"
	"promtui/internal/savedalerts"
)

var alertOperators = []string{"=", "!=", "=~", "!~"}

type alertMatcherInputs struct {
	label, value textinput.Model
	operator     int
	initialValue string
	valueEdited  bool
}

func newAlertMatcherInputs(m savedalerts.Matcher) alertMatcherInputs {
	input := func(value, placeholder string) textinput.Model {
		i := textinput.New()
		i.Prompt, i.Placeholder = "", placeholder
		i.SetValue(value)
		i.SetVirtualCursor(false)
		return i
	}
	return alertMatcherInputs{
		label: input(m.Label, "label"), value: input(m.Value, "value (may be empty)"),
		operator: max(0, slices.Index(alertOperators, m.Operator)), initialValue: m.Value,
	}
}

func (m *Model) openAlertMatcherEditor() tea.Cmd {
	m.alerts.saved = alertFiltersDialog{mode: alertSavedManual}
	d := &m.alerts.saved
	for _, matcher := range m.alerts.structured.matchers {
		d.editors = append(d.editors, newAlertMatcherInputs(matcher))
	}
	if len(d.editors) == 0 {
		d.editors = append(d.editors, newAlertMatcherInputs(savedalerts.Matcher{}))
	}
	m.sizeAlertSavedInputs()
	return d.focusMatcher(0, 0)
}

func (d *alertFiltersDialog) focusMatcher(row, field int) tea.Cmd {
	d.editors[d.row].label.Blur()
	d.editors[d.row].value.Blur()
	d.row, d.field = row, field
	switch field {
	case 0:
		return d.editors[row].label.Focus()
	case 2:
		return d.editors[row].value.Focus()
	}
	return nil
}

func (d *alertFiltersDialog) editMatcher(msg tea.Msg) tea.Cmd {
	row := &d.editors[d.row]
	if d.field == 1 {
		return nil
	}
	input := &row.label
	if d.field == 2 {
		input = &row.value
	}
	if paste, ok := msg.(tea.PasteMsg); ok {
		probe := textinput.New()
		probe.SetValue(paste.Content)
		if probe.Value() != paste.Content {
			d.err = errors.New("paste a single-line label or value; use a saved template for multiline values")
			return nil
		}
	}
	previous := input.Value()
	var cmd tea.Cmd
	*input, cmd = input.Update(msg)
	if previous != input.Value() {
		d.err = nil
		if d.field == 2 {
			row.valueEdited = true
		}
	}
	return cmd
}

func (m *Model) alertMatcherKey(k tea.KeyPressMsg) tea.Cmd {
	d := &m.alerts.saved
	key := k.Keystroke()
	switch key {
	case "up":
		return d.focusMatcher(max(0, d.row-1), d.field)
	case "down":
		return d.focusMatcher(min(d.row+1, len(d.editors)-1), d.field)
	case "tab", "shift+tab":
		total := len(d.editors) * 3
		delta := 1
		if key == "shift+tab" {
			delta = -1
		}
		next := (d.row*3 + d.field + total + delta) % total
		return d.focusMatcher(next/3, next%3)
	case "ctrl+n":
		d.editors = append(d.editors, newAlertMatcherInputs(savedalerts.Matcher{}))
		m.sizeAlertSavedInputs()
		return d.focusMatcher(len(d.editors)-1, 0)
	case "ctrl+d":
		d.editors = slices.Delete(d.editors, d.row, d.row+1)
		if len(d.editors) == 0 {
			d.editors = append(d.editors, newAlertMatcherInputs(savedalerts.Matcher{}))
			m.sizeAlertSavedInputs()
		}
		d.row, d.err = min(d.row, len(d.editors)-1), nil
		return d.focusMatcher(d.row, d.field)
	case "ctrl+enter", "ctrl+g":
		matchers := d.matcherDraft()
		match, err := savedalerts.Compile(matchers)
		if err != nil {
			d.err = err
			return nil
		}
		if len(matchers) == 0 {
			match = nil
		}
		// Opening and applying an unchanged saved filter retains its name.
		if !slices.Equal(matchers, m.alerts.structured.matchers) {
			m.setAlertMatchers(alertMatcherFilter{matchers: matchers, match: match})
		}
		m.closeAlertSaved()
		return nil
	}
	if d.field == 1 {
		match := &d.editors[d.row]
		switch key {
		case "left":
			match.operator = (match.operator + len(alertOperators) - 1) % len(alertOperators)
		case "right", "space", "enter":
			match.operator = (match.operator + 1) % len(alertOperators)
		default:
			return nil
		}
		d.err = nil
		return nil
	}
	return d.editMatcher(k)
}

func (d *alertFiltersDialog) matcherDraft() []savedalerts.Matcher {
	var result []savedalerts.Matcher
	for _, row := range d.editors {
		label, value := row.label.Value(), row.value.Value()
		if !row.valueEdited {
			value = row.initialValue
		}
		if len(d.editors) == 1 && label == "" && value == "" && row.operator == 0 {
			continue // The single initial, blank row means no structured filter.
		}
		result = append(result, savedalerts.Matcher{Label: label, Operator: alertOperators[row.operator], Value: value})
	}
	return result
}

type alertTemplateInput struct{ text textarea.Model }

type alertTemplateFields struct {
	Parameters []parameters.Parameter `yaml:"parameters,omitempty"`
	Matchers   []savedalerts.Matcher  `yaml:"matchers"`
}

func (m *Model) openAlertTemplate(item savedalerts.Filter) tea.Cmd {
	d := &m.alerts.saved
	raw, err := yaml.Marshal(alertTemplateFields{item.Parameters, item.Matchers})
	if err != nil {
		d.err = fmt.Errorf("show template: %w", err)
		return nil
	}
	d.mode = alertSavedTemplate
	text := textarea.New()
	text.Prompt, text.ShowLineNumbers = "", false
	text.CharLimit, text.MaxHeight, text.MaxWidth = 0, 0, 0
	text.SetVirtualCursor(false)
	text.SetValue(string(raw))
	if text.Value() != string(raw) {
		d.mode, d.err = alertSavedBrowse, errors.New("template does not fit the editor")
		return nil
	}
	d.template.text = text
	m.sizeAlertSavedInputs()
	return d.template.text.Focus()
}

func (t *alertTemplateInput) edit(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	t.text, cmd = t.text.Update(msg)
	return cmd
}

func (t *alertTemplateInput) filter(name string) (savedalerts.Filter, error) {
	dec := yaml.NewDecoder(bytes.NewBufferString(t.text.Value()))
	dec.KnownFields(true)
	var fields alertTemplateFields
	if err := dec.Decode(&fields); err != nil {
		return savedalerts.Filter{}, fmt.Errorf("invalid template YAML: %w", err)
	}
	if err := dec.Decode(new(yaml.Node)); err != io.EOF {
		if err == nil {
			err = errors.New("expected one YAML document")
		}
		return savedalerts.Filter{}, fmt.Errorf("invalid template YAML: %w", err)
	}
	if strings.TrimSpace(name) == "" {
		return savedalerts.Filter{}, errors.New("enter a name")
	}
	return savedalerts.Filter{Name: name, Parameters: fields.Parameters, Matchers: fields.Matchers}, nil
}
