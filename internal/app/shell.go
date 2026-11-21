package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/ui"
)

type tabID int

const (
	promQLTab tabID = iota
	alertsTab
	silencesTab
	targetsTab
	routingTab
	rulesTab
)

var tabs = [...]string{
	promQLTab:   "PromQL",
	alertsTab:   "Alerts",
	silencesTab: "Silences",
	targetsTab:  "Targets",
	routingTab:  "Routing",
	rulesTab:    "Rules",
}

type shellState struct {
	activeTab         tabID
	width, height     int
	quitting          bool
	pickingConnection bool
	connectionRow     int
}

func (m *Model) shellKey(key tea.KeyPressMsg) (tea.Cmd, bool) {
	if m.silences.modal() && !m.pickingConnection {
		k := key.String()
		if key.Mod&tea.ModAlt != 0 {
			k = key.Keystroke()
		}
		if k == "ctrl+q" && m.silences.writeCancel == nil {
			return m.act("quit", "", 0), true
		}
		return m.silenceModalKey(k, key, time.Now()), true
	}
	if m.activeTab == promQLTab && m.panel&(savedQueryKeys|queryErrorKeys) != 0 {
		return nil, false // Modal dialogs own tab and connection shortcuts.
	}
	if m.activeTab == alertsTab && m.alerts.saved.mode != alertSavedClosed {
		if key.Keystroke() == "ctrl+q" {
			return m.act("quit", "", 0), true
		}
		return m.alertSavedKey(key), true
	}
	if m.activeTab == routingTab && m.routing.dialog != routingNoDialog && !m.pickingConnection {
		return m.routingModalKey(key.String()), true
	}
	if m.activeTab == rulesTab && m.rules.overlayTitle != "" && !m.pickingConnection {
		return m.rulesModalKey(key.String()), true
	}
	targetModal := m.activeTab == targetsTab && m.targets.dialog != noTargetDialog
	if len(m.opt.Connections) > 0 {
		switch k := key.Keystroke(); {
		case m.pickingConnection && k != "ctrl+q" && k != "ctrl+c":
			return m.connectionKey(k), true
		case !m.pickingConnection && !targetModal && (k == "alt+c" || k == "c" && !m.textInputFocused()):
			m.openConnections()
			return nil, true
		}
	}
	if key.Code >= '0' && key.Code < '0'+rune(len(tabs)) {
		if key.Mod == tea.ModAlt || key.Mod == 0 && !m.textInputFocused() {
			next := tabID(key.Code - '0')
			if next != m.activeTab {
				if m.activeTab == routingTab {
					m.cancelRoutingSearch()
				}
				if m.activeTab == rulesTab {
					m.cancelRulesSearch()
					m.rules.stop()
				}
				if m.activeTab == alertsTab {
					m.alerts.stop()
				}
				if m.activeTab == targetsTab {
					m.targets.stop(nil)
					m.targets.dialog = noTargetDialog
				}
				m.activeTab = next
				if next == alertsTab {
					return m.alertsOnEntry(time.Now()), true
				}
				if next == silencesTab {
					m.alerts.silenceNotice = ""
					return m.onSilencesEntry(time.Now()), true
				}
				if next == targetsTab {
					return m.targetsOnEntry(time.Now()), true
				}
				if next == routingTab && m.routing.data == nil {
					return m.refreshRouting(), true
				}
				if next == rulesTab {
					return m.rulesOnEntry(time.Now()), true
				}
			}
			return nil, true
		}
	}
	switch key.String() {
	case "ctrl+c":
		if m.activeTab == rulesTab && m.rules.cancel != nil {
			return m.cancelRulesRefresh(time.Now()), true
		}
		if m.activeTab == routingTab && m.routing.cancel != nil {
			m.routing.stop()
			m.routing.err, m.routing.errorOffset = context.Canceled, 0
			return nil, true
		}
		if m.activeTab == silencesTab && m.silences.listCancel != nil {
			s := &m.silences
			cancelSilenceRequest(&s.listCancel)
			s.lastDone, s.listErr = time.Now(), context.Canceled
			return m.scheduleSilencesTick(s.lastDone), true
		}
		if m.activeTab == targetsTab && m.targets.cancel != nil {
			m.targets.stop(errTargetsCancelled)
			m.targets.lastDone = time.Now()
			return m.scheduleTargetsTick(m.targets.lastDone), true
		}
		if m.activeTab == alertsTab && m.alerts.cancel != nil {
			m.alerts.stop()
			m.alerts.setError(errAlertsCancelled)
			m.alerts.lastDone = time.Now()
			return m.scheduleAlertsTick(m.alerts.lastDone), true
		}
		return m.act("cancelOrQuit", "", 0), true
	case "ctrl+q":
		return m.act("quit", "", 0), true
	}
	if m.activeTab == alertsTab {
		return m.alertsKey(key), true
	}
	if m.activeTab == silencesTab {
		return m.silencesKey(key), true
	}
	if m.activeTab == targetsTab {
		return m.targetsKey(key), true
	}
	if m.activeTab == routingTab {
		return m.routingKey(key), true
	}
	if m.activeTab == rulesTab {
		return m.rulesKey(key), true
	}
	return nil, false
}

