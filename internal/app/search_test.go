package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/api"
	"promtui/internal/config"
	"promtui/internal/savedalerts"
)

func TestSharedSearch(t *testing.T) {
	fields := map[string]string{
		"alertname": "NodeDown", "severity": "critical", "labels.team.zone": "Production",
		"annotations.summary": `Connection refused "retry"`, "custom_label": "Payment Service",
	}
	all := func(f fuzzyFilter) bool {
		for _, value := range fields {
			if f.matches(value) {
				return true
			}
		}
		return f.matches("https://example.org", "10.0.0.1:1234", "env=prod")
	}
	var search parsedSearch
	for _, tc := range []struct {
		query string
		match bool
	}{
		{"", true}, {" \t ", true}, {"ndwn", true},
		{"NODE critical production", true}, {"node critical staging", false},
		{"down node", true}, {"node critical", true}, {`"node critical"`, false},
		{"node severity:CRIT", true}, {"severity:node", false}, {"unknown:critical", false},
		{`custom_label:"PAYMENT SERVICE" labels.team.zone:prod`, true},
		{`annotations.summary:"connection refused"`, true},
		{`annotations.summary:"connection refused`, true},
		{`annotations.summary:"refused \"retry\""`, true},
		{`"connection refused" severity:critical`, true},
		{`"connection refused" severity:warning`, false},
		{`https://example.org 10.0.0.1:1234 env=prod`, true},
		{"", true},
	} {
		search.update(tc.query)
		if got := search.matches(all, func(field string) string { return fields[field] }); got != tc.match {
			t.Errorf("%q matched = %v, want %v", tc.query, got, tc.match)
		}
	}
}

func TestSearchAcrossTabsAndTargetSources(t *testing.T) {
	conn := config.Connection{AlertmanagerURL: "http://alerts", Targets: []config.TargetSource{{Name: "east"}, {Name: "west"}}}
	m := New(Options{Connection: conn, AlertViews: map[string]config.TableView{
		"compact": {Columns: []config.Column{{Field: "state"}}},
	}}, nil)
	m.resize(80, 20)
	m.activeTab = alertsTab
	now := time.Now()
	m.alerts.lastOK, m.targets.lastDone = now, now
	alert := testAlert("one", "HaproxyHasNoAliveBackends", "critical", "active", now)
	alert.Labels["servicename"] = "okd4-sov-test"
	alert.Labels["custom_label"] = "frontend"
	alert.Labels["state"] = "custom"
	alert.Annotations = map[string]string{"summary": "connection refused"}
	other := testAlert("two", "HaproxyHasNoAliveBackends", "critical", "active", now)
	other.Labels["servicename"] = "staging"
	m.alerts.rows = []alertRow{newAlertRow("test", alert), newAlertRow("test", other)}
	m.alerts.rebuild("", 0, now)
	m.selectAlertView("compact")

	alertQuery := `HaproxyHasNoAliveBackends okd4-sov-test annotations.summary:"connection refused" custom_label:front state:active labels.state:custom`
	press(m, '/')
	savedPaste(m, alertQuery)
	press(m, tea.KeyEnter)
	if len(m.alerts.visible) != 1 || m.alerts.visible[0].Fingerprint != "one" {
		t.Fatal("compound alert search depended on visible columns or matched across rows")
	}

	for i, source := range m.targets.sources {
		m.targets.sources[i].rows = makeTargetRows(source.Name, api.Targets{Active: []api.Target{
			{Health: "down", ScrapeURL: "http://localhost/metrics", Labels: map[string]string{"job": "vmagent", "servicename": "postgresql", "health": "up"}},
			{Health: "up", Labels: map[string]string{"servicename": "postgresql"}},
			{Health: "down", Labels: map[string]string{"servicename": "payments"}},
		}})
	}
	m.targets.rebuild("", 0)
	targetQuery := "servicename:postgresql health:down labels.health:up scrapeUrl:metrics vmagent"
	press(m, '3', '/')
	savedPaste(m, targetQuery)
	press(m, tea.KeyEnter)
	if got := m.targets.visible; len(got) != 2 || got[0].source != "east" || got[1].source != "west" {
		t.Fatal("expected one match per target source, with built-ins taking precedence over labels")
	}
	press(m, '/', ' ', 'x', tea.KeyEsc, '1')
	if m.alerts.filter != alertQuery || len(m.alerts.visible) != 1 {
		t.Fatal("editing Targets changed Alerts search")
	}
	press(m, '3')
	if m.targets.filter != targetQuery || len(m.targets.visible) != 2 {
		t.Fatal("tab switch or Escape lost Targets search")
	}
}

