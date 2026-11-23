package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/alertmanager"
	"promtui/internal/ui"
)

const (
	routingSelectHint     = "↑↓ Select"
	routingTreeHint       = "←→ Tree"
	routingReceiverHint   = "Enter Receiver"
	routingReferencesHint = "Enter Routes"
	routingSearchHint     = "/ Filter"
	routingRefreshHint    = "r Refresh"
	routingCopyYAMLHint   = "y Copy YAML"
)

type explorerLayout struct {
	listWidth, detailWidth int
	listRows, detailRows   int
	stacked                bool
}

func (m *Model) explorerLayout() explorerLayout {
	if m.width < 68 {
		room := max(3, m.height-7)
		list := max(2, room/2)
		return explorerLayout{m.width, m.width, list, room - list, true}
	}
	list := max(28, m.width*44/100)
	return explorerLayout{list, m.width - list - 1, max(2, m.height-5), max(2, m.height-5), false}
}

func (m *Model) sizeRoutingInput() {
	m.routing.search.SetWidth(max(1, m.explorerLayout().listWidth-5))
}

func (m *Model) routingFooter() string {
	s := &m.routing
	format := func(hints []string, back string) string { return m.formatFooter(hints, back, "  ") }
	switch s.dialog {
	case routingReceiverDialog:
		hints := []string{"↑↓ Scroll"}
		if s.selectedReceiverYAML() != "" {
			hints = append(hints, routingCopyYAMLHint)
		}
		return format(hints, "Esc Close")
	case routingHelpDialog:
		return format([]string{"↑↓ Scroll"}, "Esc Close")
	case routingReferencesDialog:
		return format([]string{routingSelectHint, "Enter Jump"}, "Esc Back")
	}
	if s.search.Focused() {
		return format([]string{"Enter Apply", "Alt+0–5 Tabs"}, "Esc Cancel")
	}
	hints := []string{}
	v := s.current()
	if v.selected != "" {
		hint := routingSelectHint
		if v.details {
			hint = "↑↓ Scroll"
		}
		hints = append(hints, hint)
		if s.showReceivers {
			if len(s.data.References[v.selected]) != 0 {
				hints = append(hints, routingReferencesHint)
			}
			if s.selectedReceiverYAML() != "" {
				hints = append(hints, routingCopyYAMLHint)
			}
		} else {
			if s.data.Routes[v.selected].Receiver.Value != "" {
				hints = append(hints, routingReceiverHint)
			}
		}
	}
	other := "v Receivers"
	if s.showReceivers {
		other = "v Routes"
	}
	hints = append(hints, other)
	if s.cancel != nil {
		hints = append(hints, "Ctrl+C Cancel")
	} else if m.opt.Connection.AlertmanagerURL != "" {
		hints = append(hints, routingRefreshHint)
	}
	if s.data != nil {
		hints = append(hints, routingSearchHint)
		if v.selected != "" {
			if v.details {
				hints = append(hints, "Tab List")
			} else {
				hints = append(hints, "Tab Details")
			}
		}
		if !s.showReceivers && !v.details && len(s.filter(false)) == 0 {
			hints = append(hints, routingTreeHint)
		} else if s.showReceivers && len(s.data.Receivers) > 1 {
			hints = append(hints, "s Sort")
		}
	}
	hints = append(hints, "0–5 Tabs")
	if len(m.opt.Connections) != 0 {
		hints = append(hints, m.connectionHint())
	}
	return format(append(hints, "q Quit"), "? Help")
}

func (m *Model) routingErrorRows() []string {
	s := &m.routing
	switch {
	case m.opt.Connection.AlertmanagerURL == "":
		return paneErrorRows(m.width-4, "Routing unavailable", "Set alertmanager_url for this connection.")
	case s.cancel != nil:
		return []string{"Loading Alertmanager configuration…"}
	case s.err != nil:
		return paneErrorRows(m.width-4, "Routing load failed", routingErrorText(s.err))
	}
	return []string{"No configuration loaded · r Refresh"}
}

func routingErrorText(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "Request cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "Alertmanager connection timed out"
	}
	return err.Error()
}

