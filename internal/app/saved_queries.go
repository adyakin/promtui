package app

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/editor"
	"promtui/internal/parameters"
	"promtui/internal/savedqueries"
	"promtui/internal/ui"
)

type savedAction int

const (
	noSavedAction savedAction = iota
	addSaved
	renameSaved
	updateSaved
	deleteSaved
)

func (a savedAction) label() string {
	switch a {
	case addSaved:
		return "Save"
	case renameSaved:
		return "Rename"
	case updateSaved:
		return "Update"
	case deleteSaved:
		return "Delete"
	}
	return ""
}

type savedQueriesState struct {
	snapshot savedqueries.Snapshot
	queries  []savedqueries.Query
	matches  []int
	row      int
	input    textinput.Model
	filter   string
	// Search cancellation restores the committed filter and its selection.
	searchOrigin int
	action       savedAction
	target       int
	expression   string
	openErr, err error
}

func (q *savedQueriesState) selectedIndex() int {
	if len(q.matches) == 0 {
		return -1
	}
	return q.matches[q.row]
}

func (q *savedQueriesState) rebuild(filter string, selected, fallback int) {
	needle := strings.ToLower(filter)
	q.matches = q.matches[:0]
	row := -1
	for i, query := range q.queries {
		if strings.Contains(strings.ToLower(query.Name), needle) {
			if i == selected {
				row = len(q.matches)
			}
			q.matches = append(q.matches, i)
		}
	}
	if row < 0 {
		row = min(fallback, max(0, len(q.matches)-1))
	}
	q.row = row
}

func (m *Model) openSavedQueries(save bool) tea.Cmd {
	m.openPanel(savedBrowserKeys)
	snapshot, err := savedqueries.Open(m.opt.SavedQueriesPath)
	m.savedQueries = savedQueriesState{snapshot: snapshot, queries: snapshot.Queries(), openErr: err, err: err}
	if save {
		return m.openSavedAction(addSaved)
	}
	m.savedQueries.rebuild("", -1, 0)
	return nil
}

func (m *Model) savedInput(prompt, value string) tea.Cmd {
	q := &m.savedQueries
	q.input = textinput.New()
	q.input.Prompt = prompt
	q.input.SetVirtualCursor(false)
	m.sizeSavedInput()
	q.input.SetValue(value)
	return q.input.Focus()
}

func (m *Model) editSavedInput(msg tea.Msg) tea.Cmd {
	q := &m.savedQueries
	if m.panel != savedSearchKeys && (m.panel != savedNameKeys || q.openErr != nil) {
		return nil
	}
	before := q.input.Value()
	var cmd tea.Cmd
	q.input, cmd = q.input.Update(msg)
	if q.input.Value() != before {
		if m.panel == savedSearchKeys {
			q.rebuild(q.input.Value(), q.selectedIndex(), q.row)
		}
		q.err = nil
		if q.action == addSaved {
			q.err = savedqueries.CheckExpression(q.expression)
		}
	}
	return cmd
}

func (m *Model) startSavedSearch() tea.Cmd {
	q := &m.savedQueries
	q.searchOrigin, q.err = q.selectedIndex(), nil
	m.panel = savedSearchKeys
	return m.savedInput("Search: ", q.filter)
}

func (m *Model) finishSavedSearch(cancel bool) {
	q := &m.savedQueries
	if cancel {
		q.rebuild(q.filter, q.searchOrigin, q.row)
	} else {
		q.filter = q.input.Value()
	}
	q.input.Blur()
	m.panel = savedBrowserKeys
}

func (m *Model) openSavedAction(action savedAction) tea.Cmd {
	q := &m.savedQueries
	q.action, q.target, q.err = action, q.selectedIndex(), q.openErr
	switch action {
	case addSaved, updateSaved:
		q.expression = m.editor.Value()
		if q.err == nil {
			q.err = savedqueries.CheckExpression(q.expression)
		}
	}
	switch action {
	case addSaved, renameSaved:
		m.panel = savedNameKeys
		if q.openErr != nil {
			return nil
		}
		name := ""
		if action == renameSaved {
			name = q.queries[q.target].Name
		}
		return m.savedInput("Name: ", name)
	default:
		m.panel = savedConfirmKeys
		return nil
	}
}

func (m *Model) backSavedQueries() {
	q := &m.savedQueries
	switch {
	case m.panel == savedSearchKeys:
		m.finishSavedSearch(true)
	case m.panel == savedBrowserKeys || q.action == addSaved:
		m.panel = 0
		m.savedQueries = savedQueriesState{}
	default:
		q.input.Blur()
		q.action, q.err = noSavedAction, nil
		m.panel = savedBrowserKeys
	}
}

func (m *Model) loadSavedQuery() tea.Cmd {
	browser := &m.savedQueries
	item := browser.queries[browser.selectedIndex()]
	apply := func(values parameters.Values) (tea.Cmd, error) {
		expression := item.Expression
		if item.Parameters != nil {
			var err error
			expression, err = parameters.Resolve(expression, values)
			if err != nil {
				return nil, err
			}
		}
		next := m.editor
		next.SetExactValue(expression) // Same cursor placement as history restore.
		if next.Value() != expression {
			return nil, errors.New("this expression exceeds editor limits or contains unsupported characters")
		}
		m.editor = next
		m.backSavedQueries()
		m.cursor = -1
		m.diagnostic = editor.Diagnostic(expression)
		return m.editor.Focus(), nil
	}
	if len(item.Parameters) != 0 {
		return m.openParameters(item.Name, item.Parameters, apply)
	}
	cmd, err := apply(nil)
	browser.err = err
	return cmd
}

