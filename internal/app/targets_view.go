package app

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/api"
	"promtui/internal/ui"
)

const (
	targetSelectHint  = "↑↓ Select"
	targetDetailsHint = "Enter Details"
	targetFilterHint  = "/ Filter"
	targetSourcesHint = "f Sources"
	targetHealthHint  = "h Health"
	targetSortHint    = "s Sort"
	targetDroppedHint = "d Dropped"
	targetRefreshHint = "r Refresh"
)

func (m *Model) targetsFooter() string {
	a := &m.targets
	format := func(hints []string, back string) string { return m.formatFooter(hints, back, "   ") }
	switch a.dialog {
	case targetSources:
		return format([]string{targetSelectHint, "Enter Apply"}, "Esc Cancel")
	case targetDetails, targetHelp, targetStatus:
		hints := []string{"↑↓ Scroll"}
		if a.dialog == targetDetails && len(a.detail.Labels) != 0 {
			hints = append(hints, "y Copy labels")
		}
		return format(hints, "Esc Close")
	}
	if a.input.Focused() {
		return format([]string{"Enter Apply", "Alt+S Status", "Alt+0–2 Tabs"}, "Esc Cancel")
	}
	var hints []string
	hasError := a.hasError()
	if hasError {
		if len(m.targetErrorRows()) > m.height-5 {
			hints = append(hints, "↑↓ Scroll")
		}
		hints = append(hints, targetSourcesHint)
	} else if len(a.visible) != 0 {
		hints = append(hints, targetDetailsHint, targetSelectHint)
	}
	if len(a.sources) != 0 {
		if !hasError {
			hints = append(hints, targetSourcesHint)
		}
		if a.cancel != nil {
			hints = append(hints, "Ctrl+C Cancel")
		} else {
			hints = append(hints, targetRefreshHint)
		}
		if !hasError {
			hints = append(hints, targetFilterHint, targetHealthHint, targetSortHint)
			dropped := targetDroppedHint
			if targetHealthOrder[a.health] == targetDropped {
				dropped = "d Active"
			}
			hints = append(hints, dropped)
		}
	}
	hints = append(hints, "0–2 Tabs", "Alt+S Status")
	if len(m.opt.Connections) > 0 {
		hints = append(hints, m.connectionHint())
	}
	return format(append(hints, "q Quit"), "? Help")
}

func targetSourceError(s targetSourceState) string {
	if s.err == nil {
		return ""
	}
	text := s.err.Error()
	switch {
	case errors.Is(s.err, context.DeadlineExceeded):
		text = "Connection timed out"
	case errors.Is(s.err, api.ErrTargetsAuth):
		text = "Authentication failed: " + text
	}
	// A query-only URL may return a generic 200 HTML page instead of a 404.
	if s.fallback && !errors.Is(s.err, context.DeadlineExceeded) && !errors.Is(s.err, errTargetsCancelled) &&
		!errors.Is(s.err, api.ErrTargetsAuth) {
		text += ". If this URL doesn’t serve the Targets API, configure explicit YAML targets sources."
	}
	return text
}

func (m *Model) targetPage() int { return max(1, m.height-8) }

func (m *Model) targetErrorRows() []string {
	a := &m.targets
	var details []string
	for i, src := range a.sources {
		if a.matchesSource(i) && src.err != nil {
			details = append(details, src.Name+": "+targetSourceError(src))
		}
	}
	heading := "Targets refresh failed"
	if a.source == 0 && len(details) == len(a.sources) {
		heading = "All sources unavailable"
	}
	return paneErrorRows(m.width-4, heading, details...)
}

