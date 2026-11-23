package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/config"
	"promtui/internal/editor"
)

func TestConnectionPickerPreservesEditor(t *testing.T) {
	prod, dev := config.Connection{Name: "prod"}, config.Connection{Name: "dev"}
	m := New(Options{Connection: prod, Connections: []config.Connection{dev, prod}}, nil)
	press(m, 'i', 'c')
	if m.pickingConnection || m.editor.Value() != "c" {
		t.Fatal("plain c must type in the editor")
	}
	const draft = "sum(\n  up\n)"
	m.editor.SetValue(draft)
	m.editor.CursorUp()
	m.editor.SetCursorColumn(3)
	pos := editor.Position(m.editor)
	pressAlt(m, 'c')
	screen := ansi.Strip(m.screen())
	if m.View().Cursor != nil || !strings.Contains(screen, "Connections") ||
		!strings.Contains(screen, "prod (current)") || !strings.Contains(screen, "sum(") {
		t.Fatal(screen)
	}
	press(m, tea.KeyUp, '2', tea.KeyTab)
	m.Update(tea.PasteMsg{Content: "discard"})
	press(m, tea.KeyEsc)
	if !reflect.DeepEqual(m.opt.Connection, prod) || m.activeTab != promQLTab {
		t.Fatal("cancel switched the connection or tab")
	}
	pressAlt(m, 'c')
	press(m, tea.KeyUp, tea.KeyEnter)
	if !reflect.DeepEqual(m.opt.Connection, dev) || m.pickingConnection || m.cancel != nil || m.View().Cursor == nil ||
		m.editor.Value() != draft || editor.Position(m.editor) != pos {
		t.Fatal(m.screen())
	}
}

func TestConnectionPickerIsSharedAcrossTabs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/alerts":
			fmt.Fprint(w, `[]`)
		case "/api/v2/status":
			fmt.Fprint(w, `{"config":{"original":"{}"}}`)
		case "/api/v1/query":
			if got := r.FormValue("query"); got != "up" {
				t.Errorf("query = %q", got)
			}
			fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	old := config.Connection{Name: "old"}
	next := config.Connection{Name: "next", URL: server.URL, AlertmanagerURL: server.URL}
	m := New(Options{Connection: old, Connections: []config.Connection{old, next}, Timeout: time.Second}, nil)
	m.editor.SetValue("up")
	m.Update(completed{result: sampleResult(false)})
	queryID := m.id
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	m.cancel = cancel
	m.cache = map[cacheKey]cached{{kind: "expr"}: {}}
	pressAlt(m, '1')
	press(m, 'c', tea.KeyDown)
	if s := ansi.Strip(m.screen()); !strings.Contains(s, "Connections") || !strings.Contains(s, "› next") {
		t.Fatal(s)
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || m.activeTab != alertsTab || !reflect.DeepEqual(m.opt.Connection, next) || ctx.Err() == nil || m.cache != nil {
		t.Fatal("switch did not reset old backend state and refresh Alerts")
	}
	if msg, ok := cmd().(alertsCompleted); !ok || msg.err != nil {
		t.Fatalf("unexpected Alerts response: %#v", msg)
	} else {
		m.Update(msg)
	}
	m.Update(completed{id: queryID, result: sampleResult(false)})
	if len(m.table.Rows) != 0 || m.summary != "No query executed" {
		t.Fatal("old results survived the switch")
	}
	press(m, '3', 'c')
	if s := ansi.Strip(m.screen()); !strings.Contains(s, "Connections") || !strings.Contains(s, "› next (current)") {
		t.Fatal(s)
	}
	press(m, tea.KeyEnter, '0')
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("no PromQL request")
	}
	if msg, ok := cmd().(completed); !ok || msg.err != nil {
		t.Fatalf("unexpected PromQL response: %#v", msg)
	}
	press(m, 'c', tea.KeyUp, tea.KeyEnter)
	if !reflect.DeepEqual(m.opt.Connection, old) || m.editor.Value() != "up" {
		t.Fatal("switching from PromQL results did not preserve the query")
	}
}
