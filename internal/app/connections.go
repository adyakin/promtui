package app

import (
	"slices"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"promtui/internal/config"
	"promtui/internal/programstatus"
)

func (m *Model) connectionHint() string {
	if m.textInputFocused() {
		return "Alt+C Connections"
	}
	return "c Connections"
}

func (m *Model) openConnections() {
	m.dismiss()
	m.connectionRow = max(0, slices.IndexFunc(m.opt.Connections, func(c config.Connection) bool {
		return c.Name == m.opt.Connection.Name
	}))
	m.pickingConnection = true
}

func (m *Model) connectionKey(key string) tea.Cmd {
	switch key {
	case "up":
		m.connectionRow = max(0, m.connectionRow-1)
	case "down":
		m.connectionRow = min(len(m.opt.Connections)-1, m.connectionRow+1)
	case "esc":
		m.pickingConnection = false
	case "enter":
		m.pickingConnection = false
		next := m.opt.Connections[m.connectionRow]
		if next.Name == m.opt.Connection.Name {
			return nil
		}
		if m.stop() {
			m.reportProgramStatus(programstatus.Idle, "")
		}
		m.cache = nil
		m.panels = panels{}
		m.clearQueryResults()
		m.queryPhase = queryReady
		m.status, m.summary, m.notice, m.requestLabel = "Ready", "No query executed", "", ""

		m.alerts.stop()
		requestID, tickQueued := m.alerts.requestID, m.alerts.tickQueued
		m.alerts = newAlertsState(next.Alerts)
		m.alerts.requestID, m.alerts.tickQueued = requestID, tickQueued
		m.alerts.input.SetWidth(max(1, m.width-13))

		m.targets.stop(nil)
		targetID, targetTick, dialogID := m.targets.requestID, m.targets.tickQueued, m.targets.dialogID
		m.targets = newTargetsState(next)
		m.targets.requestID, m.targets.tickQueued, m.targets.dialogID = targetID, targetTick, dialogID
		m.targets.input.SetWidth(max(1, m.width-13))
		m.opt.Connection = next
		if m.activeTab == targetsTab {
			return m.refreshTargets()
		}
		return m.refreshAlerts()
	}
	return nil
}

func (m *Model) withConnectionPicker(screen string, cursor *tea.Cursor, w workspace) (string, *tea.Cursor) {
	if !m.pickingConnection {
		return screen, cursor
	}
	return lipgloss.NewCompositor(lipgloss.NewLayer(screen), m.dialogLayer(w)).Render(), nil
}
