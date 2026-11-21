package app

import (
	"errors"
	"net/url"
	"strings"

	tea "charm.land/bubbletea/v2"
	"promtui/internal/editor"
)

func alertGeneratorExpression(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", errors.New("No generator URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("Invalid generator URL")
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" ||
		!strings.HasSuffix(u.Path, "/graph") {
		return "", errors.New("Not a Prometheus graph URL")
	}
	params, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", errors.New("Invalid generator URL query")
	}
	key := "g0.expr"
	if !params.Has(key) {
		key = "expr"
	}
	if !params.Has(key) {
		return "", errors.New("No PromQL in generator URL")
	}
	expression := params.Get(key)
	if strings.TrimSpace(expression) == "" {
		return "", errors.New("Generator query is empty")
	}
	return expression, nil
}

func (m *Model) loadAlertQuery() tea.Cmd {
	a := &m.alerts
	expression, err := alertGeneratorExpression(a.detail.GeneratorURL)
	if err != nil {
		a.detailStatus = err.Error()
		return nil
	}
	next := m.editor
	next.SetValue(expression)
	// The textarea can normalize characters or truncate input; don't replace
	// the draft unless it can hold the exact decoded expression.
	if next.Value() != expression {
		a.detailStatus = "Query exceeds editor limits or contains unsupported characters"
		return nil
	}

	a.dialog = alertNoDialog
	a.stop()
	m.activeTab = promQLTab
	m.dismiss()
	m.panel = 0
	if m.resultFilter.input.Focused() {
		// Keep the current result filter applied while moving focus to the editor.
		m.act("finishFilter", "", filterKeys)
	}
	m.editor = next
	m.cursor = -1
	m.diagnostic = editor.Diagnostic(expression)
	return m.editor.Focus()
}
