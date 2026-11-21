package app

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/alertmanager"
	"promtui/internal/api"
	"promtui/internal/config"
)

func checkViewHeader(t *testing.T, screen, want string) {
	t.Helper()
	lines := strings.Split(ansi.Strip(screen), "\n")
	if got := strings.Join(strings.Fields(strings.Trim(lines[4], "│")), " "); got != want {
		t.Fatalf("headings = %q, want %q\n%s", got, want, screen)
	}
}

func TestCustomAlertView(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	conn := config.Connection{Name: "prod", AlertmanagerURL: "http://alertmanager",
		Alerts: config.TableView{
			Columns: []config.Column{{Field: "labels.app.service", Title: "My Service"}, {Field: "state"}, {Field: "duration"}},
			Search:  []string{"labels.hidden", "labels.other"},
		}}
	m := New(Options{Connection: conn}, nil)
	m.resize(80, 14)
	press(m, '1')
	defer m.alerts.stop()

	z := testAlert("z", "ExcludedAlert", "critical", "active", now.Add(-65*time.Second))
	z.Labels["app.service"], z.Labels["hidden"], z.Labels["other"] = "zebra", "ops-team", "server"
	a := testAlert("a", "Alpha", "high", "suppressed", now.Add(-10*time.Minute))
	a.Labels["app.service"] = "ant"
	a.Status.SilencedBy, a.Status.InhibitedBy = []string{"silence"}, []string{"inhibit"}
	missing := testAlert("missing", "Other", "", "active", time.Time{})
	rows := []alertmanager.Alert{z, a, missing}
	m.onAlertsCompleted(alertsCompleted{id: m.alerts.requestID, rows: rows}, now)
	screen, _ := m.alertsScreen(now)
	checkViewHeader(t, screen, "My Service State Duration")
	for _, text := range []string{"zebra", "1m5s", "Silenced + Inhibited", "—"} {
		if !strings.Contains(ansi.Strip(screen), text) {
			t.Fatalf("missing %q:\n%s", text, screen)
		}
	}

	selected := m.alerts.selection()
	press(m, 's', 's')
	if m.alerts.sort != "labels.app.service" || m.alerts.selection() != selected || m.alerts.visible[0].Fingerprint != "a" ||
		m.alerts.visible[2].Fingerprint != "missing" {
		t.Fatal("custom label should sort ascending, put missing last and keep selection")
	}
	press(m, 's', 's')
	if m.alerts.sort != "duration" || m.alerts.visible[0].Fingerprint != "a" {
		t.Fatal("duration must sort by elapsed time, not by its formatted text")
	}
	press(m, 's')
	if m.alerts.sort != sortAlertDefault {
		t.Fatal("sort did not wrap to the existing default")
	}
	for _, text := range []string{"zebra", "ExcludedAlert", "opsserver", "—"} {
		press(m, '/')
		m.Update(tea.PasteMsg{Content: text})
		if len(m.alerts.visible) != 0 {
			t.Errorf("%q should not match configured fields", text)
		}
		press(m, tea.KeyEsc)
	}
	press(m, '/')
	m.Update(tea.PasteMsg{Content: "opstm"})
	press(m, tea.KeyEnter)
	if len(m.alerts.visible) != 1 || m.alerts.visible[0].Fingerprint != "z" {
		t.Fatal("hidden label did not match")
	}
	id := m.alerts.selection()
	now = now.Add(alertRefreshInterval)
	if m.onAlertsTick(now) == nil {
		t.Fatal("expected automatic refresh")
	}
	m.onAlertsCompleted(alertsCompleted{id: m.alerts.requestID, rows: rows}, now)
	if len(m.alerts.visible) != 1 || m.alerts.selection() != id || m.alerts.filter != "opstm" {
		t.Fatal("refresh lost custom search or selection")
	}
	press(m, tea.KeyEnter)
	if m.alerts.dialog != alertDetails || m.alerts.detail.Fingerprint != "z" {
		t.Fatal("details lost the selected alert")
	}
}