func (m *Model) routingScreen() (string, *tea.Cursor) {
	s := &m.routing
	name, detailName, status := "Routes", "Route details", ""
	if s.showReceivers {
		name, detailName = "Receivers", "Receiver details"
	}
	var body string
	var cursor *tea.Cursor
	if s.data == nil {
		rows := make([]string, m.height-5)
		meta := renderPaneError(rows, m.routingErrorRows(), s.errorOffset)
		body = frame(name, meta, rows, m.width, s.dialog == routingNoDialog && !m.pickingConnection)
	} else {
		v := s.current()
		layout := m.explorerLayout()
		list, details := make([]string, layout.listRows), make([]string, layout.detailRows)
		list[0] = fmt.Sprintf("  %d visible", len(v.visible))
		if s.showReceivers && s.sortReceivers {
			list[0] += " · A–Z"
		}
		if len(s.filter(s.showReceivers)) != 0 {
			list[0] = "  / " + ui.Plain(v.filter)
		}
		focused := s.dialog == routingNoDialog && !m.pickingConnection
		if s.search.Focused() {
			list[0] = s.search.View()
			if c := s.search.Cursor(); c != nil && focused && !m.quitting {
				cursor = nativeCursor(min(layout.listWidth-2, 1+c.X), 2)
			}
		}
		meta := ""
		if v.selected == "" {
			list[1] = "  No matches"
			if s.showReceivers && len(s.data.Receivers) == 0 {
				list[1] = "  No receivers configured"
			}
		} else {
			row := slices.Index(v.visible, v.selected)
			room := len(list) - 1
			start := min(max(0, row-room/2), max(0, len(v.visible)-room))
			meta = fmt.Sprintf("%d/%d", row+1, len(v.visible))
			for i := start; i < min(start+room, len(v.visible)); i++ {
				line := m.routingListLine(v.visible[i], layout.listWidth-4)
				list[i-start+1] = renderStatusRow(layout.listWidth-2, i == row, "", line, "", nil)
			}
		}
		detailMeta := renderPaneError(details, m.routingDetails(layout.detailWidth), v.detailOffset)
		left := frame(name, meta, list, layout.listWidth, focused && (!v.details || s.search.Focused()))
		right := frame(detailName, detailMeta, details, layout.detailWidth, focused && v.details && !s.search.Focused())
		body = left + "\n" + right
		if !layout.stacked {
			body = lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
		}
		switch {
		case s.err != nil:
			status = "Stale data · " + routingErrorText(s.err)
		case s.cancel != nil:
			status = "Refreshing Alertmanager configuration…"
		default:
			status = fmt.Sprintf("%d routes · %d receivers · read-only", len(s.data.Order), len(s.data.Receivers))
		}
		if s.copyStatus != "" {
			status = s.copyStatus + " · " + status
		}
	}
	status = lipgloss.NewStyle().Width(m.width).Render(ansi.Truncate(" "+status, m.width, "…"))
	base := strings.Join([]string{m.header(routingTab), body, status, m.footer()}, "\n")
	if s.dialog != routingNoDialog {
		return lipgloss.NewCompositor(lipgloss.NewLayer(base), m.routingDialogLayer()).Render(), nil
	}
	return base, cursor
}

func (m *Model) routingListLine(id string, width int) string {
	s := &m.routing
	if s.showReceivers {
		r := s.receiver(id)
		count := fmt.Sprintf("%d routes", len(s.data.References[id]))
		if len(s.data.References[id]) == 1 {
			count = "1 route"
		}
		integrations := integrationSummary(r)
		return routingListSummary(r.Name, integrations+" · "+count, count, width)
	}
	node := s.data.Routes[id]
	marker := "· "
	if len(node.Children) > 0 {
		marker = "▸ "
		if s.searchTree == nil && s.expanded[id] || s.searchTree != nil && slices.ContainsFunc(node.Children, func(child string) bool {
			return s.searchTree[child]
		}) {
			marker = "▾ "
		}
	}
	indent := strings.Repeat("  ", min(node.Depth, max(0, width/4-1)))
	receiver := "→ " + alertValue(node.Receiver.Value)
	return routingListSummary(indent+marker+routeLabel(node), receiver, "", width)
}

