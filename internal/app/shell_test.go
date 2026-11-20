package app

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/config"
	"promtui/internal/editor"
)

func pressAlt(m *Model, code rune) tea.Cmd {
	_, cmd := m.Update(tea.KeyPressMsg{Code: code, Mod: tea.ModAlt, Text: string(code)})
	return cmd
}

func TestShellRoutesDigitsAroundTextInputs(t *testing.T) {
	m := New(Options{}, nil)
	press(m, '0', '1', '2')
	if m.activeTab != promQLTab || m.editor.Value() != "012" {
		t.Fatal("bare digits did not reach the editor")
	}
	m.popup = true
	press(m, '2')
	if m.activeTab != promQLTab || m.editor.Value() != "0122" {
		t.Fatal("bare digit did not reach the editor during completion")
	}
	pressAlt(m, '1')
	if m.activeTab != alertsTab || m.View().Cursor != nil {
		t.Fatal("Alt+1 did not switch away from the editor")
	}
	press(m, tea.KeyEsc, tea.KeyTab, tea.KeyF5, 'i')
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m.Update(tea.PasteMsg{Content: "discard this"})
	if m.activeTab != alertsTab || m.cancel != nil {
		t.Fatal("placeholder routed input to PromQL")
	}
	press(m, '2', '0')
	if m.activeTab != promQLTab || m.editor.Value() != "0122" || m.View().Cursor == nil {
		t.Fatal("editor focus or contents were lost")
	}
	m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	press(m, '1')
	if m.activeTab != promQLTab || m.searchText != "1" {
		t.Fatal("history search did not receive the digit")
	}
	pressAlt(m, '2')
	pressAlt(m, '0')
	if m.scope() != historyKeys || m.searchText != "1" {
		t.Fatal("history overlay state was lost")
	}
	press(m, tea.KeyEsc, tea.KeyTab)
	if m.scope() != tableKeys || m.activeTab != promQLTab {
		t.Fatal("Tab did not focus Results")
	}
	m.panel, m.filtering = seriesKeys, true
	press(m, '1')
	if m.activeTab != promQLTab || m.seriesFilter != "1" {
		t.Fatal("series filter did not receive the digit")
	}
	press(m, tea.KeyEsc, '1', '0')
	if m.activeTab != promQLTab || m.scope() != tableKeys {
		t.Fatal("bare tab keys did not restore Results focus")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.activeTab != promQLTab || m.scope() != editKeys {
		t.Fatal("Shift+Tab did not restore editor focus")
	}
}

func TestShellLayoutAndFooterOnResize(t *testing.T) {
	m := New(Options{Connection: config.Connection{Name: "q1q1"}, Ranged: true, Window: time.Hour}, nil)
	check := func(size [2]int, want ...string) {
		t.Helper()
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		content := ansi.Strip(m.screen())
		rows := strings.Split(content, "\n")
		if len(rows) != size[1] || rows[len(rows)-1] != ansi.Strip(m.footer()) {
			t.Fatalf("footer not at bottom at %v: %q", size, content)
		}
		for _, row := range rows {
			if ansi.StringWidth(row) > size[0] {
				t.Fatalf("row too wide at %v: %q", size, row)
			}
		}
		for _, text := range want {
			if !strings.Contains(content, text) {
				t.Errorf("no %q at %v:\n%s", text, size, content)
			}
		}
	}
	check([2]int{100, 24}, "q1q1", "0 PromQL", "1 Alerts", "2 Targets", "Range · Last 1h", runHint)
	pressAlt(m, '1')
	check([2]int{100, 24}, "╭─ › Alerts", "Active Alerts", "Connection: q1q1", "0–2 Tabs", "q Quit")
	if !strings.Contains(m.screen(), activeTabStyle.Render(" 1 Alerts ")) ||
		strings.Contains(m.footer(), "Run") || strings.Contains(m.footer(), "History") {
		t.Fatal("Alerts is not highlighted or displays PromQL actions")
	}
	press(m, '2')
	check([2]int{24, 10}, "╭─ › Targets", "Connection: q1q1", "Scrape Targets", "in a future", "milestone.", "0–2 Tabs")
	check([2]int{19, 6}, "Enlarge terminal", "0–2 Tabs")
	press(m, '0')
	check([2]int{70, 18}, "╭─ › Query", "Tab Results")
	if m.View().Cursor == nil {
		t.Fatal("Query did not resume after resizing on Targets")
	}
}

func TestTabSwitchDoesNotRequeryOrChangeTableState(t *testing.T) {
	m := New(Options{Timeout: time.Second, Ranged: true, Window: 6 * time.Hour}, nil)
	const expr = "sum(\n  up\n)"
	m.editor.SetValue(expr)
	m.editor.CursorUp()
	m.editor.SetCursorColumn(3)
	pos := editor.Position(m.editor)
	result := sampleResult(false)
	m.Update(completed{result: result})
	press(m, tea.KeyTab, tea.KeyDown, tea.KeyEnter)
	table, id := m.table, m.id
	for _, code := range "120120" {
		if _, cmd := m.Update(tea.KeyPressMsg{Code: code, Text: string(code)}); cmd != nil {
			t.Fatalf("switching with %c scheduled work", code)
		}
	}
	if m.scope() != tableKeys || m.id != id || m.cancel != nil || !reflect.DeepEqual(m.table, table) ||
		!m.opt.Ranged || m.opt.Window != 6*time.Hour || m.editor.Value() != expr || editor.Position(m.editor) != pos {
		t.Fatal("switching tabs did not preserve the table, range, or editor state")
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	m.cancel = cancel
	press(m, '2')
	if _, cmd := m.Update(completed{id: id, result: result}); ctx.Err() != nil || cmd != nil || m.activeTab != targetsTab {
		t.Fatal("switching cancelled or restarted the in-flight query")
	}
	press(m, '0')
	if len(m.table.Rows) != len(result.Series) || m.cancel != nil {
		t.Fatal("completion while hidden was lost")
	}
}

func TestShellClearsAndRestoresKittyPlacement(t *testing.T) {
	m := New(Options{Graphics: true, Ranged: true, Window: time.Hour}, nil)
	_, render := m.Update(completed{result: sampleResult(true)})
	press(m, tea.KeyTab, tea.KeyDown, tea.KeySpace)
	_, render = m.Update(render()) // discard initial render invalidated by visibility change
	_, upload := m.Update(render())

	// Switch away with an upload already queued, then pick another placeholder.
	press(m, '1', '2')
	_, clear := m.Update(upload())
	if clear == nil || !strings.Contains(string(clear().(tea.RawMsg).Msg.(graphPacket)), "a=d") ||
		strings.Contains(m.screen(), "Scrape Targets") {
		t.Fatal("graph was not cleared before showing the destination")
	}
	if _, cmd := m.Update(clear()); cmd != nil || !strings.Contains(m.screen(), "Scrape Targets") {
		t.Fatal("placeholder did not become visible after graph cleanup")
	}
	if _, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30}); cmd != nil {
		t.Fatal("inactive tab attempted to render a graph")
	}
	for round := range 3 {
		_, render = m.Update(tea.KeyPressMsg{Code: '0', Text: "0"})
		f := render().(graphFrame)
		if f.err != nil || f.key.cols != 98 || f.key.rows != 20 || m.scope() != graphKeys ||
			!slices.Equal(m.chart.visible, []bool{true, false}) || m.chart.selected != 1 ||
			m.id != 0 || m.cancel != nil {
			t.Fatalf("round %d: graph did not resume with current size and original state: %+v", round, f.key)
		}
		_, upload = m.Update(f)
		m.Update(upload())
		_, clear = m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
		m.Update(clear())
		if m.chart.sending || m.chart.displayed.cols != 0 || m.chart.renderCancel != nil {
			t.Fatalf("round %d: lingering graphics work or placement", round)
		}
	}
}

func TestShellDiscardsGraphRenderedAfterLeavingPromQL(t *testing.T) {
	m := New(Options{Graphics: true}, nil)
	_, render := m.Update(completed{result: sampleResult(true)})
	frame := render() // The render finished, but its message has not yet arrived.
	if cmd := pressAlt(m, '1'); cmd != nil || m.graphPlace().cols != 0 {
		t.Fatal("leaving with a pending render scheduled unexpected graphics work")
	}
	if _, cmd := m.Update(frame); cmd != nil || m.chart.displayed.cols != 0 || m.chart.renderCancel != nil {
		t.Fatal("late render was uploaded or left graphics work pending")
	}
	if cmd := pressAlt(m, '0'); cmd == nil {
		t.Fatal("returning did not schedule a fresh render")
	}
	m.CloseGraphics()
}
