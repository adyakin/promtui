package app

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"maps"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/alertmanager"
	"promtui/internal/ui"
)

func (m *Model) alertsFooter() string {
	a := &m.alerts
	if a.saved.mode != alertSavedClosed {
		return m.alertSavedFooter()
	}
	var hints []string
	back := ""
	switch {
	case a.help:
		hints, back = []string{"↑↓ Scroll"}, "Esc Close"
	case a.dialog == alertDetails:
		hints, back = []string{"↑↓ Scroll", "p Load in PromQL", "y Copy labels", "? Help"}, "Esc Close"
		if m.width < 29 {
			hints = []string{"p PromQL", "↑↓ Scroll"}
		} else if m.width < 41 {
			hints[0], hints[1] = hints[1], hints[0]
		}
		if m.opt.Connection.AlertmanagerURL != "" {
			hints = append([]string{"s Silence"}, hints...)
		}
		if len(a.detailRelations.links) > 0 {
			hints = append([]string{"i Related"}, hints...)
		}
		if len(a.detailTrail) > 0 {
			back = "Esc Back"
		}
	case a.dialog == alertRelated:
		hints, back = []string{"Enter Open", "↑↓ Select", "? Help"}, "Esc Back"
	case a.dialog == alertStatusDialog:
		hints, back = []string{"↑↓ Scroll", "? Help"}, "Esc Close"
	case a.input.Focused():
		hints, back = []string{"Enter Apply", "Alt+S Status", "Alt+0–5 Tabs"}, "Esc Cancel"
		if len(a.suggestions.matches) > 0 {
			hints = append([]string{"↑↓ Suggestion"}, hints...)
		}
		if len(m.opt.Connections) > 0 {
			hints = append(hints, m.connectionHint())
		}
	default:
		hints = append(hints, "m Matchers", "Ctrl+O Open")
		if len(m.opt.AlertViews) > 0 {
			hints = append(hints, "D Default view")
		}
		if a.structured.match != nil {
			hints = append(hints, "x Clear", "Ctrl+S Save")
		}
		if len(m.opt.Connections) > 0 {
			hints = append(hints, m.connectionHint())
		}
		if a.err != nil {
			if len(m.alertErrorRows()) > m.height-5 {
				hints = append(hints, "↑↓ Scroll")
			}
		} else {
			if len(a.visible) > 0 {
				hints = append(hints, "↑↓ Select", "Enter Details")
			}
			if !a.lastOK.IsZero() {
				hints = append(hints, "/ Filter")
			}
			if len(a.rows) > 0 {
				hints = append(hints, "s Sort")
			}
		}
		if m.opt.Connection.AlertmanagerURL != "" && a.cancel == nil {
			hints = append(hints, "r Refresh")
		}
		if a.cancel != nil {
			hints = append(hints, "Ctrl+C Cancel")
		}
		hints = append(hints, "? Help", "Alt+S Status", "0–5 Tabs", "q Quit")
	}
	return m.formatFooter(hints, back, "   ")
}

func (a *alertsState) status() string {
	if a.err != nil {
		text := "Refresh failed"
		switch {
		case errors.Is(a.err, alertmanager.ErrConfig):
			text = "Invalid alertmanager_url"
		case errors.Is(a.err, alertmanager.ErrAuth):
			text = "Authentication failed"
		case errors.Is(a.err, context.DeadlineExceeded):
			text = "Alertmanager timed out"
		case errors.Is(a.err, errAlertsCancelled):
			text = "Refresh cancelled"
		}
		if a.cancel != nil {
			text += " · retrying"
		}
		return text
	}
	if a.cancel != nil {
		return "Refreshing alerts…"
	}
	if a.refreshErr != nil {
		return "Refresh failed · showing cached alerts"
	}
	if !a.lastOK.IsZero() {
		return "Updated " + a.lastOK.UTC().Format("15:04:05") + " UTC"
	}
	return "Alertmanager not set"
}

func (m *Model) alertPage() int { return max(1, m.height-8) }

func (m *Model) alertErrorRows() []string {
	return paneErrorRows(m.width-4, m.alerts.status(), m.alerts.err.Error())
}

