package app

import (
	"slices"
	"time"

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
		m.paneTimer.stop()
		if m.stop() {
			m.reportProgramStatus(programstatus.Idle, "")
		}
		m.cache = nil
		m.panels = panels{}
		m.clearQueryResults()
		m.queryPhase = queryReady
		m.status, m.summary, m.notice, m.requestLabel = "Ready", "No query executed", "", ""

		m.alerts.stop()
		m.cancelAlertNavigation()
		m.alertNavigation.cache.at = time.Time{}
		requestID := m.alerts.requestID
		structured := m.alerts.structured
		m.alerts = newAlertsState(next.Alerts)
		m.alerts.requestID = requestID
		m.alerts.structured = structured
		m.alerts.input.SetWidth(max(1, m.width-13))

		m.silences.stop()
		serial := m.silences.serial
		m.silences = newSilencesState()
		m.silences.serial = serial
		m.sizeSilenceInputs()

		m.targets.stop(nil)
		targetID, dialogID := m.targets.requestID, m.targets.dialogID
		m.targets = newTargetsState(next)
		m.targets.requestID, m.targets.dialogID = targetID, dialogID
		m.targets.input.SetWidth(max(1, m.width-13))
		m.routing.stop()
		routingID, showReceivers := m.routing.requestID, m.routing.showReceivers
		m.routing = newRoutingState()
		m.routing.requestID, m.routing.showReceivers = routingID, showReceivers
		m.sizeRoutingInput()
		m.rules.stop()
		rulesID := m.rules.requestID
		m.rules = newRulesState(next)
		m.rules.requestID = rulesID
		m.sizeRulesInput()
		m.opt.Connection = next
		m.restoreAlertView()
		if m.activeTab == rulesTab {
			return m.refreshRules()
		}
		if m.activeTab == routingTab {
			return m.refreshRouting()
		}
		if m.activeTab == targetsTab {
			return m.refreshTargets()
		}
		if m.activeTab == silencesTab {
			return m.refreshSilences()
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