func TestAlertCompletionReplacesValueAtCursor(t *testing.T) {
	a := newAlertsState(config.TableView{})
	now := time.Now()
	row := testAlert("one", "NodeDown", "critical", "active", now)
	row.Labels["servicename"] = "okd4-sov-test"
	row.Annotations = map[string]string{"summary": "connection refused"}
	a.rows = []alertRow{newAlertRow("test", row)}
	a.refreshAlertNames()
	a.input.Focus()
	for _, tc := range []struct{ before, active, value, after string }{
		{"NodeDown okd", "NodeDown okd", "okd4-sov-test", "NodeDown okd4-sov-test"},
		{"NodeDown servicename:okd", "NodeDown servicename:okd", "okd4-sov-test", "NodeDown servicename:okd4-sov-test"},
		{`labels.zone:西 annotations.summary:"conn ref" severity:critical`, `labels.zone:西 annotations.summary:"conn`,
			"connection refused", `labels.zone:西 annotations.summary:"connection refused" severity:critical`},
	} {
		a.input.SetValue(tc.before)
		a.input.SetCursor(len([]rune(tc.active)))
		a.refreshSearchSuggestions()
		if !slices.Equal(a.suggestions.matches, []string{tc.value}) || a.searchSuggestionKey("enter") {
			t.Fatalf("%q: wrong suggestions or implicit selection: %v", tc.before, a.suggestions.matches)
		}
		if !a.searchSuggestionKey("down") || !a.searchSuggestionKey("enter") || a.input.Value() != tc.after {
			t.Fatalf("completed to %q, want %q", a.input.Value(), tc.after)
		}
	}
}

func TestEditedAlertSearchHidesSavedNameButKeepsMatchers(t *testing.T) {
	conn := config.Connection{AlertmanagerURL: "http://alerts", AlertDefaultView: "default"}
	m := New(Options{Connection: conn, AlertViews: map[string]config.TableView{"default": {}}}, nil)
	m.resize(80, 20)
	m.activeTab = alertsTab
	now := time.Now()
	m.alerts.lastOK = now
	for _, zone := range []string{"west", "east"} {
		alert := testAlert(zone, "NodeDown", "critical", "active", now)
		alert.Labels["zone"] = zone
		m.alerts.rows = append(m.alerts.rows, newAlertRow("test", alert))
	}
	m.alerts.rebuild("", 0, now)
	savedText(t, m.screen(), "Alerts | View: default")
	m.loadAlertFilter(savedalerts.Filter{Name: "West Nodes", Matchers: []savedalerts.Matcher{
		{Label: "zone", Operator: "=", Value: "west"},
	}})
	savedText(t, m.screen(), "Search: West Nodes")
	for width := minWidth; width < 50; width++ {
		m.resize(width, 20)
		border := strings.Split(ansi.Strip(m.screen()), "\n")[1]
		if ansi.StringWidth(border) != width || !strings.Contains(border, "Search:") {
			t.Fatalf("width %d: invalid saved-search header %q", width, border)
		}
	}
	m.resize(80, 20)
	press(m, '/')
	savedPaste(m, "NodeDown severity:critical")
	if strings.Contains(ansi.Strip(m.screen()), "Search: West Nodes") {
		t.Fatal("live edit showed stale saved-search name")
	}
	press(m, tea.KeyEsc)
	savedText(t, m.screen(), "Search: West Nodes")
	press(m, '/')
	savedPaste(m, "NodeDown severity:critical")
	press(m, tea.KeyEnter)
	if strings.Contains(ansi.Strip(m.screen()), "Search: West Nodes") ||
		len(m.alerts.visible) != 1 || m.alerts.visible[0].Fingerprint != "west" {
		t.Fatal("committed edit kept stale metadata or dropped the active matchers")
	}
	savedText(t, m.screen(), "Filter: NodeDown severity:critical")
}
