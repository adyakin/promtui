package app

import (
	"errors"
	"maps"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/parameters"
)

func TestParameterDialogNavigationAndValidation(t *testing.T) {
	m := New(Options{SavedQueriesPath: savedFixture(t, "A"), Graphics: true}, nil)
	m.update(completed{result: sampleResult(true)})
	place, layout := m.graphPlace(), m.workspace()
	if place.cols == 0 {
		t.Fatal("expected a graph behind the dialog")
	}
	m.chart.displayed, m.chart.wanted = place, place
	m.editor.SetValue("keep this draft")
	savedCtrl(m, 'o')
	enter := tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl}
	empty, suggestion := "", "a.b"
	defs := []parameters.Parameter{{Name: "one", Default: &empty}, {Name: "two"}, {Name: "three", Default: &suggestion}}
	calls := 0
	open := func() {
		m.openParameters("Example", defs, func(v parameters.Values) (tea.Cmd, error) {
			calls++
			want := parameters.Values{"one": "", "two": "", "three": "a.b雪${one}"}
			if !maps.Equal(v, want) {
				t.Errorf("applied values = %#v, want %#v", v, want)
			}
			if calls == 1 {
				return nil, errors.New("resolved matcher failed")
			}
			return nil, nil
		})
	}
	open()
	savedText(t, m.screen(), "Parameters: Example")
	if m.graphPlace() != place || m.workspace() != layout {
		t.Fatal("parameter dialog moved the graph")
	}
	m.Update(enter)
	if calls != 0 || m.parameterDialog.row != 1 {
		t.Fatal("untouched required value should block apply and receive focus")
	}
	savedText(t, m.screen(), "two is required")
	savedPaste(m, "not\nsingle line")
	savedText(t, m.screen(), "paste a single-line value")
	savedCtrl(m, tea.KeySpace)
	press(m, tea.KeyTab)
	savedPaste(m, "雪${one}")
	m.Update(tea.KeyPressMsg{Code: '1', Mod: tea.ModAlt})
	m.Update(tea.WindowSizeMsg{Width: minWidth, Height: minHeight})
	m.Update(tea.WindowSizeMsg{Width: 13, Height: 5})
	m.Update(tea.WindowSizeMsg{Width: minWidth, Height: minHeight})
	if m.activeTab != promQLTab || m.parameterDialog.fields[2].input.Value() != "a.b雪${one}" {
		t.Fatal("modal leaked a tab switch or lost input through resize")
	}
	m.Update(enter)
	if calls != 1 || m.parameterDialog == nil {
		t.Fatal("resolved-content failure dismissed the dialog")
	}
	for _, row := range strings.Split(ansi.Strip(m.screen()), "\n") {
		if ansi.StringWidth(row) > minWidth {
			t.Fatalf("row exceeds terminal width: %q", row)
		}
	}
	m.Update(enter)
	if calls != 2 || m.parameterDialog != nil || m.panel != savedBrowserKeys || m.editor.Value() != "keep this draft" {
		t.Fatal("dialog did not restore its caller on apply")
	}
	open()
	press(m, tea.KeyEsc)
	if calls != 2 || m.parameterDialog != nil || m.panel != savedBrowserKeys || m.editor.Value() != "keep this draft" {
		t.Fatal("cancel did not return to the browser without applying")
	}
}
