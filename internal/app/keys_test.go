package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/prometheus/common/model"
	"promtui/internal/config"
	"promtui/internal/editor"
	"promtui/internal/query"
)

func press(m *Model, codes ...rune) {
	for _, code := range codes {
		key := tea.KeyPressMsg{Code: code}
		if code >= ' ' && code <= '~' {
			key.Text = string(code)
		}
		m.Update(key)
	}
}

func TestEditorAndResultsFocus(t *testing.T) {
	m := New(Options{}, nil)
	press(m, 'q', 'g', 's', 'r', 't', '?', tea.KeyEnter, 'u', 'p')
	if got := m.editor.Value(); got != "qgsrt?\nup" || m.quitting {
		t.Fatal(got)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	if m.editor.Focused() || m.cancel == nil {
		t.Fatal("Ctrl+Enter did not start a query and focus Results")
	}
	press(m, 'i')
	if !m.editor.Focused() || m.editor.Value() != "qgsrt?\nup" {
		t.Fatal("i did not return to the editor without inserting text")
	}
	m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if m.quitting || !m.editor.Focused() || m.cancel != nil || m.status != "Cancelled" {
		t.Fatal("Ctrl+C did not cancel the active query and preserve editor focus")
	}
	m.popup = true
	press(m, tea.KeyEsc)
	if m.popup || !m.editor.Focused() {
		t.Fatal("Esc should close completion first")
	}
	press(m, tea.KeyEsc, tea.KeyEsc)
	if !m.editor.Focused() {
		t.Fatal("Esc must not leave Query")
	}
	press(m, tea.KeyTab, '?')
	if m.editor.Focused() || m.panel != helpKeys {
		t.Fatal("Tab / ? did not open Results help")
	}
	press(m, 'i', tea.KeyTab)
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.panel != helpKeys || m.editor.Focused() {
		t.Fatal("panel navigation leaked through help")
	}
	press(m, tea.KeyEsc, tea.KeyEsc, tea.KeyF5)
	if m.panel != 0 || m.editor.Focused() || m.cancel == nil {
		t.Fatal("closing help or running changed Results focus")
	}
	m.Update(completed{id: m.id, result: sampleResult(false)})
	if m.editor.Focused() {
		t.Fatal("query completion changed Results focus")
	}
	press(m, 'i')
	if !m.editor.Focused() {
		t.Fatal("i did not restore editing")
	}
	press(m, tea.KeyTab, tea.KeyF5)
	press(m, 'q')
	if !m.quitting || m.cancel != nil {
		t.Fatal("Results q should quit and cancel the run")
	}
}

func TestRunDoesNotStealFocusAfterReturningToQuery(t *testing.T) {
	m := New(Options{}, nil)
	run := tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl}
	m.Update(run)
	if !m.editor.Focused() || m.cancel != nil {
		t.Fatal("empty query left the editor")
	}
	m.editor.SetValue("up")
	m.editor.SetCursorColumn(1)
	m.popup = true
	m.Update(run)
	if m.popup || m.editor.Focused() || m.cancel == nil {
		t.Fatal("running with autocomplete open did not focus Results")
	}
	press(m, 'i')
	m.Update(completed{id: m.id, result: sampleResult(false)})
	if !m.editor.Focused() || m.editor.Column() != 1 || m.editor.Value() != "up" {
		t.Fatal("query completion disturbed the editor or its cursor")
	}
}

func TestCtrlCQuitsWhenIdle(t *testing.T) {
	for _, scope := range []keyScope{editKeys, tableKeys, helpKeys} {
		m := New(Options{}, nil)
		m.editor.SetValue("up")
		if scope != editKeys {
			press(m, tea.KeyTab)
		}
		if scope == helpKeys {
			press(m, tea.KeyF1)
		}
		_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		if !m.quitting || cmd == nil || m.editor.Value() != "up" {
			t.Fatalf("idle Ctrl+C did not quit cleanly from %v", scope)
		}
	}
}

