package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"promtui/internal/alertmanager"
	"promtui/internal/config"
)

func TestPaneRefreshIntervals(t *testing.T) {
	intervals := config.AutoRefresh{
		Alerts: 10 * time.Second, Rules: 20 * time.Second, Targets: 30 * time.Second,
		Silences: 40 * time.Second, Routing: 50 * time.Second,
	}
	now := time.Now().Add(-time.Hour)
	conn := config.Connection{Name: "test", URL: "http://metrics.invalid", AlertmanagerURL: "http://alerts.invalid"}
	for _, tc := range []struct {
		tab      tabID
		interval time.Duration
		complete func(*Model, time.Time) tea.Cmd
		pending  func(*Model) bool
	}{
		{alertsTab, intervals.Alerts, func(m *Model, now time.Time) tea.Cmd {
			return m.onAlertsCompleted(alertsCompleted{id: m.alerts.requestID}, now)
		}, func(m *Model) bool { return m.alerts.cancel != nil }},
		{rulesTab, intervals.Rules, func(m *Model, now time.Time) tea.Cmd {
			return m.onRulesLoaded(rulesLoaded{id: m.rules.requestID}, now)
		}, func(m *Model) bool { return m.rules.cancel != nil }},
		{targetsTab, intervals.Targets, func(m *Model, now time.Time) tea.Cmd {
			return m.onTargetsCompleted(targetsCompleted{id: m.targets.requestID}, now)
		}, func(m *Model) bool { return m.targets.cancel != nil }},
		{silencesTab, intervals.Silences, func(m *Model, now time.Time) tea.Cmd {
			return m.onSilencesLoaded(silencesLoaded{id: m.silences.listID}, now)
		}, func(m *Model) bool { return m.silences.listCancel != nil }},
		{routingTab, intervals.Routing, func(m *Model, now time.Time) tea.Cmd {
			return m.onRoutingLoaded(routingLoaded{id: m.routing.requestID, data: &alertmanager.Routing{
				Routes: map[string]*alertmanager.Route{alertmanager.RootRoute: {}},
			}}, now)
		}, func(m *Model) bool { return m.routing.cancel != nil }},
	} {
		t.Run(tabs[tc.tab], func(t *testing.T) {
			m := New(Options{Connection: conn}, nil)
			t.Cleanup(func() { m.act("quit", "", 0) })
			manual := func() tea.Cmd {
				_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
				return cmd
			}
			if m.switchTab(tc.tab) == nil || !tc.pending(m) {
				t.Fatal("first entry did not start a request with auto-refresh off")
			}
			if tc.complete(m, now) != nil || m.paneTimer.cancel != nil {
				t.Fatal("a snapshot scheduled polling with auto-refresh off")
			}
			m.switchTab(promQLTab)
			if m.switchTab(tc.tab) != nil || tc.pending(m) {
				t.Fatal("returning to a cached pane started a request with auto-refresh off")
			}
			if manual() == nil || !tc.pending(m) {
				t.Fatal("manual refresh did not work with auto-refresh off")
			}
			m.opt.AutoRefresh = intervals
			if tc.complete(m, now) == nil || !m.paneTimer.due.Equal(now.Add(tc.interval)) {
				t.Fatalf("deadline = %v, want %v", m.paneTimer.due, now.Add(tc.interval))
			}
			old := m.paneTimer.id
			if manual() == nil || tc.complete(m, now.Add(time.Second)) == nil {
				t.Fatal("manual refresh did not reset the schedule")
			}
			due := now.Add(time.Second + tc.interval)
			if !m.paneTimer.due.Equal(due) || m.onPaneRefreshTick(old, due) != nil || tc.pending(m) {
				t.Fatal("stale tick restarted a request after manual refresh")
			}
			if m.onPaneRefreshTick(m.paneTimer.id, due) == nil || !tc.pending(m) {
				t.Fatal("configured interval did not start a request")
			}
			if tc.complete(m, due) == nil {
				t.Fatal("automatic refresh did not schedule the next request")
			}
			old = m.paneTimer.id
			m.switchTab(promQLTab)
			if m.paneTimer.cancel != nil || m.onPaneRefreshTick(old, due.Add(tc.interval)) != nil || tc.pending(m) {
				t.Fatal("a hidden pane kept polling")
			}
			if m.switchTab(tc.tab) == nil || !tc.pending(m) {
				t.Fatal("an opted-in pane did not refresh stale data on return")
			}
		})
	}
}

