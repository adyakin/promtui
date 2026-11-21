package app

import (
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/parameters"
	"promtui/internal/savedqueries"
	"promtui/internal/ui"
)

// The expression template editor. Parameters definitions are edited as
// single-line name/default pairs; the expression is a multiline textarea.
type savedTemplateRow struct {
	nameIn textinput.Model
	defIn  textinput.Model
}

type savedTemplateState struct {
	target int // index into savedQueries.queries
	rows   []savedTemplateRow
	text   textarea.Model
	focus  int // 0..2n: two fields per parameter row; 2n is the expression
	err    error
	active bool
}

func savedTemplateInput(value string) textinput.Model {
	input := textinput.New()
	input.SetVirtualCursor(false)
	input.Prompt = ""
	input.SetValue(value)
	return input
}

func (t *savedTemplateState) resize(w, h int) {
	if t == nil || !t.active {
		return
	}
	width := min(56, max(1, w-2))
	for i := range t.rows {
		t.rows[i].nameIn.SetWidth(max(1, width/3-3))
		t.rows[i].defIn.SetWidth(max(1, width-width/3-3))
	}
	t.text.SetWidth(max(1, width-4))
	t.text.SetHeight(max(2, min(8, h-3-2*len(t.rows)-4)))
}

func (t *savedTemplateState) fieldCount() int {
	return 2*len(t.rows) + 1
}

// openSavedTemplateEditor loads the stored template for target, or converts a
// plain item by seeding its references as parameter rows. The editor never
// touches the PromQL editor while it is open.
func (m *Model) openSavedTemplateEditor(target int) tea.Cmd {
	t := &m.savedTemplate
	*t = savedTemplateState{target: target}
	query := m.savedQueries.queries[target]
	for _, p := range query.Parameters {
		row := savedTemplateRow{nameIn: savedTemplateInput(p.Name)}
		if p.Default != nil {
			row.defIn = savedTemplateInput(*p.Default)
		}
		t.rows = append(t.rows, row)
	}
	if len(t.rows) == 0 {
		// Converting a plain item: its references become the starting set.
		if refs, err := parameters.References(query.Expression); err == nil {
			for _, r := range refs {
				t.rows = append(t.rows, savedTemplateRow{nameIn: savedTemplateInput(r)})
			}
		}
	}
	t.text = textarea.New()
	t.text.SetValue(query.Expression)
	t.active = true
	t.resize(m.width, m.height)
	t.focus = 0
	m.editor.Blur()
	m.panel = savedTemplateKeys
	return m.templateFocusNow()
}

func (m *Model) backSavedTemplate() {
	t := &m.savedTemplate
	for i := range t.rows {
		t.rows[i].nameIn.Blur()
		t.rows[i].defIn.Blur()
	}
	t.text.Blur()
	t.active = false
	m.panel = savedBrowserKeys
}

func (m *Model) templateFocusNow() tea.Cmd {
	t := &m.savedTemplate
	for i := range t.rows {
		t.rows[i].nameIn.Blur()
		t.rows[i].defIn.Blur()
	}
	t.text.Blur()
	if t.focus < 2*len(t.rows) {
		row := t.rows[t.focus/2]
		if t.focus%2 == 0 {
			return row.nameIn.Focus()
		}
		return row.defIn.Focus()
	}
	return t.text.Focus()
}

func (m *Model) moveTemplateFocus(delta int) tea.Cmd {
	t := &m.savedTemplate
	t.focus = (t.focus + delta + t.fieldCount()) % t.fieldCount()
	return m.templateFocusNow()
}

func (m *Model) editTemplate(msg tea.Msg) tea.Cmd {
	t := &m.savedTemplate
	if t.focus >= 2*len(t.rows) {
		if !t.text.Focused() {
			return m.templateFocusNow()
		}
		before := t.text.Value()
		var cmd tea.Cmd
		t.text, cmd = t.text.Update(msg)
		if t.text.Value() != before {
			t.err = nil
		}
		return cmd
	}
	row := &t.rows[t.focus/2]
	if !row.nameIn.Focused() && !row.defIn.Focused() {
		return m.templateFocusNow()
	}
	var cmd tea.Cmd
	if t.focus%2 == 0 {
		before := row.nameIn.Value()
		row.nameIn, cmd = row.nameIn.Update(msg)
		if row.nameIn.Value() != before {
			t.err = nil
		}
	} else {
		before := row.defIn.Value()
		row.defIn, cmd = row.defIn.Update(msg)
		if row.defIn.Value() != before {
			t.err = nil
		}
	}
	return cmd
}

