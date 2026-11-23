package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/alertmanager"
	"promtui/internal/config"
)

func TestAlertsOpenPreservesFilterViewAndConnectionContext(t *testing.T) {
	dir := t.TempDir()
	write := func(path, text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "alerts.yaml")
	write(path, `alerts:
  - name: West
    matchers:
      - {label: zone, operator: '=', value: west}
`)
	write(filepath.Join(dir, "config.yaml"), `alerts:
  views:
    general:
      columns: [{source: alertname}]
connections:
  local:
    url: http://prometheus
    alertmanager_url: http://alertmanager
    alerts:
      default_view: general
  victoria:
    url: http://vmselect/select/0/prometheus
    alerts:
      default_view: certificates
`)
	external := filepath.Join(dir, "alert_views.d")
	if err := os.Mkdir(external, 0700); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(external, "certificates.yaml"), `views:
  certificates:
    columns:
      - {source: annotations.summary, title: Expiry, extract: 'in (.+)'}
`)
	cfg, err := config.LoadDefault(dir)
	if err != nil {
		t.Fatal(err)
	}
	first, second := cfg.Connections[0], cfg.Connections[1]
	m := New(Options{
		Connection: first, Connections: cfg.Connections, SavedAlertsPath: path, AlertViews: cfg.AlertViews,
	}, nil)
	m.resize(110, 24)
	m.activeTab = alertsTab
	now := time.Now()
	a := testAlert("one", "CertCheck", "warning", "active", now)
	a.Labels["zone"] = "west"
	a.Annotations = map[string]string{"summary": "expires in 5 days"}
	b := testAlert("two", "CertExpiry", "warning", "active", now)
	b.Labels["zone"] = "east"
	m.alerts.lastOK = now
	m.alerts.rows = []alertRow{newAlertRow("test", a), newAlertRow("test", b)}
	m.alerts.filter, m.alerts.sort = "cert", sortAlertNewest
	m.alerts.rebuild("", 0, now)
	savedText(t, m.screen(), "View: general")

	savedCtrl(m, 'o')
	screen := m.screen()
	for _, s := range []string{"West", "FILTER", "certificates", "VIEW"} {
		savedText(t, screen, s)
	}
	press(m, '/', 'w', 'e', 's', 't', tea.KeyEnter, tea.KeyEnter)
	if m.alertViewName() != "general" || m.alerts.structured.name != "West" || len(m.alerts.visible) != 1 {
		t.Fatal("opening the filter changed view or failed to apply the filter")
	}
	savedCtrl(m, 'o')
	press(m, '/', 'c', 'e', 'r', 't', tea.KeyEnter, tea.KeyEnter)
	if m.alertViewName() != "certificates" || m.alerts.structured.name != "West" ||
		m.alerts.filter != "cert" || m.alerts.sort != sortAlertNewest || len(m.alerts.visible) != 1 {
		t.Fatal("opening a view did not preserve the Alerts context")
	}
	savedText(t, m.screen(), "Expiry")
	savedText(t, m.screen(), "5 days")
	m.resize(36, 13)
	savedText(t, m.screen(), "Alerts | certificates | West")
	savedText(t, m.screen(), "5 days")
	m.resize(110, 24)
	press(m, tea.KeyEnter, tea.KeyEsc, 'd')
	if m.alertViewName() != "certificates" {
		t.Fatal("details or lowercase d changed the view")
	}

	press(m, 'c', tea.KeyDown, tea.KeyEnter)
	if m.opt.Connection.Name != second.Name || m.alertViewName() != "certificates" {
		t.Fatal("second connection did not start in its default view")
	}
	press(m, 'c', tea.KeyUp, tea.KeyEnter)
	defer m.alerts.stop()
	if m.opt.Connection.Name != first.Name || m.alertViewName() != "certificates" {
		t.Fatal("returning to first connection did not restore its selection")
	}
	press(m, 'D')
	if m.alertViewName() != "general" || m.alerts.structured.name != "West" {
		t.Fatal("resetting the view did not preserve the filter")
	}
	delete(m.opt.AlertViews, "general")
	m.restoreAlertView()
	if m.alertViewName() != "" || m.alerts.view.Columns != nil {
		t.Fatal("removing a view did not fall back to the built-in view")
	}
}

func TestAlertsAutocompleteAppliesFuzzyTextOnlyOnExplicitSelection(t *testing.T) {
	now := time.Now()
	m := New(Options{Connection: config.Connection{AlertmanagerURL: "http://alertmanager"}}, nil)
	m.activeTab = alertsTab
	a := testAlert("one", "CertCheck", "warning", "active", now)
	b := testAlert("two", "Healthcheck", "warning", "active", now)
	b.Labels["instance"] = "via-CertCheck"
	c := testAlert("three", "Other", "warning", "active", now)
	c.Labels["instance"] = "cert fleet"
	m.alerts.lastOK = now
	for _, alert := range []alertmanager.Alert{a, b, c} {
		m.alerts.rows = append(m.alerts.rows, newAlertRow("test", alert))
	}
	m.alerts.refreshAlertNames()
	m.alerts.rebuild("", 0, now)
	lineCount := len(strings.Split(ansi.Strip(m.screen()), "\n"))
	press(m, '/', 'c', 'e', 'r', 't')
	if layer := m.alertSuggestionsLayer(); layer == nil {
		t.Fatal("matching alertname did not open suggestions")
	}
	if got := len(strings.Split(ansi.Strip(m.screen()), "\n")); got != lineCount {
		t.Fatalf("autocomplete reflowed the Alerts screen: got %d lines, want %d", got, lineCount)
	}
	press(m, tea.KeyEnter)
	if m.alerts.filter != "cert" || len(m.alerts.visible) != 3 {
		t.Fatal("Enter without selection did not use the exact typed fuzzy query")
	}
	press(m, '/', tea.KeyDown, tea.KeyEnter)
	if m.alerts.filter != "CertCheck" || len(m.alerts.visible) != 2 || m.alerts.structured.match != nil {
		t.Fatal("the suggestion did not fill the fuzzy query, or created an exact matcher")
	}
	press(m, '/', 'x', tea.KeyEsc)
	if m.alerts.filter != "CertCheck" || len(m.alerts.visible) != 2 || m.alertSuggestionsLayer() != nil {
		t.Fatal("Escape did not restore the prior search results")
	}
}
