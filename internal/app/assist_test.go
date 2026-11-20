package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"promtui/internal/config"
	"promtui/internal/editor"
	"promtui/internal/history"
)

func TestCompleteFuzzyMetricPhrase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/label/__name__/values" || r.URL.Query().Get("match[]") != `{__name__=~"node.*cpu.*load.*"}` {
			t.Errorf("unexpected metadata request: %s", r.URL)
		}
		fmt.Fprint(w, `{"status":"success","data":["node_cpu_load_average","node_cpu_guest_load_average"]}`)
	}))
	defer server.Close()
	for _, tc := range []struct {
		input tea.Msg
		draft string
		want  string
	}{
		{tea.PasteMsg{Content: "node cpu load"}, "", "node_cpu_load_average"},
		{tea.KeyPressMsg{Code: ' ', Mod: tea.ModCtrl}, "node cpu load ", "node_cpu_load_average "},
	} {
		m := New(Options{Connection: config.Connection{URL: server.URL}}, nil)
		press(m, 'i')
		m.editor.SetValue(tc.draft)
		_, analyze := m.Update(tc.input)
		msg := analyze()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, cmd := range batch {
				if a, ok := cmd().(analysisMsg); ok {
					msg = a
					break
				}
			}
		}
		_, fetch := m.Update(msg)
		if fetch == nil {
			t.Fatalf("no metadata lookup for %q: %T", m.editor.Value(), msg)
		}
		m.Update(fetch())
		if !m.popup || !slices.Equal(m.options, []string{"node_cpu_load_average", "node_cpu_guest_load_average"}) {
			t.Fatal(m.options)
		}
		press(m, tea.KeyTab)
		if m.editor.Value() != tc.want || m.popup || !m.editor.Focused() {
			t.Fatal("Tab did not complete the whole phrase", m.editor.Value())
		}
	}
}

func TestMidTokenAcceptance(t *testing.T) {
	m := New(Options{}, nil)
	press(m, 'i')
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
	press(m, 'i')
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
	if !slices.Equal(m.options, []string{"vector(12)", "vector(42)"}) {
		t.Fatal(m.options)
	}
	m.Update(tea.PasteMsg{Content: "v12"})
	if !slices.Equal(m.options, []string{"vector(12)"}) {
		t.Fatal(m.options)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.editor.Value() != "vector(12)" || m.cancel != nil || m.searching {
		t.Fatal(m.screen())
	}
}