func (m *Model) textInputFocused() bool {
	if m.activeTab == silencesTab {
		return m.silences.search.Focused()
	}
	if m.activeTab == alertsTab {
		return m.alerts.input.Focused()
	}
	if m.activeTab == targetsTab {
		return m.targets.input.Focused()
	}
	if m.activeTab == routingTab {
		return m.routing.search.Focused()
	}
	if m.activeTab == rulesTab {
		return m.rules.search.Focused()
	}
	switch m.scope() {
	case editKeys, completionKeys, historyKeys, filterKeys:
		return true
	}
	return false
}

func (m *Model) footer() string {
	if m.parameterDialog != nil {
		hints := []string{"Ctrl+Enter Apply", "Tab Next", "Ctrl+Space Empty"}
		if m.width < 30 {
			hints[0] = "^Enter OK"
		}
		return m.formatFooter(hints, "Esc Cancel", "  ")
	}
	if m.pickingConnection {
		return m.formatFooter([]string{"↑↓ Select", "Enter Switch"}, "Esc Back", "   ")
	}
	if m.activeTab == silencesTab || m.silences.editor != nil {
		return m.silencesFooter()
	}
	if m.activeTab == promQLTab {
		return m.promQLFooter()
	}
	if m.activeTab == alertsTab {
		return m.alertsFooter()
	}
	if m.activeTab == routingTab {
		return m.routingFooter()
	}
	if m.activeTab == rulesTab {
		return m.rulesFooter()
	}
	return m.targetsFooter()
}

var activeTabStyle = lipgloss.NewStyle().Bold(true).
	Foreground(lipgloss.Color("0")).Background(lipgloss.Color("6"))