func TestSilencesLocalTransitionsWithoutPolling(t *testing.T) {
	now := time.Now()
	m := New(Options{Connection: config.Connection{AlertmanagerURL: "http://alerts.invalid"}}, nil)
	m.switchTab(silencesTab)
	start, end := now.Add(time.Minute), now.Add(2*time.Minute)
	pending := testSilence("pending", "pending", start, end)
	cmd := m.onSilencesLoaded(silencesLoaded{id: m.silences.listID, rows: []alertmanager.Silence{pending}}, now)
	if cmd == nil || !m.paneTimer.due.Equal(start) || m.silences.counts != [3]int{0, 1, 0} {
		t.Fatal("did not schedule the local start transition")
	}
	cmd = m.onPaneRefreshTick(m.paneTimer.id, start)
	if cmd == nil || !m.paneTimer.due.Equal(end) || m.silences.listCancel != nil || m.silences.counts != [3]int{1, 0, 0} {
		t.Fatal("start transition did not activate the silence locally")
	}
	if m.onPaneRefreshTick(m.paneTimer.id, end) != nil || m.silences.listCancel != nil || m.silences.counts != [3]int{0, 0, 1} {
		t.Fatal("end transition did not expire the silence locally")
	}
}

func TestPaneRefreshTimerConnectionSwitch(t *testing.T) {
	now := time.Now()
	first := config.Connection{Name: "first", AlertmanagerURL: "http://alerts.invalid/first"}
	second := config.Connection{Name: "second", AlertmanagerURL: "http://alerts.invalid/second"}
	m := New(Options{Connection: first, Connections: []config.Connection{first, second},
		AutoRefresh: config.AutoRefresh{Alerts: time.Minute}}, nil)
	t.Cleanup(func() { m.act("quit", "", 0) })
	m.switchTab(alertsTab)
	cmd := m.onAlertsCompleted(alertsCompleted{id: m.alerts.requestID}, now)
	if cmd == nil {
		t.Fatal("first connection did not schedule polling")
	}
	old := m.paneTimer.id
	m.connectionRow = 1
	if m.connectionKey("enter") == nil || m.paneTimer.cancel != nil || m.opt.Connection.Name != second.Name {
		t.Fatal("connection switch did not cancel the old timer and start a new request")
	}
	if msg := cmd(); msg != nil {
		t.Fatalf("canceled timer delivered %v", msg)
	}
	if m.onAlertsCompleted(alertsCompleted{id: m.alerts.requestID}, now) == nil ||
		m.onPaneRefreshTick(old, now.Add(time.Minute)) != nil || m.alerts.cancel != nil {
		t.Fatal("old tick interfered with the new connection")
	}
}

func TestPaneRefreshTimerDispatch(t *testing.T) {
	m := New(Options{Connection: config.Connection{AlertmanagerURL: "http://alerts.invalid"},
		AutoRefresh: config.AutoRefresh{Alerts: time.Minute}}, nil)
	t.Cleanup(func() { m.act("quit", "", 0) })
	m.switchTab(alertsTab)
	now := time.Now()
	m.onAlertsCompleted(alertsCompleted{id: m.alerts.requestID}, now.Add(-time.Minute))
	m.paneTimer.stop()
	tick := m.scheduleAlertsTick(now)
	if tick == nil {
		t.Fatal("no timer scheduled")
	}
	if _, request := m.Update(tick()); request == nil || m.alerts.cancel == nil {
		t.Fatal("timer message did not dispatch an Alerts request")
	}
}