func TestTabNavigationPreservesPanelState(t *testing.T) {
	m := New(Options{Graphics: true}, nil)
	const expr = "sum(\n  up\n)"
	m.editor.SetValue(expr)
	m.editor.CursorUp()
	m.editor.SetCursorColumn(2)
	pos := editor.Position(m.editor)
	result := sampleResult(true)
	result.Series[1].Labels = "cpu{host=\"" + strings.Repeat("beta", 40) + "\"}"
	m.Update(completed{result: result})
	if !m.editor.Focused() {
		t.Fatal("query completion changed Query focus")
	}
	assertFocus := func(queryFocused bool, view string) {
		t.Helper()
		screen := ansi.Strip(m.screen())
		queryActive := strings.Contains(screen, "╭─ › Query ─")
		resultsActive := strings.Contains(screen, "╭─ › Results · "+view+" ─")
		if m.editor.Focused() != queryFocused || queryActive != queryFocused || resultsActive == queryFocused {
			t.Fatal(screen)
		}
		if m.editor.Value() != expr || editor.Position(m.editor) != pos {
			t.Fatal("focus navigation changed the query or cursor")
		}
	}
	assertFocus(true, "Graph")
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	assertFocus(false, "Graph")
	press(m, tea.KeyDown, tea.KeyTab)
	assertFocus(true, "Graph")
	press(m, tea.KeyTab)
	assertFocus(false, "Graph")
	if !m.chart.enabled || m.chart.selected != 1 {
		t.Fatal("focus navigation lost the graph selection")
	}
	press(m, 't')
	m.Update(completed{id: m.id, result: query.Result{Series: result.Series}})
	press(m, tea.KeyEnd, tea.KeyEnter, tea.KeyRight)
	if m.table.X == 0 {
		t.Fatal("expected a horizontally scrolled table")
	}
	table := m.table.View(m.width-2, m.page())
	for range 2 {
		press(m, tea.KeyTab)
		assertFocus(true, "Table")
		m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
		assertFocus(false, "Table")
		if m.chart.enabled || m.table.View(m.width-2, m.page()) != table {
			t.Fatal("focus navigation changed the table view or scroll position")
		}
	}
}

func TestResultsShortcutsRunQueries(t *testing.T) {
	type request struct {
		path, query, start, end string
	}
	requests := make(chan request, 5)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- request{r.URL.Path, r.FormValue("query"), r.FormValue("start"), r.FormValue("end")}
		if r.URL.Path == "/api/v1/query_range" {
			fmt.Fprint(w, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"__name__":"up"},"values":[[1000,"1"]]}]}}`)
		} else {
			fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"__name__":"up"},"value":[1000,"1"]}]}}`)
		}
	}))
	defer server.Close()
	end := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	m := New(Options{Graphics: true, Window: time.Hour, End: end, Timeout: time.Second, Limit: 10,
		Connection: config.Connection{URL: server.URL}}, nil)
	m.editor.SetValue("up")
	run := func(key rune, window time.Duration) {
		t.Helper()
		// Use the app update and execute only the request, not the separate Kitty command.
		_, cmd := m.update(tea.KeyPressMsg{Code: key})
		if cmd == nil || m.cancel == nil || m.editor.Focused() {
			t.Fatal("shortcut did not start a query in Results")
		}
		result := cmd().(completed)
		if result.err != nil {
			t.Fatal(result.err)
		}
		got := <-requests
		want := request{path: "/api/v1/query", query: "up"}
		if window > 0 {
			want.path = "/api/v1/query_range"
			want.start, want.end = end.Add(-window).Format(time.RFC3339), end.Format(time.RFC3339)
		}
		if got != want {
			t.Fatalf("request = %+v; want %+v", got, want)
		}
		m.Update(result)
		if m.chart.enabled != (window > 0) || m.panel != 0 || m.editor.Focused() {
			t.Fatal("query did not restore the intended result mode and focus")
		}
	}
	press(m, tea.KeyTab, 'r', tea.KeyDown, tea.KeyEsc)
	if m.opt.Window != time.Hour || m.opt.Ranged || m.editor.Focused() || m.cancel != nil {
		t.Fatal("cancel changed query settings or focus")
	}
	run('g', time.Hour)
	run('t', 0)
	run('g', time.Hour)
	press(m, 'r', tea.KeyDown)
	run(tea.KeyEnter, 6*time.Hour)
}

func TestModeShortcutSupersedesInFlightRequest(t *testing.T) {
	m := New(Options{Graphics: true, Timeout: time.Second}, nil)
	m.editor.SetValue("up")
	press(m, tea.KeyTab, 'g')
	oldID := m.id
	if m.cancel == nil {
		t.Fatal("graph shortcut did not start a request")
	}
	press(m, 't')
	m.Update(completed{id: oldID, result: sampleResult(true)})
	if m.id == oldID || m.cancel == nil || m.opt.Ranged || m.chart.enabled || m.status != "Running…" {
		t.Fatal("the old range response superseded the newer table request")
	}
	m.Update(completed{id: m.id, result: sampleResult(false)})
}

func sampleResult(ranged bool) query.Result {
	r := query.Result{Series: []query.Series{
		{Name: "cpu", Labels: `cpu{host="alpha"}`, Samples: []model.SamplePair{{Timestamp: 1000, Value: 1}}},
		{Name: "cpu", Labels: `cpu{host="beta"}`, Samples: []model.SamplePair{{Timestamp: 1000, Value: 2}}},
	}}
	if ranged {
		r.Start, r.End, r.Step = time.Unix(0, 0), time.Unix(3, 0), time.Second
	}
	return r
}

