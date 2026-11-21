package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"promtui/internal/config"
	"promtui/internal/history"
)

func TestSavedItemsAcrossTabsAndConnections(t *testing.T) {
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/api/v2/alerts") {
			t.Errorf("unexpected API request: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		reads.Add(1)
		site := strings.Split(r.URL.Path, "/")[1]
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `[
{"fingerprint":"one","labels":{"alertname":%q,"service":"srv.a","severity":"warning"},"status":{"state":"active"}},
{"fingerprint":"two","labels":{"alertname":%q,"service":"wrong","severity":"warning"},"status":{"state":"active"}},
{"fingerprint":"three","labels":{"alertname":"Other","service":"srv.a","severity":"critical"},"status":{"state":"active"}}
]`, site+"CPU", site+"Disk")
	}))
	defer server.Close()

	dir := t.TempDir()
	write := func(name, contents string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	queries := write("queries.yaml", `queries:
  - name: Scoped query
    expression: 'up{instance="${instance}"}'
    parameters:
      - name: instance
        default: srv.a
`)
	alerts := write("alerts.yaml", `alerts:
  - name: Scope by service
    parameters:
      - name: service
        default: 'srv[.]a'
    matchers:
      - label: service
        operator: '=~'
        value: '${service}'
`)
	connections := []config.Connection{
		{Name: "North", URL: server.URL + "/North", AlertmanagerURL: server.URL + "/North"},
		{Name: "South", URL: server.URL + "/South", AlertmanagerURL: server.URL + "/South"},
	}
	m := New(Options{
		Connection: connections[0], Connections: connections,
		SavedQueriesPath: queries, SavedAlertsPath: alerts, Timeout: time.Second,
		ProgramStatus: true, Ranged: true, Window: time.Hour,
	}, []history.Entry{{Query: "keep history"}})
	m.update(completed{result: sampleResult(false)})
	m.programStatus = terminalStatus{} // Start observing status after the fake query completes.
	m.editor.SetValue("untouched")
	key := func(code rune, mod tea.KeyMod) tea.Cmd {
		_, cmd := m.update(tea.KeyPressMsg{Code: code, Mod: mod})
		return cmd
	}
	fetch := func(cmd tea.Cmd) {
		t.Helper()
		if cmd == nil {
			t.Fatal("expected a refresh command")
		}
		m.update(cmd())
		if m.alerts.err != nil {
			t.Fatal(m.alerts.err)
		}
	}
	requireVisible := func(want ...string) {
		t.Helper()
		if len(m.alerts.visible) != len(want) {
			t.Fatalf("got %d visible alerts, want %v", len(m.alerts.visible), want)
		}
		for i, row := range m.alerts.visible {
			if got := row.Labels["alertname"]; got != want[i] {
				t.Errorf("visible alert %d = %q, want %q", i, got, want[i])
			}
		}
	}
	fetch(key('1', 0))
	press(m, 's', '/')
	savedPaste(m, "warning")
	press(m, tea.KeyEnter)
	sort := m.alerts.sort
	savedCtrl(m, 'o')
	press(m, tea.KeyEnter, tea.KeyEsc)
	if m.parameterDialog != nil || len(m.alerts.visible) != 2 {
		t.Fatal("cancelled alert parameters changed the Alerts list")
	}
	press(m, tea.KeyEnter)
	if m.parameterDialog == nil {
		t.Fatal("saved alert filter did not prompt for parameters")
	}
	key(tea.KeyEnter, tea.ModCtrl)
	requireVisible("NorthCPU")
	if m.alerts.sort != sort || m.alerts.filter != "warning" || reads.Load() != 1 {
		t.Fatal("apply changed sort/search or issued a request")
	}
	fetch(key('r', 0))
	requireVisible("NorthCPU")

	key('0', 0)
	savedCtrl(m, 'o')
	press(m, tea.KeyEnter)
	if m.parameterDialog == nil || m.editor.Value() != "untouched" {
		t.Fatal("saved PromQL parameters did not preserve draft while prompting")
	}
	key(tea.KeyEnter, tea.ModCtrl)
	if m.editor.Value() != `up{instance="srv.a"}` || m.id != 0 || len(m.entries) != 1 ||
		len(m.table.Rows) != 2 || !m.opt.Ranged || m.opt.Window != time.Hour || m.programStatus != (terminalStatus{}) {
		t.Fatalf("loading modified execution state: editor=%q id=%d history=%d rows=%d range=%v window=%s status=%+v",
			m.editor.Value(), m.id, len(m.entries), len(m.table.Rows), m.opt.Ranged, m.opt.Window, m.programStatus)
	}
	key('1', tea.ModAlt)
	requireVisible("NorthCPU")
	press(m, 'c', tea.KeyDown)
	fetch(key(tea.KeyEnter, 0))
	if m.opt.Connection.Name != "South" || len(m.alerts.visible) != 2 {
		t.Fatalf("structured filter was not reapplied to the new connection: %d visible", len(m.alerts.visible))
	}
	for _, row := range m.alerts.visible {
		if row.Labels["service"] != "srv.a" {
			t.Fatal("structured filter was lost on connection switch")
		}
	}
	if reads.Load() != 3 {
		t.Fatalf("expected only entry, refresh and connection requests; got %d", reads.Load())
	}
}
