package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/alertmanager"
	"promtui/internal/config"
)

func TestAlertInhibitionDetailsNavigation(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Path {
		case "/am/api/v2/alerts":
			fmt.Fprint(w, `[
				{"fingerprint":"source","labels":{"alertname":"HighLatency","severity":"critical","servicename":"payments"},"status":{"state":"active"}},
				{"fingerprint":"target-a","labels":{"alertname":"HighLatency","severity":"warning","servicename":"payments","instance":"a"},"status":{"state":"suppressed","inhibitedBy":["source","missing"]}},
				{"fingerprint":"target-b","labels":{"alertname":"HighLatency","severity":"warning","servicename":"payments","instance":"b"},"status":{"state":"suppressed","inhibitedBy":["source"]}},
				{"fingerprint":"potential","labels":{"alertname":"HighLatency","severity":"warning","servicename":"payments","instance":"c"},"status":{"state":"active"}},
				{"fingerprint":"with-state","labels":{"alertname":"Unrelated","severity":"info"},"status":{"state":"inhibited"}}
			]`)
		case "/am/api/v2/status":
			config := `inhibit_rules:
- source_matchers: ['severity="critical"']
  target_matchers: ['severity="warning"']
  equal: [alertname, servicename]`
			_ = json.NewEncoder(w).Encode(map[string]any{"config": map[string]string{"original": config}})
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	m := New(Options{Timeout: time.Second, Connection: config.Connection{AlertmanagerURL: server.URL + "/am"}}, nil)
	m.resize(80, 22)
	now := time.Now()
	cmd := pressAlt(m, '1')
	m.onAlertsCompleted(cmd().(alertsCompleted), now)
	a := &m.alerts
	a.filter, a.sort = "warning", sortAlertNewest
	a.rebuild(a.rows[1].id, 0, now)
	selected := a.selection()
	press(m, tea.KeyEnter, tea.KeyPgDown)
	offset := a.offset
	detailText := func() string {
		_, rows, _, _ := m.alertDialog(now)
		return ansi.Strip(strings.Join(rows, "\n"))
	}
	text := detailText()
	for _, want := range []string{"State: inhibited", "Confirmed source: HighLatency", "Confirmed source unavailable: missing",
		"Matching rule 1 (local evaluation)", `severity="critical"`, "equal:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in\n%s", want, text)
		}
	}
	if !strings.Contains(ansi.Strip(m.footer()), "i Related") {
		t.Fatal(m.footer())
	}
	press(m, 'i')
	if !strings.Contains(ansi.Strip(m.screen()), "› Confirmed source: HighLatency") {
		t.Fatal(m.screen())
	}
	press(m, tea.KeyEnter)
	if a.detail.Fingerprint != "source" || !strings.Contains(detailText(), "Inhibits (2 confirmed)") {
		t.Fatal("could not follow source hidden by filter")
	}
	press(m, 'i', tea.KeyDown, tea.KeyEnter)
	if a.detail.Fingerprint != "target-b" {
		t.Fatal("could not select second confirmed target")
	}
	press(m, tea.KeyEsc, tea.KeyEsc)
	if a.detail.Fingerprint != "target-a" || a.offset != offset {
		t.Fatal("back did not restore original details and scroll")
	}
	press(m, 'i', tea.KeyEsc, tea.KeyEsc)
	if a.dialog != alertNoDialog || a.selection() != selected || a.filter != "warning" || a.sort != sortAlertNewest ||
		m.opt.Connection.AlertmanagerURL != server.URL+"/am" || calls.Load() != 2 {
		t.Fatal("navigation altered table context or fetched data")
	}
	a.filter = ""
	a.rebuild(a.rows[3].id, 0, now)
	press(m, tea.KeyEnter)
	if text := detailText(); !strings.Contains(text, "Potential source: HighLatency") || strings.Contains(text, "Confirmed source") {
		t.Fatal(text)
	}
	press(m, tea.KeyEsc)
	a.rebuild(a.rows[4].id, 0, now)
	press(m, tea.KeyEnter)
	if text := detailText(); !strings.Contains(text, "No runtime source reference reported.") {
		t.Fatal(text)
	}
}

func TestAlertInhibitionUnavailableConfigIsNonfatal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/status" {
			http.Error(w, "private upstream details", http.StatusForbidden)
			return
		}
		fmt.Fprint(w, `[
			{"fingerprint":"source","labels":{"alertname":"Source"},"status":{"state":"active"}},
			{"fingerprint":"target","labels":{"alertname":"Target"},"status":{"state":"suppressed","inhibitedBy":["source"]}}
		]`)
	}))
	defer server.Close()
	m := New(Options{Timeout: time.Second, Connection: config.Connection{AlertmanagerURL: server.URL}}, nil)
	m.resize(80, 22)
	now := time.Now()
	cmd := pressAlt(m, '1')
	m.onAlertsCompleted(cmd().(alertsCompleted), now)
	press(m, tea.KeyEnd, tea.KeyEnter)
	if m.alerts.err != nil || m.alerts.detail.Fingerprint != "target" {
		t.Fatal("status error disrupted Alerts")
	}
	_, rows, _, _ := m.alertDialog(now)
	text := ansi.Strip(strings.Join(rows, "\n"))
	if !strings.Contains(text, "Rule explanations unavailable") || !strings.Contains(text, "HTTP 403") ||
		strings.Contains(text, "private upstream details") || !strings.Contains(text, "Confirmed source: Source") {
		t.Fatal(text)
	}
	press(m, 'i', tea.KeyEnter)
	if m.alerts.detail.Fingerprint != "source" {
		t.Fatal("status error prevented runtime link navigation")
	}
	if m.onAlertsTick(now.Add(alertRefreshInterval)) == nil {
		t.Fatal("background refresh did not start")
	}
	a := &m.alerts
	m.onAlertsCompleted(alertsCompleted{id: a.requestID, err: alertmanager.ErrAuth}, now.Add(alertRefreshInterval))
	if a.err != nil || a.refreshErr == nil || a.dialog != alertDetails || a.detail.Fingerprint != "source" || len(a.visible) != 2 {
		t.Fatal("failed background refresh did not preserve the alert snapshot")
	}
	press(m, tea.KeyEsc)
	if a.detail.Fingerprint != "target" {
		t.Fatal("failed background refresh broke back navigation")
	}
}
