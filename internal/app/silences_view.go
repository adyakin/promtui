package app

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/alertmanager"
	"promtui/internal/ui"
)

func (m *Model) silencePage() int { return max(1, m.height-8) }

func (m *Model) silencesFooter() string {
	s := &m.silences
	format := func(hints []string, back string) string { return m.formatFooter(hints, back, "  ") }
	switch {
	case s.writeCancel != nil:
		return format(nil, "")
	case s.confirm == confirmSilenceExpire:
		return format([]string{"Enter Expire", "↑↓ Scroll"}, "Esc Back")
	case s.confirm == confirmSilenceBroad:
		return format([]string{"Enter Confirm", "↑↓ Scroll"}, "Esc Back")
	case s.showMatches:
		return format([]string{"↑↓ Scroll"}, "Esc Back")
	case s.editor != nil:
		g := m.silenceEditorGrid()
		hints := []string{"Tab Next"}
		if g.field(s.editor.focus).input == nil {
			hints = append(hints, "Enter Select")
		}
		action := "Create"
		if s.editor.original != nil {
			action = "Save"
		}
		if m.width < 40 {
			return format([]string{"Tab Next", "Ctrl+G " + action, "Ctrl+P Preview"}, "Esc Back")
		}
		return format(append(hints, "Ctrl+P Preview", "Ctrl+Enter "+action, "Ctrl+G "+action), "Esc Back")
	case s.help:
		return format([]string{"↑↓ Scroll"}, "Esc Close")
	case s.detail != nil:
		var hints []string
		if s.detailCancel == nil && s.detailErr == nil {
			if s.detail.State(time.Now()) == "expired" {
				hints = append(hints, "e New from this")
			} else {
				hints = append(hints, "e Edit", "x Expire")
			}
		}
		return format(append(hints, "a Affected alerts", "↑↓ Scroll"), "Esc Close")
	case s.search.Focused():
		return format([]string{"Enter Apply", "Alt+0–3 Tabs"}, "Esc Cancel")
	}
	var hints []string
	if m.opt.Connection.AlertmanagerURL != "" {
		expired := "e Show expired"
		if s.includeExpired {
			expired = "e Hide expired"
		}
		hints = append(hints, "n New", expired)
		if len(s.visible) > 0 {
			hints = append(hints, "Enter Details", "↑↓ Select")
		}
		if !s.lastOK.IsZero() {
			hints = append(hints, "/ Search", "s Sort")
		}
		if s.listCancel == nil {
			hints = append(hints, "r Refresh")
		} else {
			hints = append(hints, "Ctrl+C Cancel")
		}
	}
	if len(m.opt.Connections) > 0 {
		hints = append(hints, m.connectionHint())
	}
	return format(append(hints, "? Help", "0–3 Tabs", "q Quit"), "")
}

func silenceError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "Alertmanager timed out; check connectivity and refresh before retrying"
	}
	return err.Error()
}

func (m *Model) silencesScreen(now time.Time) (string, *tea.Cursor) {
	s := &m.silences
	body := make([]string, m.height-5)
	width := m.width - 2
	var cursor *tea.Cursor
	var meta string
	status := s.notice
	switch {
	case m.opt.Connection.AlertmanagerURL == "":
		copy(body, silenceWrap([]string{"Set alertmanager_url in this connection’s configuration to manage silences."}, width-2))
	case s.lastOK.IsZero() && s.listErr != nil:
		copy(body, silenceWrap([]string{"Refresh failed", silenceError(s.listErr), "", "Press r to retry."}, width-2))
	case s.lastOK.IsZero():
		body[0] = " Loading silences…"
	default:
		body[0] = fmt.Sprintf(" %d active · %d pending · %d expired", s.counts[0], s.counts[1], s.counts[2])
		if ansi.StringWidth(body[0]) > width {
			body[0] = fmt.Sprintf(" %d act · %d pen · %d exp", s.counts[0], s.counts[1], s.counts[2])
		}
		body[1] = " Sort: " + s.sort
		if s.filter != "" {
			body[1] += " · Search: " + ui.Plain(s.filter)
		}
		if s.search.Focused() {
			body[1] = s.search.View()
			if c := s.search.Cursor(); c != nil && !m.quitting {
				cursor = nativeCursor(min(m.width-2, 1+c.X), 3)
			}
		}
		if len(s.visible) == 0 {
			switch {
			case len(s.rows) == 0:
				body[2] = " No silences"
			case s.filter != "" || s.search.Focused():
				body[2] = " No matching silences"
			default:
				body[2] = " No active or pending silences"
			}
			break
		}
		meta = fmt.Sprintf("%d/%d", s.row+1, len(s.visible))
		body[2] = title.Render(silenceColumns(width, false, "Status", "Matchers", "Creator", "Comment", "Ends in"))
		room := m.silencePage()
		first := dialogStart(s.row, len(s.visible), room)
		for i := first; i < min(first+room, len(s.visible)); i++ {
			row := s.visible[i]
			left := "Ended"
			if row.EndsAt.After(now) && row.State(now) != "expired" {
				left = alertDuration(now, row.EndsAt)
			}
			body[3+i-first] = silenceColumns(width, i == s.row, silenceStateLabel(row, now), silenceMatcherText(row.Matchers), row.CreatedBy, row.Comment, left)
		}
	}
	switch {
	case s.listErr != nil && !s.lastOK.IsZero():
		message := "Refresh failed: " + silenceError(s.listErr)
		if status == "" {
			status = message
		} else {
			status += " · " + message
		}
	case s.listCancel != nil && status == "":
		status = "Refreshing silences…"
	case status == "" && !s.lastOK.IsZero():
		status = "Updated " + s.lastOK.UTC().Format("15:04:05") + " UTC"
	}
	if s.includeExpired && !s.lastOK.IsZero() {
		status = "Including expired · " + status
	}
	base := strings.Join([]string{m.header(silencesTab), frame("Silences", meta, body, m.width, true),
		lipgloss.NewStyle().Width(m.width).Render(ansi.Truncate(" "+ui.Plain(status), m.width, "…")), m.footer()}, "\n")
	return m.withSilenceDialog(base, cursor, now)
}