func (m *Model) applySavedAction() tea.Cmd {
	q := &m.savedQueries
	var snapshot savedqueries.Snapshot
	var err error
	var message string
	switch q.action {
	case addSaved:
		snapshot, err = q.snapshot.Add(q.input.Value(), q.expression, nil)
		message = "Query saved"
	case renameSaved:
		snapshot, err = q.snapshot.Rename(q.target, q.input.Value())
		message = "Query renamed"
	case updateSaved:
		snapshot, err = q.snapshot.Update(q.target, q.expression, nil)
		message = "Query updated"
		if err == nil && q.queries[q.target].Expression == q.expression {
			message = "Query already up to date"
		}
	case deleteSaved:
		snapshot, err = q.snapshot.Delete(q.target)
		message = "Query deleted"
	}
	if err != nil {
		q.err = err
		return nil
	}
	if q.action != addSaved {
		selected := q.target
		if q.action == deleteSaved {
			selected = -1 // Keep the same visible row, or the previous one at the end.
		}
		q.snapshot, q.queries = snapshot, snapshot.Queries()
		q.rebuild(q.filter, selected, q.row)
		// A renamed row that no longer matches stays selected by clearing the filter.
		if q.action == renameSaved && q.selectedIndex() != selected {
			q.filter = ""
			q.rebuild("", selected, q.row)
		}
	}
	m.backSavedQueries()
	m.savedFeedbackID++
	m.savedFeedback = message
	id := savedFeedbackExpired(m.savedFeedbackID)
	return tea.Tick(2500*time.Millisecond, func(time.Time) tea.Msg { return id })
}

type savedFeedbackExpired int

func (m *Model) savedDialogWidth() int {
	width := 48
	if m.panel == savedBrowserKeys || m.panel == savedSearchKeys || m.panel == savedTemplateKeys {
		width = 56
	}
	return min(width, m.width-2)
}

func (m *Model) sizeSavedInput() {
	if m.panel == savedNameKeys || m.panel == savedSearchKeys {
		input := &m.savedQueries.input
		input.SetWidth(max(1, m.savedDialogWidth()-5-ansi.StringWidth(input.Prompt)))
	}
}

func (m *Model) savedQueryLayer() (*lipgloss.Layer, *tea.Cursor) {
	q := &m.savedQueries
	width := m.savedDialogWidth()
	inner := width - 2
	title := "Saved Queries"
	var meta string
	var rows []string
	inputRow := -1
	text := func(s string) {
		rows = append(rows, dialogSurface.Render(" "+ansi.Truncate(s, inner-2, "…")+" "))
	}
	input := func() {
		inputRow = len(rows)
		value, _ := dialogInput(q.input, inner-2)
		text(value)
	}
	// Keep the header, status and footer uncovered, including at 24×10.
	maxRows := m.height - 5
	var errorRows []string
	if q.err != nil {
		message := q.err.Error()
		var pathErr *os.PathError
		var linkErr *os.LinkError
		switch {
		case errors.Is(q.err, savedqueries.ErrChanged):
			message = "File changed. Reopen Saved Queries."
		case errors.As(q.err, &pathErr):
			message = fmt.Sprintf("queries.yaml: %s: %v", pathErr.Op, pathErr.Err)
		case errors.As(q.err, &linkErr):
			message = fmt.Sprintf("queries.yaml: %s: %v", linkErr.Op, linkErr.Err)
		}
		wrapped := strings.Split(ansi.Hardwrap(ui.Plain(message), inner-2, false), "\n")
		limit := min(len(wrapped), 3, maxRows-3)
		if limit < len(wrapped) {
			wrapped[limit-1] = ansi.Truncate(wrapped[limit-1], inner-3, "") + "…"
		}
		errorRows = wrapped[:limit]
	}
	switch m.panel {
	case savedBrowserKeys, savedSearchKeys:
		if m.panel == savedSearchKeys {
			input()
		} else if q.filter != "" {
			text("Search: " + ui.Plain(q.filter))
		}
		if len(q.matches) == 0 && q.err == nil {
			if len(q.queries) == 0 {
				text("No saved queries yet")
				text("Ctrl+S in editor")
			} else {
				text("No matching queries")
			}
		}
		room := min(maxRows-len(rows)-len(errorRows), 10)
		first := dialogStart(q.row, len(q.matches), room)
		if len(q.matches) > 0 {
			meta = fmt.Sprintf("%d/%d", q.row+1, len(q.matches))
		}
		for i := first; i < min(first+room, len(q.matches)); i++ {
			rows = append(rows, dialogRow(ui.Plain(q.queries[q.matches[i]].Name), "", inner, i == q.row))
		}
	case savedNameKeys:
		title = "Save Query"
		if q.action == renameSaved {
			title = "Rename Saved Query"
		}
		if q.openErr == nil {
			input()
		}
	case savedConfirmKeys:
		title = q.action.label() + " Saved Query"
		if q.action == updateSaved {
			text("Replace with editor contents?")
		} else {
			text("Delete this saved query?")
		}
		text(fmt.Sprintf("%q", ui.Plain(q.queries[q.target].Name)))
	}
	for _, line := range errorRows {
		rows = append(rows, dialogSurface.Foreground(lipgloss.Color("11")).Render(" "+line+" "))
	}
	x := (m.width - width) / 2
	y := 1 + (m.height-3-len(rows)-2)/2
	layer := lipgloss.NewLayer(renderFrame(title, meta, rows, width, true, dialogSurface)).X(x).Y(y).Z(1)
	if inputRow >= 0 && !m.quitting {
		if q.input.Focused() {
			_, col := dialogInput(q.input, inner-2)
			return layer, nativeCursor(min(x+width-2, x+2+col), y+1+inputRow)
		}
	}
	return layer, nil
}
