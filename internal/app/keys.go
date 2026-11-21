package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/editor"
	"promtui/internal/programstatus"
	"promtui/internal/ui"
)

type keyScope uint32

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
	savedBrowserKeys
	savedSearchKeys
	savedNameKeys
	savedConfirmKeys
	queryErrorKeys
	savedTemplateKeys keyScope = 1 << 16
	savedQueryKeys             = savedBrowserKeys | savedSearchKeys | savedNameKeys | savedConfirmKeys
	resultKeys                 = tableKeys | graphKeys
	mainKeys                   = editKeys | resultKeys
	scrollKeys                 = helpKeys | detailKeys | labelKeys | queryErrorKeys
	allKeys                    = mainKeys | completionKeys | historyKeys | scrollKeys | rangeKeys | seriesKeys | filterKeys | savedQueryKeys | savedTemplateKeys
)

type binding struct {
	id, keys, label, hint string
	scope                 keyScope
}

const runHint = "Ctrl+Enter Run"

// Order is footer priority. Actions without a hint are listed only in Help.
var bindings = []binding{
	{"run", "ctrl+enter ctrl+g", "Ctrl+Enter / Ctrl+G", runHint, editKeys | completionKeys},
	{"results", "tab shift+tab", "Tab / Shift+Tab", "Tab Results", editKeys},
	{"saveQuery", "ctrl+s", "Ctrl+S", "Ctrl+S Save", editKeys | completionKeys},
	{"browseQueries", "ctrl+o", "Ctrl+O", "Ctrl+O Saved", editKeys | completionKeys},
	{"history", "ctrl+r", "Ctrl+R", "Ctrl+R History", editKeys | completionKeys},
	{"complete", "ctrl+space ctrl+@", "Ctrl+Space", "Ctrl+Space Complete", editKeys | completionKeys},
	{"queryError", "e", "e", "e Error details", resultKeys},
	{"edit", "i tab shift+tab", "i / Tab / Shift+Tab", "i Edit", resultKeys},
	{"graph", "g", "g", "g Graph", resultKeys},
	{"table", "t", "t", "t Table", resultKeys},
	{"series", "s", "s", "s Series", resultKeys},
	{"range", "r", "r", "r Range", resultKeys},
	{"quit", "q", "q", "q Quit", resultKeys},
	{"filterResults", "/", "/", "/ Filter", resultKeys},
	{"browseQueries", "ctrl+o", "Ctrl+O", "Ctrl+O Saved", resultKeys},
	{"select", "up down", "↑ / ↓", "↑↓ Select", resultKeys | rangeKeys | seriesKeys},
	{"toggle", "space", "Space", "Space Toggle", graphKeys | seriesKeys},
	{"expand", "enter", "Enter", "Enter Expand", tableKeys},
	{"sort", "n v", "n / v", "n/v Sort", tableKeys},
	{"labels", "l", "l", "l Labels", resultKeys},
	{"copy", "y", "y", "y Copy", resultKeys},
	{"run", "ctrl+enter ctrl+g", "Ctrl+Enter / Ctrl+G", runHint, resultKeys},
	{"accept", "tab", "Tab", "Tab Accept", completionKeys},
	{"restore", "enter", "Enter", "Enter Restore", historyKeys},
	{"apply", "enter", "Enter", "Enter Run", rangeKeys},
	{"isolate", "enter", "Enter", "Enter Isolate", seriesKeys},
	{"all", "a", "a", "a All", seriesKeys},
	{"filter", "/", "/", "/ Filter", seriesKeys},
	{"finishFilter", "enter", "Enter", "Enter Done", filterKeys},
	{"selectAssist", "up down", "↑ / ↓", "↑↓ Select", completionKeys | historyKeys},
	{"inspectCopy", "y", "y", "y Copy labels", labelKeys},
	{"copyQueryError", "y", "y", "y Copy", queryErrorKeys},
	{"scroll", "up down pgup pgdown", "↑ / ↓ / PgUp / PgDn", "↑↓ Scroll", scrollKeys},
	{"savedLoad", "enter", "Enter", "Enter Load", savedBrowserKeys},
	{"savedSelect", "up down", "↑ / ↓", "↑↓ Select", savedBrowserKeys | savedSearchKeys},
	{"savedSearch", "/", "/", "/ Search", savedBrowserKeys},
	{"savedRename", "e", "e", "e Rename", savedBrowserKeys},
	{"savedUpdate", "u", "u", "u Update", savedBrowserKeys},
	{"savedCreateTemplate", "p", "p", "p Create", savedBrowserKeys},
	{"savedDelete", "d", "d", "d Delete", savedBrowserKeys},
	{"savedSearchDone", "enter", "Enter", "Enter Done", savedSearchKeys},
	{"savedApply", "enter", "Enter", "Enter Confirm", savedNameKeys | savedConfirmKeys},
	{"savedTemplateApply", "ctrl+enter ctrl+g", "Ctrl+Enter", "Ctrl+Enter Apply", savedTemplateKeys},
	{"savedTemplateFocus", "tab up down shift+tab", "Tab Next", "Tab Next", savedTemplateKeys},
	{"savedTemplateAdd", "ctrl+a", "^A Add", "^A Add", savedTemplateKeys},
	{"savedTemplateRemove", "ctrl+d", "^D Remove", "^D Remove", savedTemplateKeys},
	{"cancelOrQuit", "ctrl+c", "Ctrl+C", "Ctrl+C Quit", allKeys},
	{"back", "esc", "Esc", "Esc Back", allKeys &^ resultKeys},
	{"help", "?", "?", "? Help", resultKeys | helpKeys | detailKeys},
	{"status", "alt+s", "Alt+S", "Alt+S Status", mainKeys | completionKeys | detailKeys | helpKeys},
	{"page", "pgup pgdown", "PgUp / PgDn", "", tableKeys},
	{"edge", "home end", "Home / End", "", tableKeys},
	{"pan", "left right", "← / →", "", tableKeys},
	{"sample", "[ ]", "[ / ]", "", tableKeys},
	{"copyLabels", "Y shift+y", "Y", "", resultKeys},
	{"recall", "alt+up alt+down", "Alt+Up / Alt+Down", "", editKeys | completionKeys},
	{"newline", "enter", "Enter", "", editKeys | completionKeys},
	{"mode", "ctrl+t", "Ctrl+T", "", mainKeys},
	{"range", "ctrl+w", "Ctrl+W", "", resultKeys},
	{"quit", "ctrl+q", "Ctrl+Q", "", allKeys},
}

