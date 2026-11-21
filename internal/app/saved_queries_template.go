package app

import (
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"gopkg.in/yaml.v3"
	"promtui/internal/parameters"
	"promtui/internal/saveditems"
)

type queryTemplateFields struct {
	Expression yaml.Node `yaml:"expression"`
	Parameters yaml.Node `yaml:"parameters,omitempty"`
}

type savedTemplateState struct {
	editor *savedYAML
	target int
	err    error
}

func (t *savedTemplateState) resize(w, h int) {
	t.editor.resize(min(76, w-2), h)
}

func (m *Model) openSavedTemplateEditor(target int) tea.Cmd {
	q := &m.savedQueries
	item := q.queries[target]
	fields := queryTemplateFields{Expression: saveditems.StringNode(item.Expression)}
	defs := item.Parameters
	if defs == nil {
		// Explicitly opening the template editor lets a user opt a plain
		// expression into template validation, with no automatic rewriting.
		defs = []parameters.Parameter{}
	}
	if err := fields.Parameters.Encode(defs); err != nil {
		q.err = err
		return nil
	}
	if fields.Parameters.Kind == yaml.SequenceNode && len(fields.Parameters.Content) == 0 {
		fields.Parameters.Style = yaml.FlowStyle
	}
	edit, err := newSavedYAML(fields)
	if err != nil {
		q.err = err
		return nil
	}
	m.savedTemplate = savedTemplateState{editor: edit, target: target}
	m.panel = savedTemplateKeys
	m.savedTemplate.resize(m.width, m.height)
	return edit.text.Focus()
}

func (m *Model) backSavedTemplate() {
	m.savedTemplate = savedTemplateState{}
	m.panel = savedBrowserKeys
}

func (m *Model) editTemplate(msg tea.Msg) tea.Cmd {
	t := &m.savedTemplate
	before := t.editor.text.Value()
	cmd := t.editor.edit(msg)
	if t.editor.text.Value() != before {
		t.err = nil
	}
	return cmd
}

func (m *Model) applySavedTemplate() tea.Cmd {
	t := &m.savedTemplate
	q := &m.savedQueries
	var fields queryTemplateFields
	if err := t.editor.decode(&fields); err != nil {
		t.err = err
		return nil
	}
	if fields.Expression.Kind != yaml.ScalarNode || fields.Expression.ShortTag() != "!!str" {
		t.err = errors.New("expression must be a string")
		return nil
	}
	var defs []parameters.Parameter
	if !fields.Parameters.IsZero() {
		if err := parameters.DecodeDefinitions(&fields.Parameters, &defs); err != nil {
			t.err = err
			return nil
		}
	}
	snapshot, err := q.snapshot.Update(t.target, fields.Expression.Value, defs)
	if err != nil {
		if errors.Is(err, saveditems.ErrChanged) {
			err = errors.New("file changed; reopen Saved Queries")
		}
		t.err = err
		return nil
	}
	q.snapshot, q.queries = snapshot, snapshot.Queries()
	q.rebuild(q.filter, t.target, q.row)
	m.backSavedTemplate()
	m.savedFeedbackID++
	m.savedFeedback = "Query updated"
	id := savedFeedbackExpired(m.savedFeedbackID)
	return tea.Tick(2500*time.Millisecond, func(time.Time) tea.Msg { return id })
}

func (m *Model) savedTemplateLayer() (*lipgloss.Layer, *tea.Cursor) {
	t := &m.savedTemplate
	return m.savedTemplateOverlay(t.editor, "Edit Expression Template", m.savedQueries.queries[t.target].Name, t.err, min(76, m.width-2))
}