func TestCustomTargetView(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	conn := config.Connection{Targets: []config.TargetSource{{Name: "east"}, {Name: "west"}},
		TargetsView: config.TableView{
			Columns: []config.Column{{Field: "labels.servicename"}, {Field: "health"},
				{Field: "last_scrape", Title: "Age"}, {Field: "scrape_url", Title: "Endpoint"}},
			Search: []string{"labels.hidden", "labels.other"},
		}}
	m := New(Options{Connection: conn}, nil)
	m.resize(96, 14)
	press(m, '3')
	defer m.targets.stop(nil)
	target := func(health, service, hidden, timestamp string) api.Target {
		return api.Target{
			Labels:           map[string]string{"servicename": service, "hidden": hidden, "other": "backend"},
			DiscoveredLabels: map[string]string{"hidden": "discovery-only"},
			Health:           health, LastScrape: timestamp, ScrapeURL: "http://host/metrics",
		}
	}
	east := api.Targets{Active: []api.Target{target("down", "zebra", "ops-team", "2025-12-31T23:59:58Z")}}
	west := api.Targets{Active: []api.Target{target("up", "ant", "other-group", "2026-01-01T01:59:50+02:00"),
		target("up", "", "", "")}, Dropped: []api.Target{{DiscoveredLabels: map[string]string{"servicename": "not-final"}}}}
	complete := func(at time.Time) {
		t.Helper()
		m.onTargetsCompleted(targetsCompleted{id: m.targets.requestID, source: 1, data: west}, at)
		m.onTargetsCompleted(targetsCompleted{id: m.targets.requestID, source: 0, data: east}, at)
	}
	complete(now)
	screen, _ := m.targetsScreen(now)
	checkViewHeader(t, screen, "SERVICENAME Health Age Endpoint")
	for _, text := range []string{"zebra", "ant", "2s ago", "—", "http://host/metrics"} {
		if !strings.Contains(ansi.Strip(screen), text) {
			t.Fatalf("missing %q:\n%s", text, screen)
		}
	}
	id := m.targets.selection()
	for range len(defaultTargetSorts) {
		press(m, 's')
	}
	got := func() []string {
		var values []string
		for _, row := range m.targets.visible {
			values = append(values, row.Labels["servicename"])
		}
		return values
	}
	if m.targets.sort != "labels.servicename" || m.targets.selection() != id || !slices.Equal(got(), []string{"ant", "zebra", ""}) {
		t.Fatal("aggregated label sort lost rows, order, or selection")
	}
	press(m, 's')
	if m.targets.sort != "last_scrape" || !slices.Equal(got(), []string{"zebra", "ant", ""}) {
		t.Fatal("scrape times should sort newest first by timestamp, with missing last")
	}
	for _, text := range []string{"zebra", "discovery-only", "opsbackend", "hidden=ops-team", "—"} {
		press(m, '/')
		m.Update(tea.PasteMsg{Content: text})
		if len(m.targets.visible) != 0 {
			t.Errorf("%q should not match configured fields", text)
		}
		press(m, tea.KeyEsc)
	}
	press(m, '/')
	m.Update(tea.PasteMsg{Content: "opstm"})
	press(m, tea.KeyEnter, 'h')
	if len(m.targets.visible) != 1 || m.targets.visible[0].source != "east" {
		t.Fatal("hidden label search should combine with health filter")
	}
	id = m.targets.selection()
	now = now.Add(targetRefreshInterval)
	if m.onTargetsTick(now) == nil {
		t.Fatal("expected automatic refresh")
	}
	complete(now)
	if len(m.targets.visible) != 1 || m.targets.selection() != id || m.targets.filter != "opstm" {
		t.Fatal("refresh lost custom search or selection")
	}
	press(m, tea.KeyEnter)
	if m.targets.dialog != targetDetails || m.targets.detail.source != "east" {
		t.Fatal("details lost internal source identity")
	}
	press(m, tea.KeyEsc, '/')
	m.targets.input.SetValue("")
	m.targets.rebuild("", 0)
	press(m, tea.KeyEnter, 'd')
	if len(m.targets.visible) != 1 || m.targets.visible[0].field("labels.servicename") != "" {
		t.Fatal("custom fields must not use discovered labels on a dropped target")
	}
}