func routingListSummary(name, summary, compact string, width int) string {
	name, summary, compact = ui.Plain(name), ui.Plain(summary), ui.Plain(compact)
	if ansi.StringWidth(name)+3+ansi.StringWidth(summary) <= width {
		return name + " · " + summary
	}
	if compact != "" {
		summary = compact
	}
	room := width - 3 - ansi.StringWidth(summary)
	if room >= width/2 {
		return ansi.Truncate(name, room, "…") + " · " + summary
	}
	return ansi.Truncate(name+" · "+summary, width, "…")
}

func integrationSummary(r *alertmanager.Receiver) string {
	if r == nil || len(r.Integrations) == 0 {
		return "No integrations"
	}
	var kinds []string
	counts := map[string]int{}
	for _, kind := range r.Integrations {
		if counts[kind] == 0 {
			kinds = append(kinds, kind)
		}
		counts[kind]++
	}
	for i, kind := range kinds {
		kinds[i] = fmt.Sprintf("%s × %d", kind, counts[kind])
	}
	return strings.Join(kinds, ", ")
}

func (m *Model) routingDetails(width int) []string {
	s := &m.routing
	if s.data == nil || s.current().selected == "" {
		return nil
	}
	if s.showReceivers {
		return s.receiverDetails(s.receivers.selected, width)
	}
	return routingWrapped(s.routeDetails(s.data.Routes[s.routes.selected]), width)
}

func routingWrapped(lines []string, width int) []string {
	width = max(1, width-4)
	rows := make([]string, 0, len(lines))
	for _, line := range lines {
		line = ansi.Wrap(ui.Plain(line), width, "")
		rows = append(rows, strings.Split(ansi.Hardwrap(line, width, true), "\n")...)
	}
	return rows
}

func settingSource(node, source string) string {
	switch source {
	case "":
		return " (default)"
	case node:
		return " (explicit)"
	case alertmanager.RootRoute:
		return " (inherited from root)"
	default:
		return " (inherited from route " + source + ")"
	}
}

func (s *routingState) routeDetails(n *alertmanager.Route) []string {
	match := "Matchers: "
	switch {
	case n.Parent == "":
		match += "none (default catch-all)"
	case len(n.Matchers) == 0:
		match += "none (matches alerts reaching this route)"
	default:
		match += "all of"
	}
	rows := []string{"Route " + n.ID + ": " + s.routePath(n.ID), match}
	for _, matcher := range n.Matchers {
		rows = append(rows, "  "+matcher.String())
	}
	group := "none"
	if len(n.GroupBy.Value) > 0 {
		group = strings.Join(n.GroupBy.Value, ", ")
		if group == "..." {
			group = "... (all labels)"
		}
	}
	rows = append(rows, "Receiver: "+alertValue(n.Receiver.Value)+settingSource(n.ID, n.Receiver.Source),
		fmt.Sprintf("Continue: %t", n.Continue))
	if n.Parent != "" {
		if n.Continue {
			rows = append(rows, "  After a match, evaluate later siblings")
		} else {
			rows = append(rows, "  After a match, skip later siblings")
		}
	}
	return append(rows, "Group by: "+group+settingSource(n.ID, n.GroupBy.Source),
		"Group wait: "+n.GroupWait.Value.String()+settingSource(n.ID, n.GroupWait.Source),
		"Group interval: "+n.GroupInterval.Value.String()+settingSource(n.ID, n.GroupInterval.Source),
		"Repeat interval: "+n.RepeatInterval.Value.String()+settingSource(n.ID, n.RepeatInterval.Source),
		"Integrations: "+integrationSummary(s.receiver(n.Receiver.Value)))
}

