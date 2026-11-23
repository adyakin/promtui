package app

import (
	"fmt"
	"image"
	"image/color"
	"maps"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/api"
	"promtui/internal/ui"
)

const rulesHelp = `↑↓ / j k Select or scroll · PgUp/PgDn Page
←→ / h l Collapse, expand or move between levels
Enter Toggle branch or focus details · Tab Switch panels
/ Fuzzy search · Enter Apply · Esc Cancel or clear
f Rule type · s Alerting state
p Load the defining expression into PromQL without running
e Full error · r Refresh · Ctrl+C Cancel request
0–5 Tabs (Alt+0–5 while typing) · c Connections · q Quit`

func (m *Model) sizeRulesInput() {
	layout := m.explorerLayout()
	m.rules.search.SetWidth(max(1, layout.listWidth-5))
	m.rules.ensureVisible(layout.listRows - 1)
}

func (m *Model) rulesFooter() string {
	s := &m.rules
	if s.overlayTitle != "" {
		return m.formatFooter([]string{"↑↓ Scroll"}, "Esc Close", "  ")
	}
	if s.search.Focused() {
		return m.formatFooter([]string{"Enter Apply", "Alt+0–5 Tabs"}, "Esc Cancel", "  ")
	}
	var hints []string
	n := s.selection()
	switch {
	case s.details:
		hints = append(hints, "↑↓ Scroll", "Tab List")
	case n != nil:
		hints = append(hints, "↑↓ Select", "Tab Details", "←→ Tree")
	}
	if n != nil && n.rule != nil && strings.TrimSpace(n.rule.Expression) != "" {
		hints = append(hints, "p PromQL")
	}
	if n.errorText() != "" {
		hints = append(hints, "e Error")
	}
	hints = append(hints, "/ Search", "f Type", "s State", "r Refresh", "0–5 Tabs")
	if s.cancel != nil {
		hints = append(hints, "Ctrl+C Cancel")
	}
	if len(m.opt.Connections) > 0 {
		hints = append(hints, m.connectionHint())
	}
	back := "? Help"
	if len(s.fuzzy()) != 0 {
		back = "Esc Clear"
	}
	return m.formatFooter(append(hints, "q Quit"), back, "  ")
}

func (m *Model) rulesScreen() (string, *tea.Cursor) {
	s := &m.rules
	layout := m.explorerLayout()
	list, details := make([]string, layout.listRows), make([]string, layout.detailRows)
	focused := s.overlayTitle == "" && !m.pickingConnection
	var cursor *tea.Cursor
	typeName, state := ruleTypeFilters[s.ruleType], ruleStateFilters[s.state]
	list[0] = "  " + alertValueOr(typeName, "All types") + " · " + alertValueOr(state, "All states")
	if s.filter != "" {
		list[0] += " · / " + ui.Plain(s.filter)
	}
	if s.search.Focused() {
		list[0] = s.search.View()
		if c := s.search.Cursor(); c != nil && focused && !m.quitting {
			cursor = nativeCursor(min(layout.listWidth-2, 1+c.X), 2)
		}
	}
	listMeta := ""
	if row := s.selectedRow(); row >= 0 {
		room := len(list) - 1
		start := min(row, max(min(s.listTop, max(0, len(s.visible)-room)), row-room+1))
		selectRule := func(i int) tea.Cmd {
			s.selectNode(s.visible[i])
			s.ensureVisible(room)
			return nil
		}
		m.mouseRegion(image.Rect(1, 2, layout.listWidth-1, 2+len(list)), func(key string) tea.Cmd {
			return selectRule(targetNavigate(key, s.selectedRow(), len(s.visible), room))
		})
		m.mouseRows(1, 3, layout.listWidth-2, start, min(start+room, len(s.visible)), "rules",
			func(i int) string { return s.visible[i].id }, func(i int) tea.Cmd {
				s.details = false
				return selectRule(i)
			}, withoutOpen(s.search.Focused(), m.mousePaneEnter))
		listMeta = fmt.Sprintf("%d/%d", row+1, len(s.visible))
		for i := start; i < min(start+room, len(s.visible)); i++ {
			list[i-start+1] = s.listLine(i, row, layout.listWidth-2)
		}
	} else {
		list[1] = "  No matching rules"
	}
	content := rulesDetails(s.selection(), layout.detailWidth)
	detailMeta := renderPaneError(details, content, s.detailTop)
	box := layout.detailBox()
	m.mouseRegion(box, func(key string) tea.Cmd {
		s.detailTop = scrollPaneError(key, s.detailTop, len(content), layout.detailRows)
		return nil
	})
	m.mouseItem(box, mouseID{}, func() tea.Cmd { s.details = true; return nil }, nil)
	left := frame("Rules", listMeta, list, layout.listWidth, focused && (!s.details || s.search.Focused()))
	right := frame("Details", detailMeta, details, layout.detailWidth, focused && s.details && !s.search.Focused())
	body := left + "\n" + right
	if !layout.stacked {
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
	}
	status := lipgloss.NewStyle().Width(m.width).Render(ansi.Truncate(" "+s.status(), m.width, "…"))
	base := strings.Join([]string{m.header(rulesTab), body, status}, "\n")
	if s.overlayTitle != "" {
		base = lipgloss.NewCompositor(lipgloss.NewLayer(base), m.rulesOverlayLayer()).Render()
	}
	return base + "\n" + m.footer(), cursor
}

func alertValueOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func (s *rulesState) status() string {
	if s.notice != "" {
		return s.notice
	}
	ok, stale, failed, warnings := 0, 0, 0, 0
	for _, src := range s.sources {
		switch {
		case src.err == nil && src.data != nil:
			ok++
		case src.err != nil && src.data != nil:
			stale++
		case src.err != nil:
			failed++
		}
		if src.data != nil && src.data.Warning != "" {
			warnings++
		}
	}
	var parts []string
	if stale > 0 {
		parts = append(parts, fmt.Sprintf("%d stale", stale))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	if warnings > 0 {
		parts = append(parts, fmt.Sprintf("%d incomplete", warnings))
	}
	if s.pending > 0 {
		parts = append(parts, fmt.Sprintf("%d loading", s.pending))
	}
	return strings.Join(append(parts, fmt.Sprintf("%d/%d sources current", ok, len(s.sources)), "read-only"), " · ")
}

func (s *rulesState) listLine(i, selected, width int) string {
	n := s.visible[i]
	depth := 0
	for p := n.parent; p != nil; p = p.parent {
		if p.parent != nil || s.showRoot(p) {
			depth++
		}
	}
	marker := "· "
	if len(n.children) > 0 {
		marker = "▸ "
		if i+1 < len(s.visible) && s.visible[i+1].parent == n {
			marker = "▾ "
		}
	}
	var label string
	var tint color.Color
	switch {
	case n.parent == nil:
		label = n.source.status()
		if n.source.err != nil {
			tint = statusRed
		}
	case n.alert != nil:
		label = n.alert.State
	case n.rule != nil:
		switch n.rule.Type {
		case api.RecordingRule:
			label = "record"
		case api.AlertingRule:
			label = alertValueOr(n.rule.State, "alert")
		default:
			label = alertValueOr(n.rule.Type, "unknown")
		}
	}
	switch label {
	case "firing":
		tint = statusRed
	case "pending":
		tint = statusYellow
	}
	label = ansi.Truncate(strings.ToUpper(ui.Plain(label)), max(1, (width-4)/2), "…")
	nameWidth := width - 2
	if label != "" {
		nameWidth -= 2 + ansi.StringWidth(label)
	}
	prefix := strings.Repeat("  ", min(depth, max(0, (nameWidth-4)/2))) + marker
	name := ansi.Truncate(prefix+ui.Plain(n.title), nameWidth, "…")
	if label == "" {
		return renderStatusRow(width, i == selected, "", name, "", nil)
	}
	name += strings.Repeat(" ", nameWidth+1-ansi.StringWidth(name))
	return renderStatusRow(width, i == selected, name, label, " ", tint)
}

func (s *ruleSourceState) status() string {
	switch {
	case s.err != nil && s.data != nil:
		return "Stale"
	case s.loading:
		return "Loading"
	case s.err != nil:
		return "Error"
	case s.data == nil:
		return "Not loaded"
	case s.data.Warning != "":
		return "Partial"
	default:
		return "OK"
	}
}

func (n *ruleNode) errorText() string {
	if n == nil {
		return ""
	}
	if n.rule != nil && n.rule.LastError != "" {
		return n.rule.LastError
	}
	if n.source.err != nil {
		return n.source.err.Error()
	}
	return ""
}

func rulesDetails(n *ruleNode, width int) []string {
	if n == nil {
		return nil
	}
	width = max(1, width-4)
	var rows []string
	add := func(key, value string) {
		if value != "" {
			rows = append(rows, detailField(key, ": ", value, true))
		}
	}
	seconds := func(key string, value *float64) {
		if value != nil {
			add(key, fmt.Sprintf("%gs", *value))
		}
	}
	integer := func(key string, value *int) {
		if value != nil {
			add(key, fmt.Sprint(*value))
		}
	}
	boolean := func(key string, value *bool) {
		if value != nil {
			add(key, fmt.Sprint(*value))
		}
	}
	at := func(key string, value time.Time) {
		if !value.IsZero() {
			add(key, value.UTC().Format(time.RFC3339Nano))
		}
	}
	evaluation := func(e api.RuleEvaluation) {
		at("Last evaluation", e.LastEvaluation)
		seconds("Evaluation duration", e.EvaluationTime)
	}
	fields := func(title string, values map[string]string) {
		if len(values) != 0 {
			rows = append(rows, "", detailHeading(title+":"))
			for _, key := range slices.Sorted(maps.Keys(values)) {
				rows = append(rows, "  "+detailField(key, " = ", values[key], false))
			}
		}
	}
	rows = append(rows, detailHeading(n.title))
	if err := n.errorText(); err != "" {
		rows = append(rows, detailField("Error", ": ", ansi.Truncate(ui.Plain(err), max(1, width-11), "…"), true)+" [e]")
	}
	if n.parent != nil {
		add("Source", n.source.Name)
	}
	if n.source.err != nil || n.parent == nil {
		add("Source status", n.source.status())
	}
	switch {
	case n.alert != nil:
		a := n.alert
		add("Rule", n.rule.Name)
		add("Group", n.group.Name)
		add("State", a.State)
		add("Value", a.Value)
		add("ID", a.ID)
		at("Active since", a.ActiveAt)
		at("Keep firing since", a.KeepFiringSince)
		boolean("Restored", a.Restored)
		boolean("Stabilizing", a.Stabilizing)
		fields("Labels", a.Labels)
		fields("Annotations", a.Annotations)
	case n.rule != nil:
		r := n.rule
		add("Group", n.group.Name)
		add("Type", alertValueOr(r.Type, "unknown"))
		add("Health", r.Health)
		if r.Expression != "" {
			rows = append(rows, "", detailHeading("Expression:"), detailText(r.Expression), "")
		}
		if r.Type == api.AlertingRule {
			add("State", r.State)
			seconds("For", r.For)
			seconds("Keep firing for", r.KeepFiringFor)
			pending := 0
			for _, a := range r.Alerts {
				if a.State == "pending" {
					pending++
				}
			}
			rows = append(rows, fmt.Sprintf("%s · %s", detailField("Active instances", ": ", fmt.Sprint(len(r.Alerts)), false),
				detailField("Pending", ": ", fmt.Sprint(pending), pending > 0)))
		} else {
			add("State", r.State)
		}
		add("ID", r.ID)
		evaluation(r.RuleEvaluation)
		integer("Last samples", r.LastSamples)
		integer("Last series fetched", r.LastSeriesFetched)
		fields("Labels", r.Labels)
		fields("Annotations", r.Annotations)
	case n.group != nil:
		g := n.group
		add("File", g.File)
		add("ID", g.ID)
		add("Datasource", g.Type)
		seconds("Interval", g.Interval)
		evaluation(g.RuleEvaluation)
		integer("Evaluation limit", g.Limit)
		integer("Concurrency", g.Concurrency)
		seconds("Evaluation offset", g.EvalOffset)
		seconds("Evaluation delay", g.EvalDelay)
		failed := 0
		for _, r := range g.Rules {
			if r.LastError != "" || r.Health == "err" {
				failed++
			}
		}
		rows = append(rows, fmt.Sprintf("%s · %s", detailField("Rules", ": ", fmt.Sprint(len(g.Rules)), false),
			detailField("Evaluation errors", ": ", fmt.Sprint(failed), failed > 0)))
		fields("Group labels", g.Labels)
	case n.source.data != nil:
		add("Groups", fmt.Sprint(len(n.source.data.Groups)))
		if len(n.source.data.Groups) == 0 {
			rows = append(rows, "No rule groups configured")
		}
	}
	if n.source.data != nil {
		add("Warning", n.source.data.Warning)
	}
	return wrappedStyled(strings.Join(rows, "\n"), width, true)
}

func (m *Model) rulesOverlayLayer() *lipgloss.Layer {
	s := &m.rules
	rows, width, room := m.rulesOverlayContent()
	body := make([]string, min(room, len(rows)))
	meta := renderPaneError(body, rows, s.overlayTop)
	for i := range body {
		if s.overlayTitle == "Rule error" {
			body[i] = detailDialogRow(detailError.Render(body[i]))
		} else {
			body[i] = dialogSurface.Render(body[i])
		}
	}
	x, y := (m.width-width)/2, (m.height-len(body)-3)/2
	m.mouseDialog(x, y, width, len(body), m.paneKey)
	return lipgloss.NewLayer(renderFrame(s.overlayTitle, meta, body, width, true, dialogSurface)).X(x).Y(y).Z(1)
}

func (m *Model) rulesOverlayContent() (rows []string, width, room int) {
	s := &m.rules
	width, room = m.targetDialogSize()
	if s.overlayTitle != "Rules help" {
		contentWidth := 24
		for _, line := range strings.Split(displayErrorText(s.overlay), "\n") {
			contentWidth = max(contentWidth, ansi.StringWidth(line)+4)
		}
		width = min(width, contentWidth)
	}
	return paneErrorRows(width-4, "", s.overlay)[1:], width, room
}