func (m *Model) alertsScreen(now time.Time) (string, *tea.Cursor) {
	a := &m.alerts
	width := m.width - 2
	body := make([]string, m.height-5)
	var cursor *tea.Cursor
	meta := ""
	switch {
	case a.err != nil:
		meta = renderPaneError(body, m.alertErrorRows(), a.errorOffset)
	case m.opt.Connection.AlertmanagerURL == "":
		copy(body, strings.Split(ansi.Wrap("Set alertmanager_url in this connection’s configuration to load alerts.", width-4, ""), "\n"))
		for i, row := range body {
			if row != "" {
				body[i] = "  " + row
			}
		}
	case a.lastOK.IsZero():
		body[0] = "  Loading from Alertmanager…"
	default:
		c := a.counts
		body[0] = fmt.Sprintf("  %d total · %d active · %d silenced · %d inhibited", c.total, c.active, c.silenced, c.inhibited)
		if ansi.StringWidth(body[0]) > width {
			body[0] = fmt.Sprintf(" %d total %d act %d sil %d inh", c.total, c.active, c.silenced, c.inhibited)
		}
		if ansi.StringWidth(body[0]) > width {
			body[0] = fmt.Sprintf(" %dT %dA %dS %dI", c.total, c.active, c.silenced, c.inhibited)
		}
		switch {
		case a.input.Focused():
			body[1] = a.input.View()
			if c := a.input.Cursor(); c != nil && !m.quitting {
				cursor = nativeCursor(min(m.width-2, 1+c.X), 3)
			}
		default:
			body[1] = "  Sort: " + viewSortTitle(a.sorts, a.sort)
			if a.filter != "" {
				body[1] = "  Filter: " + ui.Plain(a.filter) + " · " + strings.TrimSpace(body[1])
			}
		}
		switch {
		case len(a.rows) == 0:
			body[2] = "  No active alerts"
		case len(a.visible) == 0:
			body[2] = "  No matching alerts"
		default:
			room := m.alertPage()
			first := min(max(0, a.row-room/2), max(0, len(a.visible)-room))
			last := min(first+room, len(a.visible))
			table := a.tableLayout(width, now)
			body[2] = table.header()
			for i := first; i < last; i++ {
				body[3+i-first] = table.row(i == a.row, a.visible[i], now)
			}
		}
	}
	status := a.status()
	if a.silenceNotice != "" {
		status = a.silenceNotice + " · " + status
	}
	if a.detailStatus != "" && !a.help && a.dialog == alertDetails {
		status = a.detailStatus + " · " + status
	}
	if a.err == nil && len(a.visible) > 0 {
		meta = fmt.Sprintf("%d/%d", a.row+1, len(a.visible))
	}
	heading := "Alerts"
	if view := m.alertViewName(); view != "" {
		heading += " | View: " + ui.Plain(view)
	}
	if filter := a.structured.description(); filter != "" {
		heading += " | " + ui.Plain(filter)
	}
	base := strings.Join([]string{m.header(alertsTab), frame(heading, meta, body, m.width, a.saved.mode == alertSavedClosed),
		lipgloss.NewStyle().Width(m.width).Render(ansi.Truncate(" "+status, m.width, "…")), m.footer()}, "\n")
	if (a.help || a.dialog != alertNoDialog) && m.silences.editor == nil {
		return lipgloss.NewCompositor(lipgloss.NewLayer(base), m.alertDialogLayer(now)).Render(), nil
	}
	return base, cursor
}

func alertSeverityTint(severity string) color.Color {
	switch strings.ToLower(severity) {
	case "critical":
		return statusRed
	case "high":
		return statusYellow
	default:
		return nil
	}
}

func alertColumn(s string, width int) string {
	s = ansi.Truncate(ui.Plain(s), width, "…")
	return s + strings.Repeat(" ", width-ansi.StringWidth(s))
}

func alertValue(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func alertTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.UTC().Format("2006-01-02 15:04:05 UTC")
}

func alertDetailLines(a alertRow, now time.Time) []string {
	lines := []string{
		"Overview",
		"Alert: " + alertValue(a.Labels["alertname"]),
		"Severity: " + alertValue(a.Labels["severity"]),
		"State: " + a.state(),
		"Active since: " + alertTime(a.StartsAt),
		"Duration: " + alertDuration(a.StartsAt, now),
		"Last update: " + alertTime(a.UpdatedAt),
	}
	if value := a.Annotations["value"]; value != "" {
		lines = append(lines, "Evaluation value (annotation): "+value)
	}
	lines = append(lines, "", "Annotations")
	if len(a.Annotations) == 0 {
		lines = append(lines, "—")
	}
	for _, key := range []string{"summary", "description"} {
		if value, ok := a.Annotations[key]; ok {
			lines = append(lines, key+": "+alertValue(value))
		}
	}
	for _, key := range slices.Sorted(maps.Keys(a.Annotations)) {
		if key != "summary" && key != "description" {
			lines = append(lines, key+": "+alertValue(a.Annotations[key]))
		}
	}
	lines = append(lines, "", "Labels")
	if len(a.Labels) == 0 {
		lines = append(lines, "—")
	}
	for _, key := range slices.Sorted(maps.Keys(a.Labels)) {
		lines = append(lines, key+": "+alertValue(a.Labels[key]))
	}
	var receivers []string
	for _, receiver := range a.Receivers {
		receivers = append(receivers, receiver.Name)
	}
	lines = append(lines, "", "Alertmanager",
		"Receivers: "+alertValue(strings.Join(receivers, ", ")),
		"Silence IDs: "+alertValue(strings.Join(a.Status.SilencedBy, ", ")),
		"Inhibited by: "+alertValue(strings.Join(a.Status.InhibitedBy, ", ")))
	if a.GeneratorURL != "" {
		lines = append(lines, "Generator URL: "+a.GeneratorURL)
	}
	return lines
}