func silenceStateLabel(s alertmanager.Silence, now time.Time) string {
	state := s.State(now)
	return strings.ToUpper(state[:1]) + state[1:]
}

func silenceMatcherText(matchers []alertmanager.Matcher) string {
	var rows []string
	for _, m := range matchers {
		rows = append(rows, m.String())
	}
	return strings.Join(rows, ", ")
}

func silenceColumns(width int, selected bool, status, matchers, creator, comment, ends string) string {
	cols := []string{alertColumn(status, 7)}
	switch {
	case width >= 62:
		remaining := width - 2 - 8 - 7 - 8
		owner, note := min(18, remaining/4), remaining/3
		cols = append(cols, alertColumn(matchers, remaining-owner-note), alertColumn(creator, owner), alertColumn(comment, note), alertColumn(ends, 8))
	case width >= 43:
		cols = append(cols, alertColumn(matchers, width-2-7-9-8-6), alertColumn(creator, 9), alertColumn(ends, 8))
	case width >= 33:
		cols = append(cols, alertColumn(matchers, width-2-7-8-4), alertColumn(ends, 8))
	default:
		cols = append(cols, alertColumn(matchers, width-2-7-2))
	}
	return renderStatusRow(width, selected, strings.Join(cols, "  "), "", "", nil)
}

func silenceWrap(lines []string, width int) (rows []string) {
	for _, line := range lines {
		for _, p := range strings.Split(strings.ReplaceAll(line, "\r\n", "\n"), "\n") {
			for _, row := range strings.Split(ansi.Wrap(ui.Plain(p), width, ""), "\n") {
				rows = append(rows, " "+row)
			}
		}
	}
	return rows
}

func silenceMatchStatus(p silenceMatches) string {
	switch {
	case p.cancel != nil:
		return "Loading matching alerts…"
	case p.err != nil:
		return "Preview failed: " + silenceError(p.err)
	case !p.valid:
		return "Matching alerts not yet previewed"
	case len(p.rows) == 0:
		return "No alerts match (0)"
	default:
		return fmt.Sprintf("Matching alerts: %d", len(p.rows))
	}
}

