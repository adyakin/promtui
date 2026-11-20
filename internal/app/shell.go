package app

import (
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
	targetsTab
)

var tabs = [...]string{
	promQLTab:  "PromQL",
	alertsTab:  "Alerts",
	targetsTab: "Targets",
}

type shellState struct {
	activeTab         tabID
	width, height     int
	quitting          bool
	pickingConnection bool
	connectionRow     int
}

func (m *Model) shellKey(key tea.KeyPressMsg) (tea.Cmd, bool) {
	if m.activeTab == promQLTab && m.panel&savedQueryKeys != 0 {
		return nil, false // A saved query dialog owns all keys, including tab and connection shortcuts.
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
				if next == targetsTab {
					return m.targetsOnEntry(time.Now()), true
				}
			}
			return nil, true
		}
	}
	switch key.String() {
	case "ctrl+c":
		if m.activeTab == targetsTab && m.targets.cancel != nil {
			m.targets.stop(errTargetsCancelled)
			m.targets.lastDone = time.Now()
			return m.scheduleTargetsTick(m.targets.lastDone), true
		}
		if m.activeTab == alertsTab && m.alerts.cancel != nil {
			m.alerts.stop()
			m.alerts.err = errAlertsCancelled
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
	if m.activeTab == targetsTab {
		return m.targetsKey(key), true
	}
	return nil, false
}

func (m *Model) textInputFocused() bool {
	if m.activeTab == alertsTab {
		return m.alerts.input.Focused()
	}
	if m.activeTab == targetsTab {
		return m.targets.input.Focused()
	}
	switch m.scope() {
	case editKeys, completionKeys, historyKeys, filterKeys:
		return true
	}
	return false
}

func (m *Model) footer() string {
	if m.pickingConnection {
		return m.formatFooter([]string{"↑↓ Select", "Enter Switch"}, "Esc Back", "   ")
	}
	if m.activeTab == promQLTab {
		return m.promQLFooter()
	}
	if m.activeTab == alertsTab {
		return m.alertsFooter()
	}
	return m.targetsFooter()
}

var activeTabStyle = lipgloss.NewStyle().Bold(true).
	Foreground(lipgloss.Color("0")).Background(lipgloss.Color("6"))

func (m *Model) header(active tabID, statuses ...string) string {
	const brand, divider = " PromTUI", " │"
	bar := func(compact bool) string {
		var s strings.Builder
		for i, tab := range tabs {
			label := fmt.Sprintf(" %d %s ", i, tab)
			if compact {
				label = fmt.Sprintf(" %d ", i)
			}
			if tabID(i) == active {
				label = activeTabStyle.Render(label)
			}
			s.WriteString(label)
		}
		return s.String()
	}
	tabBar := bar(false)
	if ansi.StringWidth(brand+divider+tabBar) > m.width {
		tabBar = bar(true)
	}
	left := brand
	room := m.width - ansi.StringWidth(brand+divider+tabBar)
	connection := ui.Plain(m.opt.Connection.Name)
	if connection != "" && room >= 3 {
		left += " " + ansi.Truncate(connection, room-2, "…")
	}
	left += divider + tabBar
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
		return m.withConnectionPicker(screen, cursor, workspace{resultY: 1, bottomY: m.height - 3})
	}
	screen, cursor := m.targetsScreen(time.Now())
	return m.withConnectionPicker(screen, cursor, workspace{resultY: 1, bottomY: m.height - 3})
}