func (m *Model) alertDialog(now time.Time) (name string, rows []string, width, room int) {
	a := &m.alerts
	var lines []string
	switch {
	case a.help:
		name = "Alerts help"
		switch a.dialog {
		case alertDetails:
			lines = []string{"↑ / ↓ and PgUp / PgDn: scroll details", "p: load the generator query in PromQL", "y: copy all labels", "Esc or ?: return to details"}
			if len(a.detailRelations.links) > 0 {
				lines = append(lines, "i: choose a related alert", "Esc in details: go back to the previous alert, or close")
			}
			if m.opt.Connection.AlertmanagerURL != "" {
				lines = append(lines, "s: create a silence from this alert’s labels")
			}
		case alertRelated:
			lines = []string{"↑ / ↓ and PgUp / PgDn: select a related alert", "Enter: open selected alert details", "Esc or ?: return to related alerts"}
		case alertStatusDialog:
			lines = []string{"↑ / ↓ and PgUp / PgDn: scroll status", "Esc or ?: return to status"}
		default:
			lines = []string{"↑ / ↓: select an alert", "PgUp / PgDn: move one page", "Home / End: first / last alert",
				"Enter: open details", "/: fuzzy-filter alerts (↑/↓: suggest alert names, Enter: apply, Esc: cancel)",
				"m: edit label matchers · x: clear label matchers",
				"Ctrl+O: open filters or views · Ctrl+S: save active matchers",
				"D: restore the connection's default view",
				"s: cycle sort modes", "r: refresh now (auto-refresh every 30s)",
				"Alt+S: show request status", "0–5 or Alt+0–5: switch tabs", "q: quit"}
			if a.err != nil {
				lines = append([]string{"↑ / ↓ and PgUp / PgDn: scroll the error"}, lines...)
			}
		}
	case a.dialog == alertDetails:
		name, lines = "Alert details", alertDetailLines(a.detail, now)
		lines = append(lines, a.detailRelations.lines...)
	case a.dialog == alertRelated:
		name = "Related alerts"
	case a.dialog == alertStatusDialog:
		name, lines = "Alerts status", []string{a.status()}
		if a.err != nil {
			lines = append(lines, a.err.Error())
		} else if a.refreshErr != nil {
			lines = append(lines, a.refreshErr.Error())
		}
		if !a.lastOK.IsZero() {
			lines = append(lines, "Last successful refresh: "+alertTime(a.lastOK))
		}
	}
	width = min(76, m.width-max(4, m.width/8))
	if !a.help && a.dialog == alertRelated {
		room = min(len(a.detailRelations.links), m.height-max(7, m.height/3))
		return
	}
	for _, line := range lines {
		for _, paragraph := range strings.Split(strings.ReplaceAll(line, "\r\n", "\n"), "\n") {
			wrapped := strings.Split(ansi.Wrap(ui.Plain(paragraph), width-4, ""), "\n")
			if line == "Inhibition" || strings.HasPrefix(line, "Inhibits (") {
				for i, text := range wrapped {
					wrapped[i] = dialogSurface.Bold(true).Foreground(statusYellow).Render(text)
				}
			}
			rows = append(rows, wrapped...)
		}
	}
	room = min(len(rows), m.height-max(7, m.height/3))
	return
}

func (m *Model) scrollAlertDialog(key string) {
	if m.width < minWidth || m.height < minHeight {
		return
	}
	_, rows, _, room := m.alertDialog(time.Now())
	offset := &m.alerts.offset
	if m.alerts.help {
		offset = &m.alerts.helpOffset
	}
	last := max(0, len(rows)-room)
	step := 1
	if key == "pgup" || key == "pgdown" {
		step = room
	}
	if key == "up" || key == "pgup" {
		step = -step
	}
	*offset = max(0, min(min(*offset, last)+step, last))
}

func (m *Model) alertDialogLayer(now time.Time) *lipgloss.Layer {
	name, rows, width, room := m.alertDialog(now)
	start := m.alerts.offset
	if m.alerts.help {
		start = m.alerts.helpOffset
	}
	total := len(rows)
	related := !m.alerts.help && m.alerts.dialog == alertRelated
	if related {
		total = len(m.alerts.detailRelations.links)
		start = max(0, m.alerts.relatedRow-room/2)
	}
	start = min(start, total-room)
	meta := ""
	if room < total {
		meta = fmt.Sprintf("%d–%d/%d", start+1, start+room, total)
	}
	lines := make([]string, room)
	for i := range lines {
		if related {
			n := start + i
			text := ui.Plain(m.alerts.relatedAlertName(m.alerts.detailRelations.links[n]))
			line := renderStatusRow(width-2, n == m.alerts.relatedRow, "", ansi.Truncate(text, width-4, "…"), "", nil)
			lines[i] = dialogSurface.Render(line)
		} else {
			lines[i] = dialogSurface.Render(" " + rows[start+i])
		}
	}
	y := (m.height - room - 3) / 2
	return lipgloss.NewLayer(renderFrame(name, meta, lines, width, true, dialogSurface)).X((m.width - width) / 2).Y(y).Z(1)
}