func (m *Model) silenceDialogText(now time.Time) (name string, lines []string) {
	s := &m.silences
	switch {
	case s.confirm == confirmSilenceBroad:
		return "Confirm broad silence", []string{
			"These matchers can match missing or empty labels, or rely only on exclusions.",
			"Review the scope before continuing. Future alerts can also match.",
			"", "Enter confirms; Esc returns to the draft."}
	case s.confirm == confirmSilenceExpire:
		lines = []string{"Expire this silence now?", "ID: " + s.detail.ID, "Matchers: " + silenceMatcherText(s.detail.Matchers)}
		if s.writeCancel != nil {
			lines = append([]string{"Expiring silence…", ""}, lines...)
		} else if s.confirmErr != nil {
			lines = append([]string{"Expire failed: " + silenceError(s.confirmErr), ""}, lines...)
		}
		return "Expire silence", lines
	case s.showMatches:
		p := s.detailMatches
		if s.editor != nil {
			p = s.editor.preview
		} else if s.detailCancel != nil {
			return "Affected alerts", []string{"Loading silence details…"}
		}
		lines = []string{silenceMatchStatus(p), "Snapshot only; future alerts can also match."}
		if s.editor == nil && s.detailErr != nil {
			lines = append(lines, "Using matchers from the cached silence.")
		}
		for _, a := range p.rows {
			row := newAlertRow("", a)
			lines = append(lines, "", alertValue(a.Labels["alertname"])+" · "+row.state())
			for _, name := range []string{"severity", "instance", "job"} {
				if value, ok := a.Labels[name]; ok {
					lines = append(lines, name+": "+alertValue(value))
				}
			}
			for _, name := range slices.Sorted(maps.Keys(a.Labels)) {
				switch name {
				case "alertname", "severity", "instance", "job":
				default:
					lines = append(lines, name+": "+alertValue(a.Labels[name]))
				}
			}
		}
		return "Affected alerts", lines
	case s.help:
		lines = []string{"↑ / ↓: select; PgUp / PgDn: page; Home / End: first / last",
			"Enter: details · n: create · e: toggle expired", "/: fuzzy search · s: change sort · r: refresh",
			"Details: e edit/copy · x expire · a affected alerts",
			"Editor: Tab / Shift+Tab move · Enter activates buttons", "p on buttons (or Ctrl+P anywhere): preview",
			"Ctrl+Enter or Ctrl+G: create or save · Esc: back",
			"0–3: switch tabs (Alt+0–3 while searching)"}
		if s.listErr != nil {
			lines = append([]string{"Last refresh: " + silenceError(s.listErr), ""}, lines...)
		}
		return "Silences help", lines
	default:
		row := s.detail
		if s.detailCancel != nil {
			return "Silence details", []string{"Loading silence details…", "ID: " + row.ID}
		}
		lines = []string{"ID: " + row.ID, "Status: " + silenceStateLabel(*row, now), "Creator: " + row.CreatedBy,
			"Comment: " + row.Comment, "Start: " + alertTime(row.StartsAt), "End: " + alertTime(row.EndsAt)}
		if row.State(now) != "expired" {
			lines = append(lines, "Remaining: "+alertDuration(now, row.EndsAt))
		}
		lines = append(lines, "", silenceMatchStatus(s.detailMatches), "a: inspect matching alerts", "", "Matchers")
		for _, matcher := range row.Matchers {
			lines = append(lines, matcher.String())
		}
		if s.detailErr != nil {
			lines = append([]string{"Details could not refresh: " + silenceError(s.detailErr), "Showing a cached snapshot. Close and reopen to retry.", ""}, lines...)
		}
		return "Silence details", lines
	}
}

func (m *Model) silenceDialogWidth() int { return max(20, min(116, m.width-4)) }
func (m *Model) silenceDialogRoom() int  { return max(1, m.height-5) }

func (m *Model) scrollSilenceDialog(key string, now time.Time, offset *int) {
	switch key {
	case "up", "down", "pgup", "pgdown", "home", "end":
		_, lines := m.silenceDialogText(now)
		room := m.silenceDialogRoom()
		last := max(0, len(silenceWrap(lines, m.silenceDialogWidth()-4))-room)
		*offset = targetNavigate(key, min(*offset, last), last+1, room)
	}
}

func (m *Model) withSilenceDialog(base string, cursor *tea.Cursor, now time.Time) (string, *tea.Cursor) {
	s := &m.silences
	if !s.modal() {
		return base, cursor
	}
	var layer *lipgloss.Layer
	cursor = nil
	if s.editor != nil && s.confirm == noSilenceConfirm && !s.showMatches {
		layer, cursor = m.silenceEditorLayer(now)
	} else {
		name, lines := m.silenceDialogText(now)
		width := m.silenceDialogWidth()
		rows := silenceWrap(lines, width-4)
		room := min(len(rows), m.silenceDialogRoom())
		offset := s.detailOffset
		switch {
		case s.confirm != noSilenceConfirm:
			offset = s.confirmOffset
		case s.showMatches:
			offset = s.matchesOffset
		case s.help:
			offset = s.helpOffset
		}
		start := min(offset, len(rows)-room)
		layer, _, _ = m.silenceLayer(name, rows[start:start+room], width, silenceRange(start, room, len(rows)))
	}
	return lipgloss.NewCompositor(lipgloss.NewLayer(base), layer).Render(), cursor
}

