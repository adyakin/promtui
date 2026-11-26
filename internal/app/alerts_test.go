package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/alertmanager"
	"promtui/internal/config"
)

func testAlert(id, name, severity, state string, start time.Time) alertmanager.Alert {
	a := alertmanager.Alert{
		Fingerprint: id,
		Labels:      map[string]string{"alertname": name, "severity": severity},
		StartsAt:    start,
	}
	a.Status.State = state
	return a
}

func TestAlertRows(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	early := now.Add(-2 * time.Hour)
	recent := now.Add(-time.Minute)
	rows := []alertmanager.Alert{
		testAlert("late", "Duplicate", "critical", "active", recent),
		testAlert("early", "Duplicate", "CRITICAL", "active", early),
		testAlert("inhib", "Inhibited", "critical", "suppressed", recent),
		testAlert("both", "Both", "warning", "suppressed", recent),
		testAlert("sil", "Silenced", "info", "suppressed", recent),
		testAlert("queue", "Queued", "urgent", "unprocessed", early),
		testAlert("misc", "Misc", "urgent", "active", early),
		testAlert("unknown", "", "", "", time.Time{}),
	}
	rows[2].Status.InhibitedBy = []string{"parent"}
	rows[3].Status.SilencedBy, rows[3].Status.InhibitedBy = []string{"silence"}, []string{"parent"}
	rows[4].Status.SilencedBy = []string{"silence"}
	a := newAlertsState(config.TableView{})
	for _, row := range rows {
		a.rows = append(a.rows, newAlertRow("source", row))
	}
	if c := countAlerts(a.rows); c != (alertCounts{8, 3, 2, 2}) {
		t.Fatal(c)
	}
	if a.rows[3].state() != "Silenced + Inhibited" || a.rows[5].state() != "Unprocessed" || a.rows[7].state() != "Unknown" {
		t.Fatal("incorrect state labels")
	}
	checkOrder := func(want ...string) {
		t.Helper()
		var got []string
		for _, row := range a.visible {
			got = append(got, row.Fingerprint)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	a.rebuild("", 0, now)
	checkOrder("early", "late", "misc", "both", "sil", "inhib", "queue", "unknown")
	a.sort = sortAlertNewest
	a.rebuild(a.selection(), a.row, now)
	checkOrder("both", "inhib", "late", "sil", "early", "misc", "queue", "unknown")
	if a.row != 4 {
		t.Fatalf("sort lost selection: %d", a.row)
	}
	a.filter = "slncd"
	a.rebuild(a.selection(), a.row, now)
	checkOrder("both", "sil")
	a.filter = "CRIT"
	a.rebuild(a.selection(), a.row, now)
	checkOrder("inhib", "late", "early")
}

func TestAlertFilterPreservesSingleTermScope(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	critical := testAlert("fingerprint-warning", "HaproxyHasNoAliveBackends", "critical", "active", now.Add(-8*24*time.Hour-9*time.Hour))
	critical.Labels["instance"] = "proxy-west"
	critical.Labels["team"] = "certificate-ops"
	critical.Annotations = map[string]string{"summary": "Certificate warning", "runbook": "Fleet recovery"}
	critical.GeneratorURL = "https://prometheus.example/rule"
	warning := testAlert("warning", "CertCheck", "warning", "active", now.Add(-6*time.Hour))
	warning.Labels["instance"] = "node-east"

	a := newAlertsState(config.TableView{})
	for _, alert := range []alertmanager.Alert{critical, warning} {
		a.rows = append(a.rows, newAlertRow("https://cluster-west-router", alert))
	}
	for _, tc := range []struct{ filter, want string }{
		{"Cert", "warning"},
		{"warn", "warning"},
		{"crtchk", "warning"},
		{"prxywst", critical.Fingerprint},
		{"8d9h", critical.Fingerprint},
		{"runbook", ""},
		{"fleet recovery", critical.Fingerprint},
		{"certificate-ops", ""},
		{"prometheus", ""},
		{"fingerprint", ""},
	} {
		a.filter = tc.filter
		a.rebuild("", 0, now)
		if tc.want == "" && len(a.visible) == 0 ||
			tc.want != "" && len(a.visible) == 1 && a.visible[0].Fingerprint == tc.want {
			continue
		}
		t.Errorf("filter %q: got %v, want %q", tc.filter, a.visible, tc.want)
	}
}

func TestAlertIdentityAndDuration(t *testing.T) {
	a := alertmanager.Alert{Labels: map[string]string{"alertname": "Down", "instance": "one"}}
	b := alertmanager.Alert{Labels: map[string]string{"instance": "one", "alertname": "Down"}}
	key := newAlertRow("am", a).id
	if newAlertRow("am", b).id != key || newAlertRow("other", b).id == key {
		t.Fatal("fallback should use a source and a canonical label set")
	}
	b.Labels["instance"] = "two"
	if newAlertRow("am", b).id == key {
		t.Fatal("fallback lost instance label")
	}
	a.Fingerprint, b.Fingerprint = "123", "123"
	if newAlertRow("am", a).id != newAlertRow("am", b).id || newAlertRow("am", a).id == newAlertRow("other", b).id {
		t.Fatal("fingerprints must be primary and scoped to a source")
	}
	if text := alertLabelText(b.Labels); text != `{alertname="Down", instance="two"}` {
		t.Fatal(text)
	}
	now := time.Now()
	for start, want := range map[time.Time]string{
		{}: "—", now.Add(time.Minute): "0s", now.Add(-9 * time.Second): "9s",
		now.Add(-65 * time.Second): "1m5s", now.Add(-91 * time.Minute): "1h31m", now.Add(-48 * time.Hour): "2d0h",
	} {
		if got := alertDuration(start, now); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

func TestAlertsNavigationFilterAndOverlay(t *testing.T) {
	m := New(Options{Connection: config.Connection{AlertmanagerURL: "http://alertmanager"}}, nil)
	if pressAlt(m, '1') == nil || m.activeTab != alertsTab {
		t.Fatal("expected initial fetch on tab switch")
	}
	defer m.alerts.stop()
	now := time.Now()
	const annotation = "first line\n" + "longwordlongwordlongwordlongwordlongwordlongwordlongwordlongword"
	var rows []alertmanager.Alert
	for i := range 14 {
		a := testAlert(fmt.Sprint(i), fmt.Sprintf("Alert-%02d", i), "critical", "active", now.Add(time.Duration(i-30)*time.Minute))
		a.Labels["instance"] = "target"
		a.Annotations = map[string]string{"description": annotation}
		rows = append(rows, a)
	}
	m.onAlertsCompleted(alertsCompleted{id: m.alerts.requestID, rows: rows}, now)
	m.resize(80, 13)
	press(m, tea.KeyPgDown)
	if m.alerts.row != m.alertPage() || !strings.Contains(ansi.Strip(m.screen()), "Alert-05") {
		t.Fatal("page down failed to keep selected row visible")
	}
	press(m, tea.KeyEnd)
	wantID := m.alerts.selection()
	before, _ := m.alertsScreen(now)
	press(m, tea.KeyEnter)
	opened, _ := m.alertsScreen(now)
	b, o := strings.Split(ansi.Strip(before), "\n"), strings.Split(ansi.Strip(opened), "\n")
	if len(b) != len(o) || strings.TrimRight(b[0], " ") != strings.TrimRight(o[0], " ") || b[1] != o[1] || !strings.Contains(ansi.Strip(opened), "Alert details") ||
		!strings.Contains(ansi.Strip(m.footer()), "y Copy labels") || strings.Contains(ansi.Strip(m.footer()), "/ Filter") {
		t.Fatalf("details did not overlay the unchanged layout with contextual controls:\n%s", ansi.Strip(opened))
	}
	press(m, tea.KeyPgDown)
	if m.alerts.offset == 0 {
		t.Fatal("details did not scroll")
	}
	press(m, '?')
	if !strings.Contains(m.screen(), "Alerts help") {
		t.Fatal("details help did not open")
	}
	press(m, tea.KeyEsc)
	if m.alerts.dialog != alertDetails || m.alerts.offset == 0 {
		t.Fatal("help changed details state")
	}
	m.resize(24, 10)
	_, wrapped, width, _ := m.alertDialog(now)
	if !slices.ContainsFunc(wrapped, func(line string) bool { return strings.Contains(line, "longword") }) {
		t.Fatal("annotation lost when wrapping")
	}
	for _, line := range wrapped {
		if ansi.StringWidth(line) > width-4 {
			t.Fatalf("oversized dialog line %q", line)
		}
	}
	m.resize(80, 13)
	press(m, tea.KeyEsc)
	after, _ := m.alertsScreen(now)
	if before != after {
		t.Fatal("closing overlay did not restore table")
	}
	press(m, '/', 'A', '-', '0', '3', 'q')
	if m.quitting || m.activeTab != alertsTab || m.alerts.input.Value() != "A-03q" || len(m.alerts.visible) != 0 || m.alerts.cancel != nil {
		t.Fatal("filter input did not behave as cached text editing")
	}
	press(m, tea.KeyBackspace)
	if len(m.alerts.visible) != 1 || m.alerts.visible[0].Fingerprint != "3" || m.View().Cursor == nil {
		t.Fatal("interactive filter or cursor missing")
	}
	pressAlt(m, '0')
	pressAlt(m, '1')
	if !m.alerts.input.Focused() || m.alerts.input.Value() != "A-03" {
		t.Fatal("tab switch lost filter editing")
	}
	press(m, tea.KeyEsc)
	if m.alerts.currentFilter() != "" || m.alerts.selection() != wantID {
		t.Fatal("filter cancel lost previous selection")
	}
	press(m, '/')
	m.Update(tea.PasteMsg{Content: "TARGET"})
	press(m, tea.KeyEnter, '2', '1', tea.KeyHome)
	if m.alerts.filter != "TARGET" || len(m.alerts.visible) != len(rows) || m.alerts.row != 0 {
		t.Fatal("applied filter or keyboard navigation did not persist")
	}
}

func TestAlertsRefreshLifecycle(t *testing.T) {
	const interval = 17 * time.Second
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/prefix/api/v2/status" {
			fmt.Fprint(w, `{"config":{"original":"{}"}}`)
			return
		}
		if r.URL.Path != "/prefix/api/v2/alerts" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		fmt.Fprint(w, `[{"fingerprint":"a","labels":{"alertname":"First"},"status":{"state":"active"}},
			{"fingerprint":"b","labels":{"alertname":"Second"},"status":{"state":"active"}}]`)
	}))
	defer server.Close()
	m := New(Options{Timeout: time.Second, AutoRefresh: config.AutoRefresh{Alerts: interval}, Connection: config.Connection{AlertmanagerURL: server.URL + "/prefix"}}, nil)
	now := time.Now()
	first := pressAlt(m, '1')
	if first == nil || m.refreshAlerts() != nil {
		t.Fatal("overlapping refresh")
	}
	if tick := m.onAlertsCompleted(first().(alertsCompleted), now); tick == nil || m.scheduleAlertsTick(now) != nil {
		t.Fatal("expected exactly one scheduled refresh tick")
	}
	press(m, tea.KeyEnd, tea.KeyEnter)
	if m.alerts.detail.Fingerprint != "b" {
		t.Fatal("wrong initial selection")
	}
	auto := m.onPaneRefreshTick(m.paneTimer.id, now.Add(interval))
	if auto == nil || m.refreshAlerts() != nil {
		t.Fatal("automatic refresh not started or duplicate started")
	}
	newRows := []alertmanager.Alert{
		testAlert("a", "First", "info", "active", now),
		testAlert("b", "Changed", "critical", "active", now),
	}
	m.onAlertsCompleted(alertsCompleted{id: m.alerts.requestID, rows: newRows}, now.Add(interval))
	if m.alerts.row != 0 || m.alerts.visible[0].Fingerprint != "b" || m.alerts.detail.Labels["alertname"] != "Second" {
		t.Fatal("automatic refresh lost the stable selection or changed open details")
	}
	press(m, tea.KeyEsc, tea.KeyEnd)
	_ = m.refreshAlerts()
	m.onAlertsCompleted(alertsCompleted{id: m.alerts.requestID, rows: newRows[1:]}, now.Add(interval))
	if m.alerts.row != 0 || len(m.alerts.visible) != 1 || m.alerts.visible[0].Fingerprint != "b" {
		t.Fatal("disappeared selection did not move to nearest row")
	}
	_ = m.refreshAlerts()
	m.onAlertsCompleted(alertsCompleted{id: m.alerts.requestID, err: alertmanager.ErrAuth}, now.Add(interval))
	if screen := ansi.Strip(m.screen()); len(m.alerts.visible) != 1 ||
		!strings.Contains(screen, "│  Authentication failed") || strings.Contains(screen, "Changed") {
		t.Fatalf("error did not replace cached data in the pane:\n%s", screen)
	}
	pressAlt(m, 's')
	if !strings.Contains(m.screen(), "Alerts status") {
		t.Fatal("no error details")
	}
	press(m, tea.KeyEsc)
	aborted := m.refreshAlerts()
	press(m, '2')
	if obsolete := aborted().(alertsCompleted); !errors.Is(obsolete.err, context.Canceled) ||
		m.onAlertsCompleted(obsolete, now) != nil || !errors.Is(m.alerts.err, alertmanager.ErrAuth) {
		t.Fatal("obsolete request changed the alert cache")
	}
	if m.onAlertsTick(now.Add(2*interval)) != nil {
		t.Fatal("automatically refreshed hidden Alerts")
	}
	m.activeTab = alertsTab
	if m.alertsOnEntry(now.Add(2*interval)) == nil {
		t.Fatal("stale data not refreshed on return")
	}
	m.onAlertsCompleted(alertsCompleted{id: m.alerts.requestID}, now.Add(2*interval))
	if !strings.Contains(m.screen(), "No active alerts") || m.alerts.err != nil {
		t.Fatal("successful empty result not distinguished from a failure")
	}
}
