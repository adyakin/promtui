package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/config"
	"promtui/internal/editor"
	"promtui/internal/history"
	"promtui/internal/savedqueries"
)

func savedCtrl(m *Model, key rune) {
	m.Update(tea.KeyPressMsg{Code: key, Mod: tea.ModCtrl})
}

func savedPaste(m *Model, text string) { m.Update(tea.PasteMsg{Content: text}) }

func savedText(t *testing.T, screen, text string) {
	t.Helper()
	if !strings.Contains(ansi.Strip(screen), text) {
		t.Fatalf("missing %q:\n%s", text, ansi.Strip(screen))
	}
}

func savedFixture(t *testing.T, names ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "queries.yaml")
	s, err := savedqueries.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		s, err = s.Add(name, "up\n+ 1", nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestSaveQueryDialogAndEditorFocus(t *testing.T) {
	m := New(Options{SavedQueriesPath: savedFixture(t, "Existing")}, nil)
	press(m, 'i')
	m.editor.SetValue("  up\n + 2\n")
	m.editor.CursorUp()
	m.editor.SetCursorColumn(2)
	want, pos := m.editor.Value(), editor.Position(m.editor)
	savedCtrl(m, 's')
	savedText(t, m.screen(), "Save Query")
	savedText(t, m.footer(), "Enter Save")
	press(m, tea.KeyEnter)
	savedText(t, m.screen(), "enter a name")
	savedPaste(m, "  eXisting ")
	press(m, tea.KeyEnter)
	savedText(t, m.screen(), "already exists")
	press(m, tea.KeyEsc)
	if m.scope() != editKeys || editor.Position(m.editor) != pos {
		t.Fatal("cancel did not return to the editor cursor")
	}
	savedCtrl(m, 's')
	savedPaste(m, "  New  ")
	press(m, tea.KeyEnter)
	savedText(t, m.screen(), "Query saved")
	if m.scope() != editKeys || m.editor.Value() != want || editor.Position(m.editor) != pos || m.cancel != nil || len(m.entries) != 0 {
		t.Fatal("save modified the editor or ran the query")
	}
	s, err := savedqueries.Open(m.opt.SavedQueriesPath)
	got := s.Queries()
	if err != nil || len(got) != 2 || got[1].Name != "New" || got[1].Expression != want || got[1].Parameters != nil {
		t.Fatalf("wrong persisted query: %v, %v", got, err)
	}
	m.Update(savedFeedbackExpired(m.savedFeedbackID))
	if m.savedFeedback != "" {
		t.Fatal("confirmation should expire")
	}
}

func TestBrowseSearchAndLoadWithoutExecution(t *testing.T) {
	opts := Options{
		SavedQueriesPath: savedFixture(t, "CPU North", "Memory", "CPU South"),
		Window:           time.Hour,
		Ranged:           true,
		Connection:       config.Connection{Name: "backend"},
	}
	m := New(opts, []history.Entry{{Query: "history is independent"}})
	m.Update(completed{result: sampleResult(false)})
	m.editor.SetValue("my draft")
	savedCtrl(m, 'o')
	savedText(t, m.footer(), "Enter Load")
	press(m, tea.KeyDown, tea.KeyDown, '/')
	savedPaste(m, "cPu")
	if got := m.savedQueries.selectedIndex(); got != 2 || len(m.savedQueries.matches) != 2 {
		t.Fatalf("filter failed to preserve selected match: %d", got)
	}
	savedText(t, m.footer(), "Esc Cancel")
	savedPaste(m, " not present")
	savedText(t, m.screen(), "No matching queries")
	press(m, tea.KeyEsc)
	if m.savedQueries.filter != "" || m.savedQueries.selectedIndex() != 2 {
		t.Fatal("cancel did not restore previous selection")
	}
	press(m, '/', 'm', 'e', 'm', tea.KeyEnter)
	if m.savedQueries.filter != "mem" || m.savedQueries.selectedIndex() != 1 {
		t.Fatal("search did not select the matching row")
	}
	m.Update(tea.KeyPressMsg{Code: '1', Mod: tea.ModAlt})
	press(m, tea.KeyTab)
	if m.activeTab != promQLTab || m.scope() != savedBrowserKeys || m.editor.Value() != "my draft" {
		t.Fatal("browser leaked input to the tab shell or editor")
	}
	press(m, tea.KeyEnter)
	if m.scope() != editKeys || m.editor.Value() != "up\n+ 1" || m.editor.Line() != 1 || m.editor.Column() != 3 {
		t.Fatal("load did not restore the expression and editor focus")
	}
	if m.id != 0 || m.cancel != nil || len(m.table.Rows) != 2 || m.opt.Connection.Name != opts.Connection.Name ||
		!m.opt.Ranged || m.opt.Window != opts.Window || len(m.entries) != 1 || m.entries[0].Query != "history is independent" {
		t.Fatal("load modified execution state, results, or history")
	}
	savedCtrl(m, 'r')
	savedText(t, m.screen(), "history is independent")
}

func TestSavedBrowserRenameUpdateAndDelete(t *testing.T) {
	m := New(Options{SavedQueriesPath: savedFixture(t, "CPU", "Memory")}, nil)
	m.resize(100, 24)
	m.editor.SetValue("vector(5)")
	savedCtrl(m, 'o')
	press(m, 'e')
	savedText(t, m.screen(), "Name: CPU")
	savedText(t, m.footer(), "Enter Rename")
	press(m, tea.KeyEsc, 'e')
	savedCtrl(m, 'u') // Clear the prefilled name using the input's editing shortcut.
	savedPaste(m, "Load average")
	press(m, tea.KeyEnter)
	q := &m.savedQueries
	if q.selectedIndex() != 0 || q.queries[0].Name != "Load average" || q.queries[0].Expression != "up\n+ 1" {
		t.Fatalf("rename changed expression or selection: %v", q.queries)
	}
	press(m, 'u')
	savedText(t, m.screen(), "Update Saved Query")
	savedText(t, m.footer(), "Enter Update")
	press(m, tea.KeyEsc)
	if q.queries[0].Expression != "up\n+ 1" {
		t.Fatal("cancelled update modified saved query")
	}
	press(m, 'u', tea.KeyEnter)
	if m.scope() != savedBrowserKeys || q.queries[0].Expression != "vector(5)" ||
		q.queries[1].Expression != "up\n+ 1" || m.editor.Value() != "vector(5)" || m.cancel != nil {
		t.Fatal("confirmed update changed the wrong state")
	}
	press(m, 'd')
	savedText(t, m.screen(), `"Load average"`)
	savedText(t, m.footer(), "Enter Delete")
	press(m, tea.KeyEsc)
	if len(q.queries) != 2 {
		t.Fatal("cancelled deletion modified collection")
	}
	press(m, 'd', tea.KeyEnter)
	if m.scope() != savedBrowserKeys || q.selectedIndex() != 0 || q.queries[0].Name != "Memory" {
		t.Fatal("deletion did not select the nearest remaining query")
	}
	press(m, 'd', tea.KeyEnter)
	savedText(t, m.screen(), "No saved queries yet")
	press(m, tea.KeyEnter, tea.KeyEsc)
	if m.scope() != tableKeys || m.editor.Value() != "vector(5)" {
		t.Fatal("closing browser did not restore Results focus and editor text")
	}
}

func TestSavedDialogsRejectBlankExpressionsAndCorruptStorage(t *testing.T) {
	path := savedFixture(t, "CPU")
	m := New(Options{SavedQueriesPath: path}, nil)
	press(m, 'i')
	m.editor.SetValue(" \n ")
	savedCtrl(m, 's')
	savedPaste(m, "New")
	press(m, tea.KeyEnter)
	savedText(t, m.screen(), "enter a PromQL expression")
	press(m, tea.KeyEsc)
	savedCtrl(m, 'o')
	press(m, 'u', tea.KeyEnter)
	savedText(t, m.screen(), "enter a PromQL expression")
	press(m, tea.KeyEsc, 'd')
	const broken = "queries: ["
	if err := os.WriteFile(path, []byte(broken), 0600); err != nil {
		t.Fatal(err)
	}
	press(m, tea.KeyEnter)
	savedText(t, m.screen(), "File changed. Reopen Saved Queries.")
	press(m, tea.KeyEsc, tea.KeyEsc)
	savedCtrl(m, 'o')
	savedText(t, m.screen(), "invalid queries.yaml")
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != broken {
		t.Fatalf("corrupt file was altered: %q, %v", raw, err)
	}
}

func TestSavedOverlayKeepsKittyPlacementAndBounds(t *testing.T) {
	path := savedFixture(t, "A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "Last")
	m := New(Options{SavedQueriesPath: path, Graphics: true}, nil)
	m.update(completed{result: sampleResult(true)})
	place, work := m.graphPlace(), m.workspace()
	if place.cols == 0 {
		t.Fatal("test needs a visible graph")
	}
	m.chart.displayed, m.chart.wanted = place, place
	savedCtrl(m, 'o')
	if m.workspace() != work || m.graphPlace() != place || m.chart.displayed != place || m.chart.sending {
		t.Fatal("browser moved or triggered a clear of the Kitty graph")
	}
	layer, _ := m.savedQueryLayer()
	if !strings.Contains(layer.GetContent(), "\x1b[48;") {
		t.Fatal("overlay needs an explicit background above Kitty pixels")
	}
	for range 11 {
		press(m, tea.KeyDown)
	}
	m.resize(minWidth, minHeight)
	check := func() {
		t.Helper()
		rows := strings.Split(ansi.Strip(m.screen()), "\n")
		if len(rows) != minHeight || !strings.Contains(rows[0], "[0]") || !strings.Contains(rows[minHeight-1], "Esc") {
			t.Fatalf("dialog escaped screen bounds:\n%s", strings.Join(rows, "\n"))
		}
		for _, row := range rows {
			if ansi.StringWidth(row) > minWidth {
				t.Fatal(row)
			}
		}
	}
	check()
	savedText(t, m.screen(), "› Last")
	press(m, 'e')
	check()
	if m.View().Cursor == nil {
		t.Fatal("name dialog has no cursor")
	}
	press(m, tea.KeyEsc, tea.KeyEsc)
	if m.scope() != graphKeys || m.chart.selected != 0 {
		t.Fatal("closing browser did not restore graph focus")
	}
}

func TestSavedPromQLTemplateEditingAndRawLoading(t *testing.T) {
	path := savedFixture(t, "MetricsQL")
	m := New(Options{SavedQueriesPath: path}, nil)
	press(m, 'i')
	m.editor.SetValue("existing draft")
	savedCtrl(m, 'o')
	press(m, 't')
	savedText(t, m.screen(), "Edit Expression Template")
	m.Update(tea.KeyPressMsg{Code: '1', Mod: tea.ModAlt})
	if m.activeTab != promQLTab || m.panel != savedTemplateKeys {
		t.Fatal("template editor leaked a tab switch")
	}
	press(m, tea.KeyEsc, 't')
	const template = `expression: |-
  WITH (p = up{instance=~"${instance}"})
  p + "${required}" + "$${literal}"
parameters:
  - name: instance
    default: "srv.*"
  - name: required
`
	m.savedTemplate.editor.text.Reset()
	savedPaste(m, template)
	savedCtrl(m, tea.KeyEnter)
	if m.panel != savedBrowserKeys || m.editor.Value() != "existing draft" {
		t.Fatalf("saving a template changed the editor or failed: %s", ansi.Strip(m.screen()))
	}
	press(m, tea.KeyEnter)
	savedCtrl(m, tea.KeyEnter)
	if m.parameterDialog == nil || m.parameterDialog.err == nil || m.editor.Value() != "existing draft" {
		t.Fatal("applying without a required value changed the editor")
	}
	press(m, tea.KeyEsc, tea.KeyEnter)
	press(m, tea.KeyTab)
	savedPaste(m, "${instance}")
	savedCtrl(m, tea.KeyEnter)
	const want = "WITH (p = up{instance=~\"srv.*\"})\np + \"${instance}\" + \"${literal}\""
	if m.editor.Value() != want || m.id != 0 || len(m.entries) != 0 {
		t.Fatalf("invalid load or automatic execution: %q", m.editor.Value())
	}
	savedCtrl(m, 'o')
	press(m, 'u')
	text := m.savedTemplate.editor.text.Value()
	if !strings.Contains(text, "${required}") || !strings.Contains(text, "$${literal}") || !strings.Contains(text, "default:") {
		t.Fatalf("update replaced template or definitions: %s", text)
	}
	press(m, tea.KeyEsc, tea.KeyEsc)
	if m.scope() != editKeys || m.editor.Value() != want {
		t.Fatal("closing the template editor did not restore the original focus")
	}
}

func TestSavedQueryLiteralEscapingIsOptIn(t *testing.T) {
	m := New(Options{SavedQueriesPath: savedFixture(t)}, nil)
	path := m.opt.SavedQueriesPath
	const raw = `queries:
  - name: Legacy
    expression: 'up{job="${raw}/$${literal}"}'
  - name: Escaping only
    expression: 'up{job="$${literal}"}'
    parameters: []
`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	savedCtrl(m, 'o')
	press(m, tea.KeyEnter)
	if m.editor.Value() != `up{job="${raw}/$${literal}"}` || m.parameterDialog != nil {
		t.Fatalf("legacy text was interpreted: %q", m.editor.Value())
	}
	savedCtrl(m, 'o')
	press(m, tea.KeyDown, 'e')
	savedCtrl(m, 'u')
	savedPaste(m, "Escape")
	press(m, tea.KeyEnter, tea.KeyEnter)
	if m.editor.Value() != `up{job="${literal}"}` || m.parameterDialog != nil {
		t.Fatalf("an escaped query did not load directly: %q", m.editor.Value())
	}
}