func (m *Model) mainScope() keyScope {
	switch {
	case m.editor.Focused():
		return editKeys
	case m.graphView():
		return graphKeys
	default:
		return tableKeys
	}
}

func (m *Model) scope() keyScope {
	switch {
	case m.resultFilter.input.Focused() || m.panel == seriesKeys && m.filtering:
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
	case "queryError", "copyQueryError":
		return m.queryPhase == queryFailed
	case "graph":
		return m.opt.Graphics
	case "series":
		return m.opt.Graphics && !m.chart.result.Start.IsZero() && len(m.chart.visible) > 0
	case "filterResults":
		return len(m.resultFilter.allRows) > 0
	case "select", "toggle":
		switch scope {
		case graphKeys:
			return m.width >= minWidth && m.height >= minHeight && m.graphSeriesCount() > 0
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
	case "savedLoad", "savedSelect", "savedRename", "savedUpdate", "savedDelete":
		return len(m.savedQueries.matches) > 0
	case "savedSearch":
		return len(m.savedQueries.queries) > 0
	case "savedApply":
		return m.savedQueries.openErr == nil
	case "scroll":
		if scope == queryErrorKeys {
			_, _, limit := m.errorViewport()
			return limit > 0
		}
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
		if scope == graphKeys {
			return m.graphSeriesCount() > 0
		}
		return len(m.table.Rows) > 0
	}
	return true
}

func (m *Model) promQLFooter() string {
	scope := m.scope()
	var hints []string
	reserved := ""
	gap := "   "
	if scope&savedQueryKeys != 0 || scope == savedTemplateKeys {
		gap = "  "
	}
	if scope == queryErrorKeys {
		gap = " "
	}
	compact := scope == editKeys && m.width < 1+len(runHint)+len("Tab Results")+len("Esc Back")+2*len(gap)
	if compact {
		gap = "  "
	}
	for _, b := range bindings {
		if b.scope&scope == 0 || b.hint == "" || !m.available(b, scope) ||
			scope == completionKeys && (b.id == "run" || b.id == "history" || b.id == "complete" ||
				b.id == "saveQuery" || b.id == "browseQueries") {
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
		if b.id == "back" && (scope == labelKeys || scope == queryErrorKeys) {
			hint = "Esc Close"
		}
		if b.id == "back" && scope == filterKeys && m.resultFilter.input.Focused() {
			hint = "Esc Cancel"
		}
		if b.id == "scroll" && scope == queryErrorKeys && m.width < 27 {
			hint = "↑↓"
		}
		if b.id == "back" && (scope&savedQueryKeys != 0 || scope == savedTemplateKeys) {
			hint = "Esc Cancel"
			if scope == savedBrowserKeys {
				hint = "Esc Close"
			}
		}
		if b.id == "savedApply" {
			hint = "Enter " + m.savedQueries.action.label()
			if len(hint)+len("Esc Cancel")+len(gap)+1 > m.width {
				hint = "Enter OK"
			}
		}
		if b.id == "back" {
			reserved = hint
		} else {
			hints = append(hints, hint)
			if len(m.opt.Connections) > 0 && (b.id == "results" || b.id == "edit") {
				hints = append(hints, m.connectionHint())
			}
			if b.id == "results" {
				hints = append(hints, "Alt+0–5 Tabs")
			}
			if b.id == "edit" {
				hints = append(hints, "0–5 Tabs")
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
		text := title.Render(key)
		if label != "" {
			text += " " + label
		}
		visible = append(visible, text)
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
	if k.Keystroke() == "alt+s" {
		key = "alt+s"
	}
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
	case savedNameKeys, savedSearchKeys:
		return m.editSavedInput(k), true
	case savedTemplateKeys:
		return m.editTemplate(k), true
	case historyKeys, filterKeys:
		if m.resultFilter.input.Focused() {
			return m.editResultFilter(k), true
		}
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
		if m.stop() {
			m.reportProgramStatus(programstatus.Idle, "")
			m.clearQueryResults()
			m.queryPhase = queryCancelled
			m.status, m.summary = "Cancelled", "Query cancelled"
			break
		}
		fallthrough
	case "quit":
		m.dismiss()
		m.stop()
		m.alerts.stop()
		m.silences.stop()
		m.targets.stop(nil)
		m.routing.stop()
		m.rules.stop()
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
		case savedBrowserKeys, savedSearchKeys, savedNameKeys, savedConfirmKeys:
			m.backSavedQueries()
		case savedTemplateKeys:
			m.backSavedTemplate()
		case editKeys, completionKeys:
			m.dismiss()
			m.editor.Blur()
		case historyKeys:
			m.dismiss()
		case filterKeys:
			if m.resultFilter.input.Focused() {
				r := &m.resultFilter
				r.input.Blur()
				if r.input.Value() != r.before {
					r.input.SetValue(r.before)
					m.refreshResultFilter()
				}
				m.table, r.anchor = r.anchor, ui.Table{}
				break
			}
			m.seriesFilter = m.filterDraft
			m.refreshSeries()
			m.filtering = false
			m.panel = 0
		default:
			m.panel = 0
		}
	case "help":
		if scope == helpKeys {
			m.panel = 0
		} else {
			m.openPanel(helpKeys)
		}
	case "status":
		if scope == detailKeys {
			m.panel = 0
		} else {
			m.openPanel(detailKeys)
		}
	case "queryError":
		m.openPanel(queryErrorKeys)
	case "copyQueryError":
		text := copyQueryError(m.queryError.full)
		return func() tea.Msg { return text }
	case "graph":
		return m.runMode(true)
	case "table":
		return m.runMode(false)
	case "mode":
		return m.runMode(!m.opt.Ranged)
	case "range":
		m.openRange()
	case "series":
		m.openSeries()
	case "select":
		switch scope {
		case graphKeys:
			row := m.graphRow()
			if m.graphIndex(row) == m.chart.selected {
				row = min(max(0, row+delta), m.graphSeriesCount()-1)
			}
			m.chart.selected = m.graphIndex(row)
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
		} else {
			m.chart.selected = m.graphIndex(m.graphRow())
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
	case "filterResults":
		r := &m.resultFilter
		r.before, r.anchor = r.input.Value(), m.table
		r.input.CursorEnd()
		return r.input.Focus()
	case "finishFilter":
		if m.resultFilter.input.Focused() {
			m.resultFilter.input.Blur()
			m.resultFilter.anchor = ui.Table{}
			break
		}
		m.filtering = false
	case "apply":
		m.opt.Window = m.ranges[m.rangeRow]
		m.panel = 0
		m.setQueryMode(true)
		return m.run()
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
				row := m.selectedSeries()
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
		m.sortResults(key == "v")
	case "scroll":
		step := 1
		if scope == queryErrorKeys {
			_, room, limit := m.errorViewport()
			if key == "pgup" || key == "pgdown" {
				step = room
			}
			m.offset = min(max(0, min(m.offset, limit)+delta*step), limit)
			break
		}
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
	case "saveQuery":
		return m.openSavedQueries(true)
	case "browseQueries":
		return m.openSavedQueries(false)
	case "savedSelect":
		q := &m.savedQueries
		q.row = min(max(0, q.row+delta), len(q.matches)-1)
	case "savedLoad":
		return m.loadSavedQuery()
	case "savedSearch":
		return m.startSavedSearch()
	case "savedSearchDone":
		m.finishSavedSearch(false)
	case "savedRename":
		return m.openSavedAction(renameSaved)
	case "savedUpdate":
		q := &m.savedQueries
		if idx := q.selectedIndex(); idx >= 0 && len(q.queries[idx].Parameters) > 0 {
			return m.openSavedTemplateEditor(idx) // Explicit template editing for parameterized items.
		}
		return m.openSavedAction(updateSaved)
	case "savedCreateTemplate":
		if q := &m.savedQueries; q.selectedIndex() >= 0 {
			return m.openSavedTemplateEditor(q.selectedIndex())
		}
		return nil
	case "savedDelete":
		return m.openSavedAction(deleteSaved)
	case "savedApply":
		return m.applySavedAction()
	case "savedTemplateApply":
		return m.applySavedTemplate()
	case "savedTemplateFocus":
		delta := 1
		if key == "up" || key == "shift+tab" {
			delta = -1
		}
		return m.moveTemplateFocus(delta)
	case "savedTemplateAdd":
		return m.addTemplateParam()
	case "savedTemplateRemove":
		return m.removeTemplateParam()
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
