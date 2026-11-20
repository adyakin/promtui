package app

import (
	"fmt"
	"strings"

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

var tabs = [...]struct {
	name, heading, description string
}{
	promQLTab:  {name: "PromQL"},
	alertsTab:  {"Alerts", "Active Alerts", "Alert exploration will be implemented in a future milestone."},
	targetsTab: {"Targets", "Scrape Targets", "Target exploration will be implemented in a future milestone."},
}

type shellState struct {
	activeTab     tabID
	width, height int
	quitting      bool
}

func (m *Model) shellKey(key tea.KeyPressMsg) (tea.Cmd, bool) {
	if key.Code >= '0' && key.Code < '0'+rune(len(tabs)) {
		if key.Mod == tea.ModAlt || key.Mod == 0 && !m.textInputFocused() {
			m.activeTab = tabID(key.Code - '0')
			return nil, true
		}
	}
	switch key.String() {
	case "ctrl+c":
		return m.act("cancelOrQuit", "", 0), true
	case "ctrl+q":
		return m.act("quit", "", 0), true
	case "q":
		if m.activeTab != promQLTab {
			return m.act("quit", "", 0), true
		}
	}
	// Placeholders have one panel and no local key bindings.
	return nil, m.activeTab != promQLTab
}

func (m *Model) textInputFocused() bool {
	if m.activeTab != promQLTab {
		return false
	}
	switch m.scope() {
	case editKeys, completionKeys, historyKeys, filterKeys:
		return true
	}
	return false
}

func (m *Model) footer() string {
	if m.activeTab == promQLTab {
		return m.promQLFooter()
	}
	hints := []string{"0–2 Tabs", "q Quit"}
	if m.cancel != nil {
		hints = append(hints, "Ctrl+C Cancel")
	}
	return m.formatFooter(hints, "", "   ")
}

var activeTabStyle = lipgloss.NewStyle().Bold(true).
	Foreground(lipgloss.Color("0")).Background(lipgloss.Color("6"))

func (m *Model) header(active tabID, statuses ...string) string {
	const brand, divider = " PromTUI", " │"
	bar := func(compact bool) string {
		var s strings.Builder
		for i, tab := range tabs {
			label := fmt.Sprintf(" %d %s ", i, tab.name)
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
	// until Bubble Tea has queued the delete, so images never cover a placeholder.
	if m.activeTab == promQLTab || m.chart.displayed.cols != 0 || m.chart.sending {
		screen, cursor := m.promQLScreen()
		if m.activeTab != promQLTab {
			cursor = nil
		}
		return screen, cursor
	}
	tab := tabs[m.activeTab]
	rows := make([]string, m.height-4)
	wrapped := strings.Split(ansi.Wrap(tab.description, m.width-4, ""), "\n")
	top, gap := 1, 1
	if len(wrapped)+4 > len(rows) {
		top = 0
	}
	if len(wrapped)+3 > len(rows) {
		gap = 0
	}
	rows[top] = "  " + title.Render(tab.heading)
	rows[top+1] = "  Connection: " + ui.Plain(m.opt.Connection.Name)
	start := top + 2 + gap
	for i, line := range wrapped[:min(len(wrapped), len(rows)-start)] {
		rows[start+i] = "  " + line
	}
	return strings.Join([]string{m.header(m.activeTab), frame(tab.name, "", rows, m.width, true), m.footer()}, "\n"), nil
}
