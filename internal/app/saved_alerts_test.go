package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"promtui/internal/alertmanager"
	"promtui/internal/savedalerts"
)

func TestAlertsManualMatchersAndSavedCRUD(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alerts.yaml")
	m := New(Options{SavedAlertsPath: path}, nil)
	m.resize(100, 24)
	m.activeTab = alertsTab
	now := time.Now()
	m.alerts.lastOK = now
	m.alerts.rows = []alertRow{
		newAlertRow("test", alertmanager.Alert{Labels: map[string]string{"alertname": "One", "service": "${literal}", "zone": "west"}}),
		newAlertRow("test", alertmanager.Alert{Labels: map[string]string{"alertname": "Two", "service": "other", "zone": "west"}}),
	}
	m.alerts.rebuild("", 0, now)
	apply := func() { savedCtrl(m, tea.KeyEnter) }
	press(m, 'm')
	savedText(t, m.screen(), "Alert label matchers")
	savedPaste(m, "service")
	press(m, tea.KeyTab, tea.KeyTab)
	savedPaste(m, "${literal}")
	savedCtrl(m, 'n')
	savedPaste(m, "zone")
	press(m, tea.KeyTab, tea.KeyRight, tea.KeyRight, tea.KeyTab)
	savedPaste(m, "we")
	apply()
	if len(m.alerts.visible) != 0 || len(m.alerts.structured.matchers) != 2 {
		t.Fatal("manual regex did not use full-string semantics")
	}
	press(m, 'm', tea.KeyDown)
	savedCtrl(m, 'd')
	press(m, tea.KeyEsc)
	if len(m.alerts.structured.matchers) != 2 {
		t.Fatal("cancelling manual editor changed matchers")
	}
	press(m, 'm', tea.KeyDown)
	savedCtrl(m, 'd')
	apply()
	if len(m.alerts.visible) != 1 || m.alerts.visible[0].Labels["alertname"] != "One" {
		t.Fatal("removing the unmatched expression did not recover the alert")
	}
	savedCtrl(m, 's')
	savedPaste(m, "Literal label")
	press(m, tea.KeyEnter)
	savedText(t, m.screen(), "Literal label")
	press(m, 'x')
	if len(m.alerts.visible) != 2 {
		t.Fatal("clearing did not restore the unfiltered list")
	}
	savedCtrl(m, 'o')
	press(m, '/', 'l', 'b', 'l', tea.KeyEnter, tea.KeyEnter)
	if len(m.alerts.visible) != 1 || m.alerts.structured.matchers[0].Value != "${literal}" {
		t.Fatal("fuzzy browser did not resolve the literal match")
	}
	savedCtrl(m, 'o')
	press(m, 'e')
	savedCtrl(m, 'u')
	savedPaste(m, "Renamed")
	press(m, tea.KeyEnter)
	if m.alerts.structured.name != "Renamed" {
		t.Fatal("active name did not follow rename")
	}
	press(m, 't')
	savedText(t, m.screen(), "Edit Alert Filter Template")
	savedCtrl(m, 'a')
	// Replace the editor buffer with externally usable YAML while in the dialog.
	m.alerts.saved.template.text.SetValue("matchers:\n  - label: service\n    operator: '='\n    value: other\n")
	apply()
	if m.alerts.saved.err != nil {
		t.Fatal(m.alerts.saved.err)
	}
	if len(m.alerts.visible) != 1 || m.alerts.visible[0].Labels["alertname"] != "One" {
		t.Fatal("saving the template mutated the running filter")
	}
	press(m, tea.KeyEnter)
	if len(m.alerts.visible) != 1 || m.alerts.visible[0].Labels["alertname"] != "Two" {
		t.Fatal("reload did not use the edited template")
	}
	savedCtrl(m, 'o')
	press(m, 'u', tea.KeyEnter)
	if m.alerts.saved.err != nil {
		t.Fatal(m.alerts.saved.err)
	}
	press(m, 'd', tea.KeyEsc)
	if len(m.alerts.saved.items) != 1 {
		t.Fatal("cancel deleted the saved filter")
	}
	press(m, 'd', tea.KeyEnter)
	savedText(t, m.screen(), "No saved alert filters yet")
	snapshot, err := savedalerts.Open(path)
	if err != nil || len(snapshot.Filters()) != 0 || len(m.alerts.visible) != 1 || m.alerts.structured.name != "" {
		t.Fatalf("delete did not preserve the active runtime filter: %v", err)
	}
}

func TestSavedAlertTemplateUpdatePreservesParameters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alerts.yaml")
	raw := `alerts:
  - name: Service
    parameters:
      - name: service
        default: srv.*
    matchers:
      - label: service
        operator: "=~"
        value: "${service}"
`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	m := New(Options{SavedAlertsPath: path}, nil)
	m.activeTab = alertsTab
	savedCtrl(m, 'o')
	press(m, tea.KeyEnter)
	savedCtrl(m, 'u')
	savedPaste(m, "[")
	savedCtrl(m, tea.KeyEnter)
	if m.parameterDialog == nil || m.parameterDialog.err == nil || m.alerts.structured.match != nil {
		t.Fatal("invalid resolved regex mutated the Alerts filter")
	}
	press(m, tea.KeyEsc, tea.KeyEnter)
	savedCtrl(m, tea.KeyEnter)
	savedText(t, m.screen(), "Service (service=srv.*)")
	savedCtrl(m, 'o')
	press(m, 'u')
	if m.alerts.saved.mode != alertSavedTemplate || !strings.Contains(m.alerts.saved.template.text.Value(), "${service}") {
		t.Fatal("update reinserted the resolved value instead of the template")
	}
	savedCtrl(m, tea.KeyEnter)
	got, err := os.ReadFile(path)
	if err != nil || string(got) != raw {
		t.Fatalf("no-op template update rewrote its template: %v\n%s", err, got)
	}
}