func silenceRange(first, room, total int) string {
	if room < total {
		return fmt.Sprintf("%d–%d/%d", first+1, first+room, total)
	}
	return ""
}

func (m *Model) silenceLayer(name string, rows []string, width int, meta string) (*lipgloss.Layer, int, int) {
	x, y := (m.width-width)/2, max(1, (m.height-len(rows)-3)/2)
	contents := renderFrame(name, meta, rows, width, true, dialogSurface)
	return lipgloss.NewLayer(contents).X(x).Y(y).Z(2), x, y
}

type silenceAction uint8

const (
	silenceToggleStart silenceAction = iota
	silenceToggleEnd
	silenceCycleOp
	silenceRemoveMatcher
	silenceAddMatcher
	silencePreviewAction
)

type silenceField struct {
	id, width int
	label     string
	input     *textinput.Model
	action    silenceAction
	matcher   int
}

type silenceGridRow struct {
	text   string
	fields []silenceField
}

type silenceGrid struct {
	rows         []silenceGridRow
	count, width int
}

func (g *silenceGrid) add(fields ...silenceField) {
	for i := range fields {
		fields[i].id = g.count
		g.count++
	}
	g.rows = append(g.rows, silenceGridRow{fields: fields})
}

func (g silenceGrid) field(id int) silenceField {
	for _, row := range g.rows {
		for _, f := range row.fields {
			if f.id == id {
				return f
			}
		}
	}
	return silenceField{matcher: -1}
}

func (m *Model) silenceEditorGrid() silenceGrid {
	d := m.silences.editor
	g := silenceGrid{width: m.silenceDialogWidth() - 4}
	w := g.width
	text := func(label string, width int, input *textinput.Model, matcher int) silenceField {
		return silenceField{label: label, width: width, input: input, matcher: matcher}
	}
	button := func(label string, action silenceAction, matcher int) silenceField {
		return silenceField{width: ansi.StringWidth(label) + 2, label: label, action: action, matcher: matcher}
	}
	start := button("Start: Now", silenceToggleStart, -1)
	if d.scheduled {
		start = button("Start: Date", silenceToggleStart, -1)
	}
	end := button("End: Duration", silenceToggleEnd, -1)
	endInput := &d.duration
	if d.until {
		end = button("End: Date", silenceToggleEnd, -1)
		endInput = &d.end
	}
	startGroup := func(width int) []silenceField {
		group := []silenceField{start}
		if d.scheduled {
			group = append(group, text("", width-start.width-1, &d.start, -1))
		} else {
			group[0].width = width
		}
		return group
	}
	endGroup := func(width int) []silenceField {
		return []silenceField{end, text("", width-end.width-1, endInput, -1)}
	}
	switch {
	case w >= 70:
		g.add(append(startGroup(w/2-1), endGroup(w-w/2)...)...)
	case w >= 35:
		g.add(startGroup(w)...)
		g.add(endGroup(w)...)
	default:
		g.add(start)
		if d.scheduled {
			g.add(text("", w, &d.start, -1))
		}
		g.add(end)
		g.add(text("", w, endInput, -1))
	}
	if w >= 48 {
		g.add(text("Creator: ", w/2-1, &d.creator, -1), text("Comment: ", w-w/2, &d.comment, -1))
	} else {
		g.add(text("Creator: ", w, &d.creator, -1))
		g.add(text("Comment: ", w, &d.comment, -1))
	}
	g.rows = append(g.rows, silenceGridRow{text: "Matchers · label / operator / value"})
	for i := range d.matchers {
		row := &d.matchers[i]
		op := button(silenceOperators[row.operator], silenceCycleOp, i)
		remove := button("Remove", silenceRemoveMatcher, i)
		if w >= 44 {
			op.width = 4
			rest := w - op.width - remove.width - 3
			name := min(28, rest*2/5)
			g.add(text("", name, &row.name, i), op, text("", rest-name, &row.value, i), remove)
		} else {
			g.add(text("", w-op.width-1, &row.name, i), op)
			g.add(text("", w-remove.width-1, &row.value, i), remove)
		}
	}
	add, preview := button("Add matcher", silenceAddMatcher, -1), button("Preview", silencePreviewAction, -1)
	if w >= add.width+preview.width+1 {
		g.add(add, preview)
	} else {
		g.add(add)
		g.add(preview)
	}
	return g
}