func (s *routingState) receiverDetails(name string, width int) []string {
	rows := routingWrapped([]string{"Name: " + name, ""}, width)
	switch r := s.receiver(name); {
	case r == nil:
		rows = append(rows, routingWrapped([]string{"Receiver not present in runtime configuration"}, width)...)
	case r.YAML == "":
		rows = append(rows, routingWrapped([]string{"Receiver YAML unavailable"}, width)...)
	default:
		for _, line := range strings.Split(strings.TrimSuffix(r.YAML, "\n"), "\n") {
			// Hard wrap long values without collapsing YAML indentation.
			line = ansi.Hardwrap(ui.Plain(line), max(1, width-4), true)
			rows = append(rows, strings.Split(line, "\n")...)
		}
	}
	refs := s.data.References[name]
	lines := []string{"", fmt.Sprintf("Configuration references: %d", len(refs))}
	for _, id := range refs {
		n := s.data.Routes[id]
		lines = append(lines, "  "+id+"  "+s.routePath(id)+settingSource(id, n.Receiver.Source))
	}
	return append(rows, routingWrapped(lines, width)...)
}

func (m *Model) routingDialogContent() (name string, rows []string, width, room int) {
	s := &m.routing
	width, room = m.targetDialogSize()
	switch s.dialog {
	case routingReceiverDialog:
		name = "Receiver details"
		rows = s.receiverDetails(s.data.Routes[s.routes.selected].Receiver.Value, width)
	case routingReferencesDialog:
		name = "Jump to route"
		for _, id := range s.data.References[s.receivers.selected] {
			n := s.data.Routes[id]
			relation := "explicit"
			if n.Receiver.Source != id {
				relation = "inherited"
			}
			row := fmt.Sprintf("%s %s (%s)", id, routeLabel(n), relation)
			rows = append(rows, ui.Plain(row))
		}
	default:
		name = "Routing help"
		rows = routingWrapped([]string{
			routingSelectHint + " · j/k also work · PgUp/PgDn Page",
			routingTreeHint + " · h/l also work · Tab List/Details",
			routingReceiverHint + " (Routes) · " + routingReferencesHint + " (Receivers)",
			"v Switch view · " + routingSearchHint + " (Enter apply; erase to clear)",
			"s Sort receivers by name / configuration order",
			routingCopyYAMLHint + " (Receivers or receiver overlay)",
			routingRefreshHint + " (no polling) · 0–5 Tabs · q Quit",
		}, width)
	}
	return name, rows, width, min(room, len(rows))
}

func (m *Model) routingDialogLayer() *lipgloss.Layer {
	s := &m.routing
	name, rows, width, room := m.routingDialogContent()
	body := make([]string, room)
	start := min(s.dialogRow, len(rows)-room)
	if s.dialog == routingReferencesDialog {
		start = min(max(0, s.dialogRow-room/2), len(rows)-room)
	}
	for i, row := range rows[start : start+room] {
		row = " " + ansi.Truncate(row, width-4, "…")
		style := dialogSurface
		if s.dialog == routingReferencesDialog && start+i == s.dialogRow {
			style = style.Reverse(true).Width(width - 2)
		}
		body[i] = style.Render(row)
	}
	meta := ""
	if room < len(rows) {
		meta = fmt.Sprintf("%d–%d/%d", start+1, start+room, len(rows))
	}
	return lipgloss.NewLayer(renderFrame(name, meta, body, width, true, dialogSurface)).
		X((m.width - width) / 2).Y((m.height - len(body) - 3) / 2).Z(1)
}

func (m *Model) routingModalKey(k string) tea.Cmd {
	s := &m.routing
	switch {
	case k == "ctrl+q":
		return m.act("quit", "", 0)
	case k == "esc" || (s.dialog == routingHelpDialog && k == "?"):
		s.dialog = routingNoDialog
	case k == "y" && s.dialog == routingReceiverDialog:
		return s.copyReceiverYAML()
	case s.dialog == routingReferencesDialog:
		refs := s.data.References[s.receivers.selected]
		if k == "enter" {
			s.jump(refs[s.dialogRow])
		} else {
			_, room := m.targetDialogSize()
			s.dialogRow = targetNavigate(routingNavKey(k), s.dialogRow, len(refs), room)
		}
	default:
		_, rows, _, room := m.routingDialogContent()
		s.dialogRow = scrollPaneError(routingNavKey(k), s.dialogRow, len(rows), room)
	}
	return nil
}
