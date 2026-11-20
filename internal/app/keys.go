package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/editor"
)

type keyScope uint16

const (
	editKeys keyScope = 1 << iota
	tableKeys
	graphKeys
	completionKeys
	historyKeys
	helpKeys
	detailKeys
	labelKeys
	rangeKeys
	seriesKeys
	filterKeys
	resultKeys = tableKeys | graphKeys
	mainKeys   = editKeys | resultKeys
	scrollKeys = helpKeys | detailKeys | labelKeys
	allKeys    = mainKeys | completionKeys | historyKeys | scrollKeys | rangeKeys | seriesKeys | filterKeys
)

type binding struct {
	id, keys, label, hint string
	scope                 keyScope
}

const runHint = "Ctrl+Enter Run"

// Order is footer priority. Actions without a hint are listed only in Help.
var bindings = []binding{
	{"run", "f5 ctrl+enter ctrl+g", "Ctrl+Enter / F5 / Ctrl+G", runHint, editKeys | completionKeys},
	{"results", "tab shift+tab", "Tab / Shift+Tab", "Tab Results", editKeys},
	{"history", "ctrl+r", "Ctrl+R", "Ctrl+R History", editKeys | completionKeys},
	{"complete", "ctrl+space ctrl+@", "Ctrl+Space", "Ctrl+Space Complete", editKeys | completionKeys},
	{"edit", "i tab shift+tab", "i / Tab / Shift+Tab", "i Edit", resultKeys},
	{"graph", "g", "g", "g Graph", resultKeys},
	{"table", "t", "t", "t Table", resultKeys},
	{"series", "s", "s", "s Series", resultKeys},
	{"range", "r", "r", "r Range", resultKeys},
	{"quit", "q", "q", "q Quit", resultKeys},
	{"select", "up down", "↑ / ↓", "↑↓ Select", resultKeys | rangeKeys | seriesKeys},
	{"toggle", "space", "Space", "Space Toggle", graphKeys | seriesKeys},
	{"expand", "enter", "Enter", "Enter Expand", tableKeys},
	{"sort", "n v", "n / v", "n/v Sort", tableKeys},
	{"labels", "l", "l", "l Labels", resultKeys},
	{"copy", "y", "y", "y Copy", resultKeys},
	{"run", "f5 ctrl+enter ctrl+g", "Ctrl+Enter / F5 / Ctrl+G", runHint, resultKeys},
	{"accept", "tab", "Tab", "Tab Accept", completionKeys},
	{"restore", "enter", "Enter", "Enter Restore", historyKeys},
	{"apply", "enter", "Enter", "Enter Run", rangeKeys},
	{"isolate", "enter", "Enter", "Enter Isolate", seriesKeys},
	{"all", "a", "a", "a All", seriesKeys},
	{"filter", "/", "/", "/ Filter", seriesKeys},
	{"finishFilter", "enter", "Enter", "Enter Done", filterKeys},
	{"selectAssist", "up down", "↑ / ↓", "↑↓ Select", completionKeys | historyKeys},
	{"inspectCopy", "y", "y", "y Copy labels", labelKeys},
	{"scroll", "up down pgup pgdown", "↑ / ↓ / PgUp / PgDn", "↑↓ Scroll", scrollKeys},
	{"cancelOrQuit", "ctrl+c", "Ctrl+C", "Ctrl+C Quit", allKeys},
	{"back", "esc", "Esc", "Esc Back", allKeys &^ resultKeys},
	{"help", "f1", "F1", "F1 Help", editKeys | completionKeys},
	{"help", "f1 ?", "F1 / ?", "? Help", resultKeys},
	{"status", "f2", "F2", "F2 Status", mainKeys},
	{"page", "pgup pgdown", "PgUp / PgDn", "", tableKeys},
	{"edge", "home end", "Home / End", "", tableKeys},
	{"pan", "left right", "← / →", "", tableKeys},
	{"sample", "[ ]", "[ / ]", "", tableKeys},
	{"copyLabels", "Y shift+y", "Y", "", resultKeys},
	{"recall", "alt+up alt+down", "Alt+Up / Alt+Down", "", editKeys | completionKeys},
	{"newline", "enter", "Enter", "", editKeys | completionKeys},
	{"legacyView", "f4", "F4", "", mainKeys},
	{"mode", "ctrl+t", "Ctrl+T", "", mainKeys},
	{"range", "ctrl+w", "Ctrl+W", "", resultKeys},
	{"quit", "ctrl+q", "Ctrl+Q", "", allKeys},
}

func (m *Model) mainScope() keyScope {
	switch {
	case m.editor.Focused():
		return editKeys
	case m.chart.enabled:
		return graphKeys
	default:
		return tableKeys
	}
}

