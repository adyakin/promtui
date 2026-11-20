package app

import (
	"context"
	"errors"
	"fmt"
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

var selectedAlertStyle = lipgloss.NewStyle().Reverse(true)

func (m *Model) alertsFooter() string {
	a := &m.alerts
	var hints []string
	back := ""
	switch {
	case a.help:
		hints, back = []string{"↑↓ Scroll"}, "Esc Close"
	case a.dialog == alertDetails:
		hints, back = []string{"↑↓ Scroll", "y Copy labels", "? Help"}, "Esc Close"
	case a.dialog == alertError:
		hints, back = []string{"↑↓ Scroll", "? Help"}, "Esc Close"
	case a.input.Focused():
		hints, back = []string{"Enter Apply", "Alt+0–2 Tabs"}, "Esc Cancel"
		if len(m.opt.Connections) > 0 {
			hints = append(hints, m.connectionHint())
		}
	default:
		if len(m.opt.Connections) > 0 {
			hints = append(hints, m.connectionHint())
		}
		if a.err != nil {
			hints = append(hints, "F2 Error")
		}
		if len(a.visible) > 0 {
			hints = append(hints, "↑↓ Select", "Enter Details")
		}
		if !a.lastOK.IsZero() {
			hints = append(hints, "/ Filter")
		}
		if len(a.rows) > 0 {
			hints = append(hints, "s Sort")
		}
		if m.opt.Connection.AlertmanagerURL != "" && a.cancel == nil {
			hints = append(hints, "r Refresh")
		}
		if a.cancel != nil {
			hints = append(hints, "Ctrl+C Cancel")
		}
		hints = append(hints, "? Help", "0–2 Tabs", "q Quit")
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
		if !a.lastOK.IsZero() {
			text = "Stale · " + text
		}
		return text
	}
	if a.cancel != nil {
		return "Refreshing alerts…"
	}
	if !a.lastOK.IsZero() {
		return "Updated " + a.lastOK.UTC().Format("15:04:05") + " UTC"
	}
	return "Alertmanager not set"
}

func (m *Model) alertPage() int { return max(1, m.height-8) }

func (m *Model) alertsScreen(now time.Time) (string, *tea.Cursor) {
	a := &m.alerts
	width := m.width - 2
	body := make([]string, m.height-5)
	var cursor *tea.Cursor
	switch {
	case m.opt.Connection.AlertmanagerURL == "":
		copy(body, strings.Split(ansi.Wrap("Set alertmanager_url in this connection’s TOML configuration to load alerts.", width-4, ""), "\n"))
		for i, row := range body {
			if row != "" {
				body[i] = "  " + row
			}
		}
	case a.lastOK.IsZero():
		switch {
		case a.err != nil:
			body[0] = "  F2: error details"
			if a.cancel == nil {
				body[1] = "  r: retry"
			}
		default:
			body[0] = "  Loading from Alertmanager…"
		}
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
			body[1] = "  Sort: state/severity"
			if a.newestFirst {
				body[1] = "  Sort: newest"
			}
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
			body[2] = title.Render(alertColumns(width, "  ", "State", "Severity", "Alert", "Instance", "Duration"))
			room := m.alertPage()
			first := min(max(0, a.row-room/2), max(0, len(a.visible)-room))
			for i := first; i < min(first+room, len(a.visible)); i++ {
				item := a.visible[i]
				prefix := "  "
				if i == a.row {
					prefix = "› "
				}
				row := alertColumns(width, prefix, item.state(), alertValue(item.Labels["severity"]), alertValue(item.Labels["alertname"]),
					alertValue(item.Labels["instance"]), alertDuration(item.StartsAt, now))
				if i == a.row {
					row = selectedAlertStyle.Width(width).Render(row)
				}
				body[3+i-first] = row
			}
		}
	}
	status := a.status()
	if a.copyStatus != "" && !a.help && a.dialog == alertDetails {
		status = a.copyStatus + " · " + status
	}
	meta := ""
	if len(a.visible) > 0 {
		meta = fmt.Sprintf("%d/%d", a.row+1, len(a.visible))
	}
	base := strings.Join([]string{m.header(alertsTab), frame("Alerts", meta, body, m.width, true),
		lipgloss.NewStyle().Width(m.width).Render(ansi.Truncate(" "+status, m.width, "…")), m.footer()}, "\n")
	if a.help || a.dialog != alertNoDialog {
		return lipgloss.NewCompositor(lipgloss.NewLayer(base), m.alertDialogLayer(now)).Render(), nil
	}
	return base, cursor
}

func alertColumns(width int, prefix, state, severity, name, instance, duration string) string {
	const separator = "  "
	if width < 43 {
		n := 11
		if state == "Silenced + Inhibited" {
			state = "Sil.+Inhib."
		}
		return prefix + alertColumn(state, n) + separator + ansi.Truncate(ui.Plain(name), width-2-n-len(separator), "…")
	}
	stateWidth := 11
	if width >= 76 {
		stateWidth = 20
	} else if state == "Silenced + Inhibited" {
		state = "Sil.+Inhib."
	}
	const severityWidth, durationWidth = 8, 8
	remaining := width - 2 - stateWidth - severityWidth - durationWidth - 4*len(separator)
	instanceWidth := max(5, remaining*2/5)
	return prefix + strings.Join([]string{alertColumn(state, stateWidth), alertColumn(severity, severityWidth),
		alertColumn(name, remaining-instanceWidth), alertColumn(instance, instanceWidth), ansi.Truncate(duration, durationWidth, "…")}, separator)
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
			lines = []string{"↑ / ↓ and PgUp / PgDn: scroll details", "y: copy all labels", "Esc or ?: return to details"}
		case alertError:
			lines = []string{"↑ / ↓ and PgUp / PgDn: scroll the error", "Esc or ?: return to the error"}
		default:
			lines = []string{"↑ / ↓: select an alert", "PgUp / PgDn: move one page", "Home / End: first / last alert",
				"Enter: open details", "/: filter label values (Enter applies, Esc cancels)",
				"s: switch between state/severity and newest-first", "r: refresh now (auto-refresh every 30s)",
				"F2: inspect the last refresh error", "0 / 1 / 2 or Alt+0 / 1 / 2: switch tabs", "q: quit"}
		}
	case a.dialog == alertDetails:
		name, lines = "Alert details", alertDetailLines(a.detail, now)
	case a.dialog == alertError:
		name = "Alertmanager error"
		if a.err != nil {
			lines = []string{a.err.Error()}
		} else {
			lines = []string{"The latest refresh succeeded."}
		}
	}
	width = min(76, m.width-max(4, m.width/8))
	for _, line := range lines {
		for _, paragraph := range strings.Split(strings.ReplaceAll(line, "\r\n", "\n"), "\n") {
			rows = append(rows, strings.Split(ansi.Wrap(ui.Plain(paragraph), width-4, ""), "\n")...)
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
	start = min(start, len(rows)-room)
	meta := ""
	if room < len(rows) {
		meta = fmt.Sprintf("%d–%d/%d", start+1, start+room, len(rows))
	}
	lines := make([]string, room)
	for i, row := range rows[start : start+room] {
		lines[i] = dialogSurface.Render(" " + row)
	}
	y := (m.height - room - 3) / 2
	return lipgloss.NewLayer(renderFrame(name, meta, lines, width, true, dialogSurface)).X((m.width - width) / 2).Y(y).Z(1)
}