func (m *Model) targetsScreen(now time.Time) (string, *tea.Cursor) {
	a := &m.targets
	body := make([]string, m.height-5)
	width := m.width - 2
	var cursor *tea.Cursor
	var counts [targetAllActive]int
	ready, failed, loading := 0, 0, 0
	for _, src := range a.sources {
		if src.loading {
			loading++
		}
		if src.err != nil {
			failed++
		} else if !src.lastOK.IsZero() {
			ready++
			for i, n := range src.counts {
				counts[i] += n
			}
		}
	}
	status := ""
	hasError := a.hasError()
	switch {
	case len(a.sources) == 0:
		for i, line := range wrapTargetLines([]string{"Missing target configuration", "Set the connection URL or YAML targets sources."}, width-4) {
			if i < len(body) {
				body[i] = "  " + line
			}
		}
	case ready == 0:
		body[0] = "  Current totals unavailable"
	default:
		body[0] = fmt.Sprintf("  Targets: %d UP · %d DOWN · %d UNKNOWN · %d DROPPED", counts[targetUp],
			counts[targetDown], counts[targetUnknown], counts[targetDropped])
		if ansi.StringWidth(body[0]) > width {
			body[0] = fmt.Sprintf(" %d↑ %d↓ %d? %d drop", counts[targetUp], counts[targetDown], counts[targetUnknown], counts[targetDropped])
		}
	}
	if len(a.sources) > 0 {
		sourceLabel := "All sources"
		if a.source != 0 {
			src := a.sources[a.source-1]
			sourceLabel = src.Name + " (" + src.status() + ")"
		}
		body[1] = "  " + targetHealthNames[targetHealthOrder[a.health]] + " · " + ui.Plain(sourceLabel) + " · Sort: " + targetSortNames[a.sort]
		if a.filter != "" {
			body[1] = "  " + targetHealthNames[targetHealthOrder[a.health]] + " · /" + ui.Plain(a.filter) + " · " + ui.Plain(sourceLabel) + " · Sort: " + targetSortNames[a.sort]
		}
		if a.input.Focused() && !hasError {
			body[1] = a.input.View()
			if c := a.input.Cursor(); c != nil && !m.quitting {
				cursor = nativeCursor(min(m.width-2, 1+c.X), 3)
			}
		}
		status = fmt.Sprintf("%d/%d OK", ready, len(a.sources))
		switch {
		case failed == len(a.sources) && loading == 0:
			status = "All sources unavailable"
		case ready < len(a.sources):
			status = "Incomplete · " + status
			if failed > 0 {
				status += fmt.Sprintf(" · %d failed", failed)
			}
		default:
			status = "Sources: " + status
		}
		if loading > 0 {
			status += " · Refreshing"
		}
	}
	meta := ""
	switch {
	case hasError:
		clear(body)
		meta = renderPaneError(body, m.targetErrorRows(), a.errorOffset)
	case len(a.visible) > 0:
		meta = fmt.Sprintf("%d/%d", a.row+1, len(a.visible))
		body[2] = title.Render(targetColumns(width, false, [6]string{"Health", "Instance", "Job", "Source", "Last scrape", "Duration"}))
		room := m.targetPage()
		first := min(max(0, a.row-room/2), max(0, len(a.visible)-room))
		for i := first; i < min(first+room, len(a.visible)); i++ {
			row := a.visible[i]
			body[3+i-first] = targetColumns(width, i == a.row, [6]string{targetHealthNames[row.health], alertValue(row.instance), alertValue(row.job), row.source,
				targetScrapeAge(row.lastScrape, now), targetDuration(row.LastScrapeDuration)})
		}
	case len(a.sources) != 0:
		body[2] = "  No targets matching the current filter"
		if a.currentFilter() == "" {
			success, pending, total := false, false, 0
			health := targetHealthOrder[a.health]
			for i, src := range a.sources {
				if a.matchesSource(i) {
					success = success || !src.lastOK.IsZero()
					pending = pending || src.loading
					if health == targetDropped {
						total += src.counts[targetDropped]
					} else {
						total += src.counts[targetDown] + src.counts[targetUp] + src.counts[targetUnknown]
					}
				}
			}
			switch {
			case !success && pending:
				body[2] = "  Loading targets…"
			case !success:
				body[2] = "  No target data · f Sources"
			case total == 0 && health == targetDropped:
				body[2] = "  No dropped targets"
			case total == 0:
				body[2] = "  No active targets"
			}
		}
	}
	if a.dialog == targetDetails && a.copyStatus != "" {
		status = a.copyStatus + " · " + status
	}
	base := strings.Join([]string{m.header(targetsTab), frame("Targets", meta, body, m.width, true),
		lipgloss.NewStyle().Width(m.width).Render(ansi.Truncate(" "+status, m.width, "…")), m.footer()}, "\n")
	if a.dialog != noTargetDialog {
		return lipgloss.NewCompositor(lipgloss.NewLayer(base), m.targetDialogLayer()).Render(), nil
	}
	return base, cursor
}

