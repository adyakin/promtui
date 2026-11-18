package app

import (
	"context"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"promtui/internal/config"
	"promtui/internal/editor"
	"promtui/internal/history"
)

func TestMidTokenAcceptance(t *testing.T) {
	m := New(Options{}, nil)
	m.editor.SetValue("sum(\nnode_foo + up\n)")
	m.editor.CursorUp()
	m.editor.SetCursorColumn(4)
	m.context = editor.At(m.editor.Value(), editor.Position(m.editor))
	m.options = []string{"node_cpu"}
	m.popup = true
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if !m.popup || !m.editor.Focused() || m.editor.Value() != "sum(\nnode_foo + up\n)" {
		t.Fatal("Shift+Tab escaped the completion overlay")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.editor.Value() != "sum(\nnode_cpu + up\n)" || m.editor.Column() != 8 || m.editor.Line() != 1 || m.popup || !m.editor.Focused() {
		t.Fatal(m.editor.Value(), m.editor.Column(), m.editor.Line())
	}
}
func TestCacheAndStaleResponse(t *testing.T) {
	m := New(Options{}, nil)
	m.generation = 2
	c := editor.Context{Kind: "expr", Prefix: "no"}
	a := analysisMsg{2, context.Background(), c, "", true}
	k := cacheKey{"expr", "", "", "no"}
	m.cache = map[cacheKey]cached{k: {time.Now().Add(time.Minute), []string{"node_count"}}}
	if m.analyzed(a) != nil || !slices.Contains(m.options, "node_count") {
		t.Fatal(m.options)
	}
	m.dismiss()
	m.suggested(suggestionsMsg{2, k, []string{"obsolete"}, nil})
	if m.popup || len(m.options) > 0 {
		t.Fatal(m.options)
	}
}
func TestHistoryFuzzyRestoreWithoutExecuting(t *testing.T) {
	m := New(Options{Connection: config.Connection{Name: "A"}}, []history.Entry{{Connection: "B", Query: "vector(12)"}, {Connection: "A", Query: "vector(42)"}})
	m.editor.SetValue("draft")
	m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if !m.searching || !m.editor.Focused() || m.searchText != "" {
		t.Fatal("Tab leaked through History")
	}
	press(m, tea.KeyEsc)
	if m.searching || !m.editor.Focused() || m.editor.Value() != "draft" {
		t.Fatal("Esc did not restore Query from History")
	}
	m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m.Update(tea.PasteMsg{Content: "v42"})
	if !slices.Equal(m.options, []string{"vector(42)"}) {
		t.Fatal(m.options)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.editor.Value() != "vector(42)" || m.cancel != nil || m.searching {
		t.Fatal(m.screen())
	}
}