func (m *Model) header(active tabID, statuses ...string) string {
	brand := "PromTUI"
	const divider = " │"
	bar := func(compact, tight bool) string {
		var s strings.Builder
		for i, tab := range tabs {
			label := fmt.Sprintf(" [%d] %s ", i, tab)
			if compact {
				if i > 0 && !tight {
					s.WriteByte(' ')
				}
				label = fmt.Sprintf("[%d]", i)
			} else {
				label = strings.TrimRight(label, " ")
			}
			if tabID(i) == active {
				label = activeTabStyle.Render(label)
			}
			s.WriteString(label)
		}
		return s.String()
	}
	tabBar := bar(false, false)
	tabDivider := divider
	if ansi.StringWidth(brand+divider+tabBar) > m.width {
		tabBar = bar(true, false)
		tabDivider = divider + " "
		if ansi.StringWidth(brand+tabDivider+tabBar) > m.width {
			brand, tabDivider = "", ""
		}
		if len(statuses) != 0 && ansi.StringWidth(tabBar+statuses[len(statuses)-1])+1 > m.width {
			tabBar = bar(true, true)
		}
	}
	left := brand
	room := m.width - ansi.StringWidth(brand+tabDivider+tabBar)
	connection := ui.Plain(m.opt.Connection.Name)
	if brand != "" && connection != "" && room >= 3 {
		left += " " + ansi.Truncate(connection, room-2, "…")
	}
	left += tabDivider + tabBar
	gap := m.width - ansi.StringWidth(left)
	for _, status := range statuses {
		if n := ansi.StringWidth(status); n+1 <= gap {
			left += strings.Repeat(" ", gap-n) + status
			break
		}
	}
	return lipgloss.NewStyle().Width(m.width).Render(ansi.Truncate(left, m.width, "…"))
}

func (m *Model) queryStatus() []string {
	mode := "Instant"
	if m.opt.Ranged {
		mode = "Range · " + windowLabel(m.opt.Window)
	}
	status := mode + " · UTC"
	if !m.opt.End.IsZero() {
		return []string{status + " │ " + m.opt.End.UTC().Format("2006-01-02 15:04"), status, mode, "UTC"}
	}
	return []string{status, mode, "UTC"}
}

func (m *Model) renderScreen() (string, *tea.Cursor) {
	if m.width < minWidth || m.height < minHeight {
		lines := make([]string, max(1, m.height))
		lines[0] = ansi.Truncate("Enlarge terminal (24×10)", m.width, "…")
		if len(lines) > 1 {
			lines[len(lines)-1] = m.footer()
		}
		return strings.Join(lines, "\n"), nil
	}
	screen, cursor := m.renderWorkspace()
	if m.parameterDialog != nil {
		layer, cursor := m.parameterLayer()
		return lipgloss.NewCompositor(lipgloss.NewLayer(screen), layer).Render(), cursor
	}
	return screen, cursor
}

func (m *Model) renderWorkspace() (string, *tea.Cursor) {
	// A queued upload cannot be interrupted. Keep the old workspace visible
	// until Bubble Tea has queued the delete, so images never cover another tab.
	if m.activeTab == promQLTab || m.chart.displayed.cols != 0 || m.chart.sending {
		screen, cursor := m.promQLScreen()
		if m.activeTab != promQLTab {
			cursor = nil
		}
		return m.withConnectionPicker(screen, cursor, m.workspace())
	}
	if m.activeTab == alertsTab {
		screen, cursor := m.alertsScreen(time.Now())
		if m.alerts.saved.mode != alertSavedClosed && !m.pickingConnection {
			layer, caret := m.alertSavedLayer()
			return lipgloss.NewCompositor(lipgloss.NewLayer(screen), layer).Render(), caret
		}
		screen, cursor = m.withSilenceDialog(screen, cursor, time.Now())
		return m.withConnectionPicker(screen, cursor, workspace{resultY: 1, bottomY: m.height - 3})
	}
	if m.activeTab == silencesTab {
		screen, cursor := m.silencesScreen(time.Now())
		return m.withConnectionPicker(screen, cursor, workspace{resultY: 1, bottomY: m.height - 3})
	}
	if m.activeTab == routingTab {
		screen, cursor := m.routingScreen()
		return m.withConnectionPicker(screen, cursor, workspace{resultY: 1, bottomY: m.height - 3})
	}
	if m.activeTab == rulesTab {
		screen, cursor := m.rulesScreen()
		return m.withConnectionPicker(screen, cursor, workspace{resultY: 1, bottomY: m.height - 3})
	}
	screen, cursor := m.targetsScreen(time.Now())
	return m.withConnectionPicker(screen, cursor, workspace{resultY: 1, bottomY: m.height - 3})
}