func (m *Model) addTemplateParam() tea.Cmd {
	t := &m.savedTemplate
	t.rows = append(t.rows, savedTemplateRow{nameIn: savedTemplateInput("")})
	t.focus = 2 * (len(t.rows) - 1)
	return m.templateFocusNow()
}

func (m *Model) removeTemplateParam() tea.Cmd {
	t := &m.savedTemplate
	if t.focus >= 2*len(t.rows) {
		return nil
	}
	t.rows = slices.Delete(t.rows, t.focus/2, t.focus/2+1)
	t.focus = min(t.focus, 2*len(t.rows))
	return m.templateFocusNow()
}

func (m *Model) applySavedTemplate() tea.Cmd {
	t := &m.savedTemplate
	defs := make([]parameters.Parameter, 0, len(t.rows))
	for _, row := range t.rows {
		def := (*string)(nil)
		if v := row.defIn.Value(); v != "" {
			def = &v
		}
		defs = append(defs, parameters.Parameter{Name: row.nameIn.Value(), Default: def})
	}
	expr := t.text.Value()
	var err error
	if err = savedqueries.CheckExpression(expr); err == nil && len(defs) > 0 {
		err = parameters.Validate(defs, expr)
	}
	if err != nil {
		t.err = err
		return nil
	}
	q := &m.savedQueries
	snapshot, err := q.snapshot.Update(t.target, expr, defs)
	if err != nil {
		t.err = err
		return nil
	}
	message := "Query updated"
	if q.queries[t.target].Expression == expr {
		message = "Query already up to date"
	}
	q.snapshot, q.queries = snapshot, snapshot.Queries()
	q.rebuild(q.filter, t.target, q.row)
	m.backSavedTemplate()
	m.savedFeedbackID++
	m.savedFeedback = message
	id := savedFeedbackExpired(m.savedFeedbackID)
	return tea.Tick(2500*time.Millisecond, func(time.Time) tea.Msg { return id })
}

func (m *Model) savedTemplateLayer() (*lipgloss.Layer, *tea.Cursor) {
	t := &m.savedTemplate
	width := m.savedDialogWidth()
	inner := width - 2
	var rows []string
	var errorRows []string
	if t.err != nil {
		wrapped := strings.Split(ansi.Hardwrap(ui.Plain(t.err.Error()), inner-2, false), "\n")
		errorRows = wrapped[:min(len(wrapped), 3)]
	}
	room := m.height - 5 - len(errorRows)
	appendRow := func(line string, selected bool) {
		if room <= 0 {
			return
		}
		rows = append(rows, dialogRow(line, "", inner, selected))
		room--
	}
	for i, row := range t.rows {
		appendRow(ui.Plain(row.nameIn.View()), 2*i == t.focus)
		appendRow(ui.Plain(row.defIn.View()), 2*i+1 == t.focus)
	}
	textRow := -1
	for i, line := range strings.Split(ui.Plain(t.text.View()), "\n") {
		if textRow < 0 {
			textRow = len(rows)
		}
		appendRow(line, t.focus == 2*len(t.rows) && i == 0)
	}
	if textRow < 0 {
		textRow = len(rows) - 1
	}
	for _, line := range errorRows {
		rows = append(rows, dialogSurface.Foreground(lipgloss.Color("11")).Render(" "+line+" "))
	}
	x := (m.width - width) / 2
	y := 1 + (m.height-3-len(rows)-2)/2
	layer := lipgloss.NewLayer(renderFrame("Edit Expression Template", "", rows, width, true, dialogSurface)).X(x).Y(y).Z(1)
	if !m.quitting {
		if t.focus < 2*len(t.rows) {
			row := t.rows[t.focus/2]
			input := row.nameIn
			if t.focus%2 != 0 {
				input = row.defIn
			}
			if c := input.Cursor(); c != nil {
				return layer, nativeCursor(min(x+width-2, x+3+c.X), y+1+t.focus/2)
			}
		} else if c := t.text.Cursor(); c != nil {
			return layer, nativeCursor(min(x+width-2, x+3+c.X), y+1+textRow)
		}
	}
	return layer, nil
}