func targetColumns(width int, selected bool, values [6]string) string {
	// The first two cells of each row hold the selection marker.
	sizes := [6]int{8, 9, 8, 8, 11, 8}
	used, count := 2+sizes[0], 1
	for _, n := range sizes[1:] {
		if used+2+n > width {
			break
		}
		used, count = used+2+n, count+1
	}
	if count > 1 {
		sizes[1] += width - used
	}
	var after strings.Builder
	for i := 1; i < count; i++ {
		after.WriteString("  ")
		after.WriteString(alertColumn(values[i], sizes[i]))
	}
	var tint color.Color
	switch values[0] {
	case targetHealthNames[targetDown]:
		tint = statusRed
	case targetHealthNames[targetUnknown]:
		tint = statusYellow
	}
	return renderStatusRow(width, selected, "", alertColumn(values[0], sizes[0]), after.String(), tint)
}

func targetScrapeAge(at, now time.Time) string {
	if at.IsZero() {
		return "—"
	}
	return max(0, now.Sub(at)).Truncate(time.Second).String() + " ago"
}

func targetDuration(value *float64) string {
	if value == nil {
		return "—"
	}
	d := time.Duration(*value * float64(time.Second))
	if d > 0 && d < time.Millisecond {
		return "<1ms"
	}
	return d.Round(time.Millisecond).String()
}

func targetDetailLines(row targetRow) []string {
	lines := []string{"Source: " + row.source}
	if row.health == targetDropped {
		lines = append(lines, "Dropped target (not scraped)")
	} else {
		lines = append(lines, "Health: "+targetHealthNames[row.health])
	}
	lines = append(lines, "Job: "+alertValue(row.job), "Instance: "+alertValue(row.instance), "Scrape pool: "+alertValue(row.ScrapePool))
	if row.health != targetDropped {
		lines = append(lines, "Scrape URL: "+alertValue(row.ScrapeURL), "Last scrape: "+alertTime(row.lastScrape),
			"Last scrape duration: "+targetDuration(row.LastScrapeDuration))
		for _, field := range []struct{ name, value string }{{"Global URL", row.GlobalURL}, {"Scrape interval", row.ScrapeInterval},
			{"Scrape timeout", row.ScrapeTimeout}} {
			if field.value != "" {
				lines = append(lines, field.name+": "+field.value)
			}
		}
		lines = append(lines, "Last scrape error: "+alertValue(row.LastError))
	}
	for _, group := range []struct {
		name   string
		labels map[string]string
	}{{"Labels", row.Labels}, {"Discovered labels", row.DiscoveredLabels}} {
		lines = append(lines, "", group.name)
		if len(group.labels) == 0 {
			lines = append(lines, "—")
		} else {
			lines = append(lines, targetLabels(group.labels)...)
		}
	}
	return lines
}

func (m *Model) targetDialogSize() (width, room int) {
	return min(76, m.width-max(4, m.width/8)), max(1, m.height-max(7, m.height/3))
}