func (m *Model) sizeSilenceInputs() {
	m.silences.search.SetWidth(max(1, m.width-3-ansi.StringWidth(m.silences.search.Prompt)))
	if m.silences.editor == nil {
		return
	}
	for _, row := range m.silenceEditorGrid().rows {
		for _, f := range row.fields {
			if f.input != nil {
				f.input.SetWidth(max(1, f.width-ansi.StringWidth(f.label)-1))
				f.input.SetCursor(f.input.Position())
			}
		}
	}
}

func (m *Model) focusSilenceField(id int) tea.Cmd {
	d := m.silences.editor
	for _, row := range m.silenceEditorGrid().rows {
		for _, f := range row.fields {
			if f.input != nil {
				f.input.Blur()
			}
		}
	}
	d.focus = id
	m.sizeSilenceInputs()
	if f := m.silenceEditorGrid().field(id); f.input != nil {
		return f.input.Focus()
	}
	return nil
}

func (m *Model) silenceEditorLayer(now time.Time) (*lipgloss.Layer, *tea.Cursor) {
	s, d := &m.silences, m.silences.editor
	g := m.silenceEditorGrid()
	var lines []string
	focusRow, caret := 0, -1
	for _, row := range g.rows {
		line := " "
		if row.text != "" {
			line += ansi.Truncate(row.text, g.width, "…")
		}
		for i, f := range row.fields {
			if i > 0 {
				line += " "
			}
			active := f.id == d.focus
			if active {
				focusRow = len(lines)
			}
			var cell string
			if f.input == nil {
				cell = "[" + f.label + "]"
			} else {
				cell = f.label + ui.Plain(ansi.Strip(f.input.View()))
				if active && f.input.Focused() {
					if c := f.input.Cursor(); c != nil {
						caret = ansi.StringWidth(line) + min(f.width-1, ansi.StringWidth(f.label)+c.X)
					}
				}
			}
			cell = alertColumn(cell, f.width)
			if active {
				cell = dialogSurface.Reverse(true).Bold(true).Render(cell)
			}
			line += cell
		}
		lines = append(lines, line)
	}
	name, action := "New silence", "Create"
	if d.original != nil {
		name, action = "Edit silence", "Save changes"
	} else if d.copied {
		name = "New silence from expired"
	}
	zoneAt := now
	if d.scheduled {
		if t, err := d.startTime(now); err == nil {
			zoneAt = t
		}
	}
	zoneFormat := "MST -07:00"
	if g.width < 32 {
		zoneFormat = "-07:00"
	}
	zone := "Local: " + zoneAt.Local().Format(zoneFormat)
	if d.until {
		if end, err := d.endTime(zoneAt); err == nil && end.Local().Format("-07:00") != zoneAt.Local().Format("-07:00") {
			zone += " → " + end.Local().Format(zoneFormat)
		}
	}
	preview := "Ctrl+P preview"
	if d.preview.valid {
		preview = silenceMatchStatus(d.preview)
	} else if d.preview.err != nil {
		preview = "Preview failed (Ctrl+P)"
	}
	info := []string{zone + " · " + preview, "Ctrl+Enter / Ctrl+G " + action}
	if g.width < 65 {
		preview = "Ctrl+P Preview"
		if d.preview.valid {
			preview = fmt.Sprintf("Matches: %d", len(d.preview.rows))
		} else if d.preview.err != nil {
			preview = "Preview failed"
		}
		shortAction := action
		if d.original != nil {
			shortAction = "Save"
		}
		info = []string{preview, zone, "Ctrl+G " + shortAction}
	}
	if d.scheduled || d.until {
		if g.width >= 65 {
			info[1] += " · " + localSilencePlaceholder
		}
	}
	if s.writeCancel != nil {
		info = []string{"Submitting to Alertmanager…"}
	} else if d.err != nil {
		info = []string{silenceError(d.err), "Ctrl+Enter / Ctrl+G " + action}
	}
	status := silenceWrap(info, g.width)
	room := m.silenceDialogRoom()
	limit := min(len(status), max(1, min(4, room-2)))
	if limit < len(status) && d.err != nil {
		status[limit-1] = ansi.Truncate(status[limit-1], g.width-1, "") + "…"
	}
	status = status[:limit]
	visible := min(len(lines), room-len(status))
	first := dialogStart(focusRow, len(lines), visible)
	rows := append(append([]string{}, lines[first:first+visible]...), status...)
	layer, x, y := m.silenceLayer(name, rows, g.width+4, silenceRange(first, visible, len(lines)))
	if caret >= 0 && !m.quitting && s.writeCancel == nil {
		return layer, nativeCursor(x+1+caret, y+1+focusRow-first)
	}
	return layer, nil
}
