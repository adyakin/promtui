package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/config"
	"promtui/internal/savedalerts"
)

func TestUnifiedAlertPickerKeepsFiltersAndViewsIndependent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alerts.yaml")
	snapshot, err := savedalerts.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Unrelated", "Certificates"} {
		snapshot, err = snapshot.Add(savedalerts.Filter{Name: name, Matchers: []savedalerts.Matcher{
			{Label: "severity", Operator: "=", Value: "critical"},
		}})
		if err != nil {
			t.Fatal(err)
		}
	}
	conn := config.Connection{Name: "prod", Alerts: config.TableView{
		Columns: []config.Column{{Field: "labels.team", Title: "Owning team"}},
		Search:  []string{"alertname"},
	}}
	m := New(Options{Connection: conn, SavedAlertsPath: path, AlertViews: map[string]config.TableView{
		"Victoria":     {Columns: []config.Column{{Field: "duration"}}},
		"Certificates": {Columns: []config.Column{{Field: "instance"}}, Search: []string{"duration"}},
	}}, nil)
	m.activeTab = alertsTab
	m.resize(100, 24)
	a := &m.alerts
	now := time.Now()
	a.rows = []alertRow{
		newAlertRow("prod", testAlert("1", "CertExpiry", "critical", "active", now)),
		newAlertRow("prod", testAlert("2", "CertOther", "warning", "active", now)),
	}
	a.filter, a.sort = "crt", "labels.team"
	a.rebuild("", 0, now)

	savedCtrl(m, 'o')
	screen := ansi.Strip(m.screen())
	for _, text := range []string{"Open — Alerts", "FILTER  Certificates", "VIEW    Certificates", "VIEW    Victoria"} {
		savedText(t, screen, text)
	}
	if strings.Index(screen, "VIEW    Certificates") > strings.Index(screen, "VIEW    Victoria") {
		t.Fatal("named views are not sorted")
	}
	press(m, '/', 'C', 'R', 'T', tea.KeyEnter)
	if want := []alertPickerItem{alertFilterItem(1), {kind: alertPickerView, viewName: "Certificates"}}; !slices.Equal(a.saved.matches, want) {
		t.Fatalf("fuzzy search returned %#v, want %#v", a.saved.matches, want)
	}
	press(m, tea.KeyEnter)
	if a.structured.name != "Certificates" || m.alertViewName() != "" || !slices.Equal(a.view.Columns, conn.Alerts.Columns) || len(a.visible) != 1 {
		t.Fatal("filter did not apply independently of the view")
	}
	id := a.selection()

	savedCtrl(m, 'o')
	press(m, '/', 'c', 'r', 't', tea.KeyEnter, tea.KeyDown)
	press(m, 'e', 'u', 't', 'd')
	if a.saved.mode != alertSavedBrowse || a.saved.target != -1 {
		t.Fatal("a view accepted filter edit or delete keys")
	}
	press(m, tea.KeyEnter)
	if m.alertViewName() != "Certificates" || a.view.Columns[0].Field != "instance" || a.saved.mode != alertSavedClosed {
		t.Fatal("view did not apply")
	}
	if a.structured.name != "Certificates" || a.filter != "crt" || !slices.Equal(a.view.Search, conn.Alerts.Search) ||
		a.sort != "labels.team" || viewSortTitle(a.sorts, a.sort) != "Owning team" || a.selection() != id || len(a.visible) != 1 {
		t.Fatal("applying a view changed the current filter, selection or sort")
	}
	if !slices.ContainsFunc(a.sorts, func(s viewSort) bool { return s.field == "instance" }) {
		t.Fatal("new column missing from sort modes")
	}

	press(m, tea.KeyEnter)
	if a.dialog != alertDetails {
		t.Fatal("Enter did not open alert details")
	}
	press(m, tea.KeyEsc)
	if m.alertViewName() != "Certificates" || a.dialog != alertNoDialog || a.selection() != id {
		t.Fatal("opening and closing alert details lost the view or selection")
	}

	savedCtrl(m, 'o')
	press(m, '/', 'c', 'r', 't', tea.KeyEnter, tea.KeyEnter)
	if m.alertViewName() != "Certificates" || a.structured.name != "Certificates" || a.saved.mode != alertSavedClosed {
		t.Fatal("applying a filter reset the selected view")
	}

	savedCtrl(m, 'o')
	press(m, '/', 'c', 'r', 't', tea.KeyEnter, 'd', tea.KeyEnter)
	updated, err := savedalerts.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if filters := updated.Filters(); len(filters) != 1 || filters[0].Name != "Unrelated" || m.alertViewName() != "Certificates" {
		t.Fatalf("delete used the picker row rather than the snapshot index: %v", filters)
	}
	press(m, tea.KeyEsc)
	if a.saved.mode != alertSavedClosed {
		t.Fatal("Esc did not close the picker")
	}
}

func TestUnifiedAlertPickerWithUnreadableFilters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alerts.yaml")
	if err := os.WriteFile(path, []byte("invalid: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m := New(Options{SavedAlertsPath: path, AlertViews: map[string]config.TableView{
		"Victoria": {Columns: []config.Column{{Field: "state"}}},
	}}, nil)
	m.activeTab = alertsTab
	m.resize(minWidth, minHeight)
	savedCtrl(m, 'o')
	savedText(t, m.screen(), "VIEW    Victoria")
	press(m, '/', 'v', 'c', 't', tea.KeyEnter, tea.KeyEnter)
	if m.alertViewName() != "Victoria" || m.alerts.saved.mode != alertSavedClosed {
		t.Fatal("invalid filters prevented selecting a configured view")
	}
}

func TestAlertViewRestoreAndReset(t *testing.T) {
	alpha := config.Connection{Name: "alpha", AlertDefaultView: "standard", Alerts: config.TableView{
		Columns: []config.Column{{Field: "state"}},
		Search:  []string{"severity"},
	}}
	beta := config.Connection{Name: "beta", Alerts: config.TableView{Search: []string{"instance"}}}
	m := New(Options{Connection: alpha, AlertViews: map[string]config.TableView{
		"standard": {Columns: []config.Column{{Field: "alertname"}}},
		"custom":   {Columns: []config.Column{{Field: "duration"}}},
	}}, nil)
	check := func(name, column string) {
		t.Helper()
		got := m.alerts.view.Columns
		if m.alertViewName() != name ||
			(column == "" && got != nil || column != "" && (len(got) != 1 || got[0].Field != column)) ||
			!slices.Equal(m.alerts.view.Search, m.opt.Connection.Alerts.Search) {
			t.Fatalf("want %q with column %q; got %q %+v", name, column, m.alertViewName(), m.alerts.view)
		}
	}
	switchTo := func(c config.Connection) {
		m.opt.Connection = c
		m.alerts = newAlertsState(c.Alerts)
		m.restoreAlertView()
	}

	m.restoreAlertView()
	check("standard", "alertname")
	if !m.selectAlertView("custom") {
		t.Fatal("failed to select custom view")
	}
	switchTo(beta)
	check("", "")
	switchTo(alpha)
	check("custom", "duration")
	m.resetAlertView()
	check("standard", "alertname")
	switchTo(beta)
	switchTo(alpha)
	check("standard", "alertname")

	m.selectAlertView("custom")
	delete(m.opt.AlertViews, "custom")
	m.restoreAlertView()
	check("standard", "alertname")
	delete(m.opt.AlertViews, "standard")
	m.restoreAlertView()
	check("", "state")
}