func wrapTargetLines(lines []string, width int) []string {
	var rows []string
	for _, line := range lines {
		for _, paragraph := range strings.Split(strings.ReplaceAll(line, "\r\n", "\n"), "\n") {
			rows = append(rows, strings.Split(ansi.Wrap(ui.Plain(paragraph), width, ""), "\n")...)
		}
	}
	return rows
}

func (m *Model) targetTextDialog() (name string, rows []string, width, room int) {
	a := &m.targets
	width, room = m.targetDialogSize()
	name = "Targets help"
	lines := []string{targetSelectHint + " target · PgUp/PgDn Page · Home/End First/last",
		targetDetailsHint + " · " + targetFilterHint + " (Enter apply, Esc cancel)", targetSourcesHint + " · " + targetHealthHint + " · " + targetSortHint,
		targetDroppedHint + "/active · " + targetRefreshHint + " (auto every 30s)", "Alt+S: show request status",
		"0–2 Tabs (Alt+0–2 while typing) · c Connections · q Quit"}
	if a.dialog == targetDetails {
		name, lines = "Target details", targetDetailLines(a.detail)
	} else if a.dialog == targetStatus {
		name, lines = "Targets status", []string{}
		for _, src := range a.sources {
			lines = append(lines, src.Name+" · "+src.status())
			if src.err != nil {
				lines = append(lines, targetSourceError(src))
			}
		}
		if len(lines) == 0 {
			lines = []string{"No targets configured"}
		}
	}
	rows = wrapTargetLines(lines, max(1, width-4))
	return name, rows, width, min(room, len(rows))
}

func (m *Model) targetSourcePage() int {
	_, room := m.targetDialogSize()
	if room >= 3 && slices.ContainsFunc(m.targets.sources, func(src targetSourceState) bool { return src.err != nil }) {
		room -= min(3, room-1)
	}
	return room
}

func (m *Model) targetDialogLayer() *lipgloss.Layer {
	a := &m.targets
	var name, meta string
	var rows []string
	width := 0
	if a.dialog == targetSources {
		name = "Target Sources"
		width, _ = m.targetDialogSize()
		total, page := len(a.sources)+1, m.targetSourcePage()
		start := min(max(0, a.dialogRow-page/2), max(0, total-page))
		for i := start; i < min(start+page, total); i++ {
			marker, text := "  ", "All sources"
			if i == a.source {
				marker = "● "
			}
			if i > 0 {
				src := a.sources[i-1]
				text = fmt.Sprintf("%s · %s · %d active / %d dropped", src.status(), ui.Plain(src.Name),
					src.counts[targetUp]+src.counts[targetDown]+src.counts[targetUnknown], src.counts[targetDropped])
			}
			line := " " + marker + ansi.Truncate(text, width-6, "…")
			if i == a.dialogRow {
				line = dialogSurface.Reverse(true).Width(width - 2).Render(line)
			}
			rows = append(rows, dialogSurface.Render(line))
		}
		_, room := m.targetDialogSize()
		if a.dialogRow > 0 {
			src := a.sources[a.dialogRow-1]
			if err := targetSourceError(src); err != "" {
				lines := wrapTargetLines([]string{err}, width-4)
				for i := 0; i < min(room-len(rows), len(lines)); i++ {
					rows = append(rows, dialogSurface.Render(" "+lines[i]))
				}
			}
		}
		if page < total {
			meta = fmt.Sprintf("%d/%d", a.dialogRow+1, total)
		}
	} else {
		var content []string
		var page int
		name, content, width, page = m.targetTextDialog()
		start := min(a.dialogRow, len(content)-page)
		for _, line := range content[start : start+page] {
			rows = append(rows, dialogSurface.Render(" "+line))
		}
		if page < len(content) {
			meta = fmt.Sprintf("%d–%d/%d", start+1, start+page, len(content))
		}
	}
	return lipgloss.NewLayer(renderFrame(name, meta, rows, width, true, dialogSurface)).
		X((m.width - width) / 2).Y((m.height - len(rows) - 3) / 2).Z(1)
}
