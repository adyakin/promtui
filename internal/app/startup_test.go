package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"promtui/internal/config"
)

func TestStartupPane(t *testing.T) {
	conn := config.Connection{Name: "test", URL: "http://metrics.invalid", AlertmanagerURL: "http://alerts.invalid"}
	for _, tc := range []struct {
		name string
		tab  tabID
	}{
		{"", promQLTab},
		{"promql", promQLTab},
		{"alerts", alertsTab},
		{"silences", silencesTab},
		{"targets", targetsTab},
		{"routing", routingTab},
		{"rules", rulesTab},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(Options{Connection: conn, StartPane: tc.name}, nil)
			t.Cleanup(func() { m.act("quit", "", 0) })
			cmd := m.Init()
			if m.activeTab != tc.tab || (cmd != nil) != (tc.tab != promQLTab) {
				t.Fatalf("active tab = %v, command queued = %t; want tab %v", m.activeTab, cmd != nil, tc.tab)
			}
			for tab, loading := range map[tabID]bool{
				alertsTab:   m.alerts.cancel != nil,
				silencesTab: m.silences.listCancel != nil,
				targetsTab:  m.targets.cancel != nil,
				routingTab:  m.routing.cancel != nil,
				rulesTab:    m.rules.cancel != nil,
			} {
				if loading != (tab == tc.tab) {
					t.Errorf("%s loading = %t on startup in %s", tabs[tab], loading, tabs[tc.tab])
				}
			}
		})
	}
}

func TestStartupPaneKeepsGraphicsProbe(t *testing.T) {
	m := New(Options{
		Connection:    config.Connection{AlertmanagerURL: "http://alerts.invalid"},
		StartPane:     "alerts",
		ProbeGraphics: true,
	}, nil)
	t.Cleanup(func() { m.act("quit", "", 0) })
	batch, ok := m.Init()().(tea.BatchMsg)
	if !ok || len(batch) != 2 || m.alerts.cancel == nil {
		t.Fatal("expected a batch with the alert load and graphics probe")
	}
	if _, ok := batch[1]().(tea.RawMsg); !ok {
		t.Fatal("missing graphics probe")
	}
}