func TestGraphSeriesAndTableActions(t *testing.T) {
	m := New(Options{Graphics: true}, nil)
	m.Update(completed{result: sampleResult(true)})
	press(m, tea.KeyTab, tea.KeyDown)
	if s := ansi.Strip(m.footer()); !strings.Contains(s, "Tab Query   g Graph   t Table   s Series   r Range   q Quit") {
		t.Fatal(s)
	}
	if !strings.Contains(m.legend(), "› 2/2") {
		t.Fatal(m.legend())
	}
	press(m, tea.KeySpace, 'l')
	if m.chart.visible[1] || m.panel != labelKeys || !strings.Contains(m.labelText, "beta") {
		t.Fatal("graph actions did not target the selected series")
	}
	press(m, tea.KeyEsc, 's', '/')
	press(m, tea.KeyTab)
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.scope() != filterKeys || m.seriesFilter != "" || m.editor.Focused() {
		t.Fatal("Tab leaked through the series filter")
	}
	m.Update(tea.PasteMsg{Content: "ALPHA"})
	if len(m.filtered) != 1 || m.filtered[0] != 0 {
		t.Fatal(m.filtered)
	}
	press(m, tea.KeyEnter, tea.KeyEnter)
	if !m.chart.visible[0] || m.chart.visible[1] || m.panel != seriesKeys {
		t.Fatal("isolate did not keep only the matching series")
	}
	press(m, tea.KeyTab)
	if m.scope() != seriesKeys || m.editor.Focused() {
		t.Fatal("Tab leaked through the series selector")
	}
	press(m, '/')
	m.Update(tea.PasteMsg{Content: "draft"})
	press(m, tea.KeyEsc)
	if m.panel != 0 || m.filtering || m.seriesFilter != "ALPHA" || m.editor.Focused() {
		t.Fatal("Esc did not cancel the filter and restore Results focus")
	}
	press(m, 's')
	press(m, 'a', tea.KeyEsc)
	if !m.chart.visible[1] {
		t.Fatal("a did not show all series")
	}
	m.editor.SetValue("up")
	press(m, 't')
	m.Update(completed{id: m.id, result: sampleResult(false)})
	press(m, tea.KeyEnd, tea.KeyEnter)
	if m.chart.enabled || !m.table.Expanded || !strings.Contains(ansi.Strip(m.screen()), `Labels: cpu{host="beta"}`) {
		t.Fatal(m.screen())
	}
	press(m, tea.KeyHome)
	if m.table.Row != 0 || m.table.Expanded {
		t.Fatal("could not return to the first row")
	}
	press(m, 'g')
	m.Update(completed{id: m.id, result: sampleResult(true)})
	if !m.chart.enabled || m.chart.selected != 0 {
		t.Fatal("table toggle lost graph state")
	}
	m.resize(20, 9)
	press(m, tea.KeySpace)
	if !m.chart.visible[0] {
		t.Fatal("Space toggled a series while its selection was not visible")
	}
}

func TestSeriesEnterRestoresPreviousVisibility(t *testing.T) {
	m := New(Options{Graphics: true}, nil)
	m.Update(completed{result: sampleResult(true)})
	press(m, tea.KeyTab, 's', tea.KeyDown, tea.KeyEnter)
	if m.chart.visible[0] || !m.chart.visible[1] || !strings.Contains(ansi.Strip(m.footer()), "Enter Restore") {
		t.Fatal("Enter did not isolate the selected series")
	}
	press(m, tea.KeyEnter)
	if !m.chart.visible[0] || !m.chart.visible[1] || m.panel != seriesKeys || m.chart.selected != 1 {
		t.Fatal("Enter did not restore previous visibility")
	}
}

func TestFooterAvailability(t *testing.T) {
	m := New(Options{}, nil)
	m.resize(110, 24)
	foot := func() string { return ansi.Strip(m.footer()) }
	if s := foot(); !strings.Contains(s, "Ctrl+Enter Run") || !strings.Contains(s, "Tab Results") || !strings.Contains(s, "F1 Help") || strings.Contains(s, "Graph") {
		t.Fatal(s)
	}
	press(m, tea.KeyTab)
	if s := foot(); !strings.Contains(s, "Tab Query   t Table   r Range   q Quit   i Edit") || strings.Contains(s, "Copy") || strings.Contains(s, "Graph") {
		t.Fatal(s)
	}
	press(m, 'r')
	if s := foot(); !strings.Contains(s, "Enter Run") || !strings.Contains(s, "Esc Back") || strings.Contains(s, "Tab Query") {
		t.Fatal(s)
	}
	press(m, tea.KeyEsc)
	m.resize(minWidth, minHeight)
	if s := foot(); !strings.Contains(s, "Tab Query") {
		t.Fatal(s)
	}
	press(m, tea.KeyTab)
	if s := foot(); !strings.Contains(s, "^Enter Run  Tab Results") {
		t.Fatal(s)
	}
}