func TestTableViewsResetOnConnectionSwitch(t *testing.T) {
	now := time.Now()
	custom := config.Connection{Name: "custom", URL: "http://query", AlertmanagerURL: "http://alerts",
		Alerts:      config.TableView{Columns: []config.Column{{Field: "labels.team"}}, Search: []string{"labels.team"}},
		TargetsView: config.TableView{Columns: []config.Column{{Field: "labels.team"}}, Search: []string{"labels.team"}}}
	plain := config.Connection{Name: "plain", URL: custom.URL, AlertmanagerURL: custom.AlertmanagerURL}
	m := New(Options{Connection: custom, Connections: []config.Connection{custom, plain}}, nil)
	m.resize(90, 14)
	defer m.alerts.stop()
	defer m.targets.stop(nil)
	a := testAlert("id", "AlertName", "critical", "active", now)
	a.Labels["team"] = "platform"
	populateAlerts := func() {
		t.Helper()
		m.onAlertsCompleted(alertsCompleted{id: m.alerts.requestID, rows: []alertmanager.Alert{a}}, now)
	}
	populateTargets := func() {
		t.Helper()
		m.onTargetsCompleted(targetsCompleted{id: m.targets.requestID, data: api.Targets{
			Active: []api.Target{{Labels: map[string]string{"team": "platform", "instance": "node01"}}},
		}}, now)
	}
	press(m, '1')
	populateAlerts()
	checkViewHeader(t, m.screen(), "TEAM")
	press(m, '3')
	populateTargets()
	checkViewHeader(t, m.screen(), "TEAM")
	press(m, '/', 'p', 'l', 't', tea.KeyEnter)
	press(m, 'c', tea.KeyDown, tea.KeyEnter)
	populateTargets()
	checkViewHeader(t, m.screen(), "Health Instance Job Source Last scrape Duration")
	if m.targets.filter != "" || !m.targets.matches(m.targets.visible[0], newFuzzyFilter("node01")) {
		t.Fatal("default target search did not replace custom state")
	}
	press(m, '1')
	populateAlerts()
	checkViewHeader(t, m.screen(), "State Severity Alert Instance Duration")
	if !m.alerts.matches(m.alerts.visible[0], newFuzzyFilter("alrtnm"), now) ||
		m.alerts.matches(m.alerts.visible[0], newFuzzyFilter("platform"), now) {
		t.Fatal("default alert search did not replace custom fields")
	}
	press(m, 'c', tea.KeyUp, tea.KeyEnter)
	populateAlerts()
	checkViewHeader(t, m.screen(), "TEAM")
	if !m.alerts.matches(m.alerts.visible[0], newFuzzyFilter("pltfrm"), now) ||
		m.alerts.matches(m.alerts.visible[0], newFuzzyFilter("alrtnm"), now) {
		t.Fatal("switching back did not restore custom search")
	}
}

func TestConfiguredTableFitsNarrowWidths(t *testing.T) {
	var cols []config.Column
	for i := range 100 {
		cols = append(cols, config.Column{Field: fmt.Sprintf("labels.%d", i), Title: "多字 heading"})
	}
	for w := 3; w <= 80; w++ {
		table := layoutViewTable(config.TargetsView, cols, w)
		if len(table.cols) != w-2 {
			t.Fatalf("width %d: got %d columns", w, len(table.cols))
		}
		row := table.row(true, func(string, int) (string, color.Color) { return "values 多字", nil })
		if ansi.StringWidth(table.header()) != w || ansi.StringWidth(row) != w || !strings.HasPrefix(ansi.Strip(row), "› ") {
			t.Fatalf("width %d: invalid header or selection: %q | %q", w, table.header(), row)
		}
	}
}

func TestCustomColumnsKeepDefaultSearch(t *testing.T) {
	now := time.Now()
	cols := []config.Column{{Field: "labels.servicename"}}
	alerts := newAlertsState(config.TableView{Columns: cols})
	alert := newAlertRow("source", testAlert("id", "HighCPU", "critical", "active", now))
	alert.Labels["servicename"] = "billing"
	if !alerts.matches(alert, newFuzzyFilter("hcpu"), now) || alerts.matches(alert, newFuzzyFilter("billing"), now) {
		t.Fatal("custom columns changed default alert search")
	}
	targets := newTargetsState(config.Connection{TargetsView: config.TableView{Columns: cols}})
	target := makeTargetRows("source", api.Targets{Active: []api.Target{{DiscoveredLabels: map[string]string{"region": "north"}}}})
	if !targets.matches(&target[0], newFuzzyFilter("region=north")) {
		t.Fatal("custom columns changed default target search")
	}
}