func (m *Model) scope() keyScope {
	switch {
	case m.panel == seriesKeys && m.filtering:
		return filterKeys
	case m.panel != 0:
		return m.panel
	case m.searching:
		return historyKeys
	case m.popup:
		return completionKeys
	default:
		return m.mainScope()
	}
}

func (m *Model) available(b binding, scope keyScope) bool {
	switch b.id {
	case "graph", "legacyView":
		return m.opt.Graphics
	case "series":
		return m.opt.Graphics && !m.chart.result.Start.IsZero() && len(m.chart.visible) > 0
	case "select", "toggle":
		switch scope {
		case graphKeys:
			return m.width >= minWidth && m.height >= minHeight && len(m.chart.visible) > 0
		case seriesKeys:
			return len(m.filtered) > 0
		case rangeKeys:
			return true
		}
	case "isolate":
		return len(m.filtered) > 0
	case "all", "filter":
		return !m.chart.result.Start.IsZero() && len(m.chart.visible) > 0
	case "accept", "restore", "selectAssist":
		return len(m.options) > 0
	case "scroll":
		if scope == labelKeys {
			return len(m.labelFields) > 1
		}
		return len(m.panelBody()) > m.height-2
	case "inspectCopy":
		return m.canCopyLabels()
	case "sample":
		return len(m.table.Rows) > 0 && len(m.table.Rows[m.table.Row].Samples) > 1
	}
	switch b.id {
	case "select", "expand", "labels", "copy", "copyLabels", "sort", "page", "edge", "pan":
		return len(m.table.Rows) > 0
	}
	return true
}

func (m *Model) promQLFooter() string {
	scope := m.scope()
	var hints []string
	reserved := ""
	gap := "   "
	compact := scope == editKeys && m.width < 1+len(runHint)+len("Tab Results")+len("Esc Back")+2*len(gap)
	if compact {
		gap = "  "
	}
	for _, b := range bindings {
		if b.scope&scope == 0 || b.hint == "" || !m.available(b, scope) ||
			scope == completionKeys && (b.id == "run" || b.id == "history" || b.id == "complete" || b.id == "help") {
			continue
		}
		hint := b.hint
		if b.id == "cancelOrQuit" {
			if m.cancel != nil {
				hint = "Ctrl+C Cancel"
			} else if scope != editKeys {
				continue // Results already shows q Quit; overlays show their own actions.
			} else {
				hint = "^C Quit"
			}
		}
		if compact && b.id == "run" {
			hint = "^Enter Run"
		}
		if b.id == "expand" && m.table.Expanded {
			hint = "Enter Collapse"
		}
		if b.id == "isolate" && m.willRestoreSeries() {
			hint = "Enter Restore"
		}
		if b.id == "back" && scope == labelKeys {
			hint = "Esc Close"
		}
		if b.id == "back" {
			reserved = hint
		} else {
			hints = append(hints, hint)
			if len(m.opt.Connections) > 0 && (b.id == "results" || b.id == "edit") {
				hints = append(hints, m.connectionHint())
			}
			if b.id == "results" {
				hints = append(hints, "Alt+0–2 Tabs")
			}
			if b.id == "edit" {
				hints = append(hints, "0–2 Tabs")
			}
		}
	}
	return m.formatFooter(hints, reserved, gap)
}

func (m *Model) formatFooter(hints []string, reserved, gap string) string {
	width := 1
	budget := m.width
	if reserved != "" {
		budget -= ansi.StringWidth(reserved) + len(gap)
	}
	var visible []string
	add := func(hint string) {
		key, label, _ := strings.Cut(hint, " ")
		visible = append(visible, title.Render(key)+" "+label)
	}
	for _, hint := range hints {
		if w := ansi.StringWidth(hint); width+w <= budget {
			add(hint)
			width += w + len(gap)
		}
	}
	if reserved != "" && width+ansi.StringWidth(reserved) <= m.width {
		add(reserved)
	}
	return lipgloss.NewStyle().Width(m.width).Render(ansi.Truncate(" "+strings.Join(visible, gap), m.width, ""))
}

func (m *Model) key(k tea.KeyPressMsg) (tea.Cmd, bool) {
	scope, key := m.scope(), k.String()
	for _, b := range bindings {
		if b.scope&scope == 0 || key == "" || strings.ContainsRune(key, ' ') || !strings.Contains(" "+b.keys+" ", " "+key+" ") {
			continue
		}
		if b.id == "newline" {
			return nil, false // The textarea owns text editing.
		}
		if !m.available(b, scope) {
			if b.id == "accept" {
				m.dismiss()
			}
			return nil, true
		}
		return m.act(b.id, key, scope), true
	}
	// Unbound Tab variants in overlays must not reach text inputs or panel navigation.
	if key == "tab" || key == "shift+tab" {
		return nil, true
	}
	switch scope {
	case historyKeys, filterKeys:
		if key == "backspace" {
			m.editFilter("", true)
		} else if k.Text != "" {
			m.editFilter(k.Text, false)
		}
	}
	return nil, scope != editKeys && scope != completionKeys
}

