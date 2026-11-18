package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/prometheus/common/model"
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
	m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if m.quitting || !m.editor.Focused() {
		t.Fatal("idle Ctrl+C changed state")
	}
	press(m, tea.KeyF5)
	if !m.editor.Focused() || m.cancel == nil {
		t.Fatal("F5 did not start a query in the editor")
	}
	m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
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
		queryActive := strings.Contains(screen, "› Query ━")
		resultsActive := strings.Contains(screen, "› Results · "+view+" ━")
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
	press(m, 'g', tea.KeyEnd, tea.KeyEnter, tea.KeyRight)
	if m.table.X == 0 {
		t.Fatal("expected a horizontally scrolled table")
	}
	table := m.table.View(m.width, m.page())
	for range 2 {
		press(m, tea.KeyTab)
		assertFocus(true, "Table")
		m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
		assertFocus(false, "Table")
		if m.chart.enabled || m.table.View(m.width, m.page()) != table {
			t.Fatal("focus navigation changed the table view or scroll position")
		}
	}
}

func TestRangeSelectorApplyAndCancel(t *testing.T) {
	m := New(Options{Window: time.Hour}, nil)
	m.editor.SetValue("up")
	m.Update(completed{result: sampleResult(false)})
	press(m, tea.KeyTab, 'r', tea.KeyDown, tea.KeyEsc)
	if m.opt.Window != time.Hour || m.opt.Ranged || m.editor.Focused() {
		t.Fatal("cancel changed query settings or focus")
	}
	press(m, 'r', tea.KeyDown, tea.KeyEnter)
	if m.opt.Window != 6*time.Hour || !m.opt.Ranged || m.panel != 0 || m.cancel != nil || len(m.table.Rows) != 2 || m.editor.Value() != "up" {
		t.Fatal("range apply did not independently update query settings")
	}
	press(m, 't')
	if m.opt.Ranged {
		t.Fatal("mode did not toggle")
	}
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
	if s := ansi.Strip(m.footer()); !strings.Contains(s, "Tab Query   g Table   s Series   r Range   q Quit") {
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
	press(m, 'a', tea.KeyEsc, 'g', tea.KeyEnd, tea.KeyEnter)
	if !m.chart.visible[1] || m.chart.enabled || m.cancel != nil || !m.table.Expanded || !strings.Contains(ansi.Strip(m.screen()), `Labels: cpu{host="beta"}`) {
		t.Fatal(m.screen())
	}
	press(m, tea.KeyHome)
	if m.table.Row != 0 || m.table.Expanded {
		t.Fatal("could not return to the first row")
	}
	press(m, 'g')
	if !m.chart.enabled || m.chart.selected != 0 {
		t.Fatal("table toggle lost graph state")
	}
	m.resize(20, 9)
	press(m, tea.KeySpace)
	if !m.chart.visible[0] {
		t.Fatal("Space toggled a series while its selection was not visible")
	}
}

func TestFooterAvailability(t *testing.T) {
	m := New(Options{}, nil)
	m.resize(110, 24)
	foot := func() string { return ansi.Strip(m.footer()) }
	if s := foot(); !strings.Contains(s, "F5 Run") || !strings.Contains(s, "Tab Results") || !strings.Contains(s, "F1 Help") || strings.Contains(s, "Graph") {
		t.Fatal(s)
	}
	press(m, tea.KeyTab)
	if s := foot(); !strings.Contains(s, "Tab Query   r Range   q Quit") || strings.Contains(s, "Copy") || strings.Contains(s, "Graph") {
		t.Fatal(s)
	}
	press(m, 'r')
	if s := foot(); !strings.Contains(s, "Enter Apply") || !strings.Contains(s, "Esc Back") || strings.Contains(s, "Tab Query") {
		t.Fatal(s)
	}
	press(m, tea.KeyEsc)
	m.resize(minWidth, minHeight)
	if s := foot(); !strings.Contains(s, "Tab Query") {
		t.Fatal(s)
	}
	press(m, tea.KeyTab)
	if s := foot(); !strings.Contains(s, "Tab Results") {
		t.Fatal(s)
	}
}