func (m *Model) act(id, key string, scope keyScope) tea.Cmd {
	delta := 1
	switch key {
	case "up", "pgup", "left", "alt+down", "]":
		delta = -1
	}
	switch id {
	case "cancelOrQuit":
		if m.cancel != nil {
			m.stop()
			m.status = "Cancelled"
			break
		}
		fallthrough
	case "quit":
		m.dismiss()
		m.stop()
		m.alerts.stop()
		m.quitting = true
	case "run":
		m.dismiss()
		cmd := m.run()
		if cmd != nil {
			m.editor.Blur()
		}
		return cmd
	case "results":
		m.dismiss()
		m.editor.Blur()
	case "edit":
		return m.editor.Focus()
	case "back":
		switch scope {
		case editKeys, completionKeys:
			m.dismiss()
			m.editor.Blur()
		case historyKeys:
			m.dismiss()
		case filterKeys:
			m.seriesFilter = m.filterDraft
			m.refreshSeries()
			m.filtering = false
			m.panel = 0
		default:
			m.panel = 0
		}
	case "help":
		m.openPanel(helpKeys)
	case "status":
		m.openPanel(detailKeys)
	case "graph":
		return m.runMode(true)
	case "table":
		return m.runMode(false)
	case "mode", "legacyView":
		return m.runMode(!m.opt.Ranged)
	case "range":
		m.openRange()
	case "series":
		m.openSeries()
	case "select":
		switch scope {
		case graphKeys:
			m.chart.selected = min(max(0, m.chart.selected+delta), len(m.chart.visible)-1)
		case seriesKeys:
			m.seriesRow = min(max(0, m.seriesRow+delta), len(m.filtered)-1)
			m.chart.selected = m.filtered[m.seriesRow]
		case rangeKeys:
			m.rangeRow = min(max(0, m.rangeRow+delta), len(m.ranges)-1)
		default:
			m.table.Move(delta)
		}
	case "toggle":
		m.isolated = nil
		if scope == seriesKeys {
			m.chart.selected = m.filtered[m.seriesRow]
		}
		m.chart.visible[m.chart.selected] = !m.chart.visible[m.chart.selected]
		m.chart.revision++
	case "isolate":
		m.isolateSeries()
	case "all":
		m.isolated = nil
		for i := range m.chart.visible {
			m.chart.visible[i] = true
		}
		m.chart.revision++
	case "filter":
		m.filterDraft = m.seriesFilter
		m.filtering = true
	case "finishFilter":
		m.filtering = false
	case "apply":
		m.opt.Window = m.ranges[m.rangeRow]
		m.panel = 0
		return m.runMode(true)
	case "expand":
		m.table.Expanded = !m.table.Expanded
		m.table.X = 0
	case "labels":
		m.openLabels()
	case "inspectCopy":
		return copyText(m.labelText)
	case "copy", "copyLabels":
		s := m.selectedSeries().Labels
		if id == "copy" {
			if scope == graphKeys {
				row := m.chart.result.Series[m.chart.selected]
				s = row.Samples[len(row.Samples)-1].Value.String()
			} else {
				_, s, _ = m.table.Fields(m.table.Row)
			}
		}
		return copyText(s)
	case "page":
		m.table.Move(delta * m.page())
	case "edge":
		n := len(m.table.Rows) - 1
		if key == "home" {
			n = 0
		}
		m.table.Move(n - m.table.Row)
	case "pan":
		m.table.Pan(delta*8, m.width-2)
	case "sample":
		m.table.Sample(delta)
	case "sort":
		m.table.Sort(key == "v")
	case "scroll":
		step := 1
		if scope == labelKeys {
			if key == "pgup" || key == "pgdown" {
				step = max(1, m.dialogBounds(m.workspace()).height-2)
			}
			m.offset = min(max(0, m.offset+delta*step), len(m.labelFields)-1)
			break
		}
		if key == "pgup" || key == "pgdown" {
			step = m.height - 2
		}
		m.offset = min(max(0, m.offset+delta*step), max(0, len(m.panelBody())-m.height+2))
	case "history":
		m.dismiss()
		m.searchText, m.searching = "", true
		m.searchHistory()
	case "complete":
		return m.schedule(true)
	case "recall":
		m.recall(delta)
		return m.schedule(false)
	case "accept":
		m.accept()
		m.cursor = -1
		m.dismiss()
		m.diagnostic = editor.Diagnostic(m.editor.Value())
	case "restore":
		m.editor.SetValue(m.options[m.selected])
		m.cursor = -1
		return m.schedule(false)
	case "selectAssist":
		m.selected = min(max(0, m.selected+delta), len(m.options)-1)
	}
	return nil
}

func copyText(s string) tea.Cmd { return func() tea.Msg { return copied{clipboard.WriteAll(s)} } }
