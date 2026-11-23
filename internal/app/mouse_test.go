package app

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/prometheus/common/model"
	"promtui/internal/config"
	"promtui/internal/query"
)

func clickAt(x, y int) tea.MouseClickMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func wheelDownAt(x, y int) tea.MouseWheelMsg {
	return tea.MouseWheelMsg{X: x, Y: y, Button: tea.MouseWheelDown}
}

func columnOf(rendered, label string) int {
	before, _, ok := strings.Cut(ansi.Strip(rendered), label)
	if !ok {
		return -1
	}
	return ansi.StringWidth(before)
}

func TestMouseTabsOnResizeAndDisabled(t *testing.T) {
	m := New(Options{Mouse: true, Connection: config.Connection{Name: "測試"}}, nil)
	t0 := time.Now()
	for _, width := range []int{110, 42, minWidth} {
		m.resize(width, 24)
		view := m.View()
		header, _, _ := strings.Cut(view.Content, "\n")
		if view.MouseMode != tea.MouseModeCellMotion {
			t.Fatal("mouse not enabled")
		}
		x := columnOf(header, "[3]")
		if x < 0 {
			t.Fatal("Targets tab not visible")
		}
		m.handleMouse(clickAt(x, 0), t0)
		if m.activeTab != targetsTab {
			t.Errorf("Targets tab not clickable at width %d: %q", width, header)
		}
		press(m, '0')
	}
	m.opt.Mouse = false
	m.resize(80, 22)
	view := m.View()
	if view.MouseMode != tea.MouseModeNone || len(m.mouse.hits) != 0 {
		t.Fatal("disabled mouse still captured")
	}
	m.Update(clickAt(26, 0))
	if m.activeTab != promQLTab {
		t.Fatal("disabled mouse switched tabs")
	}
	press(m, 'i', '3')
	if m.activeTab != promQLTab || m.editor.Value() != "3" {
		t.Fatal("keyboard tab shortcut swallowed query input")
	}
	m.Update(tea.KeyPressMsg{Code: '3', Mod: tea.ModAlt})
	if m.activeTab != targetsTab {
		t.Fatal("Alt+3 no longer switches tabs from the editor")
	}
}

func mouseTargetFixture() *Model {
	m := New(Options{Mouse: true, Connection: config.Connection{URL: "http://example.org"}}, nil)
	m.activeTab = targetsTab
	for _, name := range []string{"first", "second", "third", "fourth", "fifth", "sixth", "seventh"} {
		m.targets.visible = append(m.targets.visible, &targetRow{id: name, instance: name, health: targetUp})
	}
	m.resize(80, 12)
	return m
}

func TestMouseRowsDoubleClickScrollRefreshAndModal(t *testing.T) {
	m := mouseTargetFixture()
	a := &m.targets
	t0 := time.Now()
	click := func(x, y, millis int) {
		m.View()
		m.handleMouse(clickAt(x, y), t0.Add(time.Duration(millis)*time.Millisecond))
	}
	click(5, 4, 0) // table heading
	click(5, 6, 1) // second row
	if a.row != 1 || a.dialog != noTargetDialog {
		t.Fatal("first click didn't just select the second row")
	}
	click(5, 7, 20)
	if a.row != 2 || a.dialog != noTargetDialog {
		t.Fatal("clicks on different rows activated details")
	}
	a.visible[2], a.visible[3] = a.visible[3], a.visible[2] // refresh/reorder
	click(5, 7, 40)
	if a.dialog != noTargetDialog {
		t.Fatal("different item at old coordinates was treated as a double-click")
	}
	m.View()
	m.handleMouse(wheelDownAt(5, 7), t0.Add(50*time.Millisecond))
	if a.row != 3 {
		t.Fatal("wheel didn't select the next target")
	}
	click(5, 7, 60) // now points to row 3 after the viewport moved
	if a.dialog != noTargetDialog {
		t.Fatal("wheel did not clear pending double-click")
	}
	click(5, 7, 450)
	if a.dialog != noTargetDialog {
		t.Fatal("two slow clicks opened details")
	}
	selected := a.selection()
	click(5, 7, 500)
	if a.dialog != targetDetails || a.detail.id != selected {
		t.Fatalf("double-click didn't open the selected target: %q", a.detail.id)
	}
	header, _, _ := strings.Cut(m.View().Content, "\n")
	x := columnOf(header, "[0]")
	click(x, 0, 520)
	click(3, 6, 540)
	if m.activeTab != targetsTab || a.selection() != selected || a.dialog != targetDetails {
		t.Fatal("click escaped modal")
	}
	_, _, _, room := m.targetTextDialog()
	y := (m.height-room-3)/2 + 1
	m.View()
	m.handleMouse(wheelDownAt(m.width/2, y), t0.Add(550*time.Millisecond))
	if a.dialogRow != 1 || a.selection() != selected {
		t.Fatal("wheel did not scroll only the modal")
	}
	press(m, tea.KeyEsc, tea.KeyUp, tea.KeyEnter)
	if a.dialog != targetDetails || a.detail.id == selected {
		t.Fatal("keyboard selection and Enter no longer open details")
	}
}

func TestMouseTargetSourceOverlay(t *testing.T) {
	m := mouseTargetFixture()
	press(m, 'f')
	a := &m.targets
	width, _ := m.targetDialogSize()
	x, y := (m.width-width)/2+2, (m.height-2-3)/2+2
	t0 := time.Now()
	m.View()
	m.handleMouse(clickAt(x, y), t0)
	if a.dialogRow != 1 || a.source != 0 {
		t.Fatal("source click should highlight but not apply")
	}
	press(m, tea.KeyEnter)
	if a.source != 1 || a.dialog != noTargetDialog {
		t.Fatal("Enter didn't apply the clicked source")
	}
}

func TestMousePromQLTableDetails(t *testing.T) {
	m := New(Options{Mouse: true}, nil)
	m.queryPhase = querySucceeded
	for _, name := range []string{"first", "second", "third"} {
		m.table.Rows = append(m.table.Rows, query.Series{Name: name, Labels: name, Samples: []model.SamplePair{{Value: 1}}})
	}
	m.table.Expanded = true
	y := m.workspace().bodyY
	t0 := time.Now()
	m.View()
	m.handleMouse(clickAt(5, y+2), t0) // Expanded label row, not a data row.
	if m.table.Row != 0 || !m.table.Expanded {
		t.Fatal("click on expanded labels selected a data row")
	}
	m.handleMouse(clickAt(5, y+4), t0.Add(time.Millisecond))
	if m.table.Row != 1 || m.table.Expanded || m.panel != 0 {
		t.Fatal("click after expanded labels did not select the second data row")
	}
	m.View()
	m.handleMouse(clickAt(5, y+2), t0.Add(100*time.Millisecond))
	if m.panel != labelKeys || m.labelText != "second" {
		t.Fatal("double-click on the same series did not show its labels")
	}
}

func TestMouseRoutingPanesAcrossLayoutChange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(routingHandler))
	defer server.Close()
	m := New(Options{Mouse: true, Connection: config.Connection{AlertmanagerURL: server.URL}}, nil)
	loadRouting(t, m, pressAlt(m, '4'))
	t0 := time.Now()
	m.resize(110, 18)
	m.View()
	m.handleMouse(clickAt(5, 4), t0)
	if m.routing.routes.selected != "0/0" || m.routing.dialog != routingNoDialog {
		t.Fatal("click did not select the child route")
	}
	m.resize(34, 14)
	m.View()
	m.handleMouse(clickAt(5, 4), t0.Add(time.Millisecond))
	if m.routing.dialog != routingNoDialog {
		t.Fatal("resize didn't clear pending double-click")
	}
	details := m.explorerLayout().detailBox()
	m.handleMouse(wheelDownAt(details.Min.X, details.Min.Y), t0.Add(50*time.Millisecond))
	if m.routing.routes.detailOffset == 0 || m.routing.routes.selected != "0/0" || m.routing.routes.details {
		t.Fatal("wheel over stacked details changed the list or keyboard focus")
	}
	m.View()
	m.handleMouse(clickAt(5, 4), t0.Add(100*time.Millisecond))
	m.View()
	m.handleMouse(clickAt(5, 4), t0.Add(200*time.Millisecond))
	if m.routing.dialog != routingReceiverDialog {
		t.Fatal("double-click didn't open the selected route's receiver details")
	}
}

func TestMouseCopiesWrappedTargetURL(t *testing.T) {
	m := mouseTargetFixture()
	m.resize(40, 18)
	value := "https://example.org/" + strings.Repeat("界", 50) + "?flag=1"
	m.targets.visible[0].ScrapeURL = value
	press(m, tea.KeyEnter)
	a := &m.targets
	_, content, _, room := m.targetTextDialog()
	first := 0
	for i, row := range content {
		if strings.Contains(row, ansi.SetHyperlink(value)) && strings.HasPrefix(ansi.Strip(row), "界") {
			first = i
			break
		}
	}
	if first == 0 {
		t.Fatal("expected wrapped link")
	}
	a.dialogRow = min(first, len(content)-room)
	view := m.View()
	if !strings.Contains(view.Content, ansi.SetHyperlink(value)) {
		t.Fatalf("composited overlay lost the OSC 8 hyperlink:\n%q", view.Content)
	}
	for _, hit := range m.mouse.hits {
		if hit.selectItem == nil {
			continue
		}
		// Only link runs should remain clickable after the modal blocks the background.
		p := hit.box.Min
		msg := clickAt(p.X, p.Y)
		msg.Mod = tea.ModShift
		if cmd := m.handleMouse(msg, time.Now()); cmd != nil || m.showURLFeedback {
			t.Fatal("modifier-click was intercepted")
		}
		if cmd := m.handleMouse(clickAt(p.X, p.Y), time.Now()); cmd == nil || !m.showURLFeedback {
			t.Fatal("left-click on wrapped link didn't request a clipboard write")
		}
		_, cmd := m.update(tea.KeyPressMsg{Code: 'u', Text: "u"})
		if cmd == nil || !m.showURLFeedback {
			t.Fatal("keyboard shortcut didn't request a clipboard write")
		}
		return
	}
	t.Fatal("no clickable link cells")
}

func TestMouseAlertGeneratorIsLinked(t *testing.T) {
	m := New(Options{Mouse: true}, nil)
	m.activeTab = alertsTab
	a := testAlert("id", "test", "warning", "active", time.Now())
	a.GeneratorURL = "https://example.org/graph?g0.expr=up"
	m.alerts.openAlertDetails(newAlertRow("local", a))
	m.alerts.offset = 1000
	view := m.View()
	if !strings.Contains(view.Content, ansi.SetHyperlink(a.GeneratorURL)) {
		t.Fatal("generator URL not linked")
	}
	_, cmd := m.update(tea.KeyPressMsg{Code: 'u', Text: "u"})
	if cmd == nil {
		t.Fatal("generator URL keyboard copy missing")
	}
}

func TestDetailURLClipboardWritesOriginalValue(t *testing.T) {
	m := mouseTargetFixture()
	value := "https://example.org/p?a=a%20b&x=2#frag"
	m.targets.visible[0].ScrapeURL = value
	press(m, tea.KeyEnter)
	var terminal bytes.Buffer
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	wantMessage := tea.SetClipboard(value)()
	sent := make(chan struct{}, 1)
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(&terminal),
		tea.WithWindowSize(80, 24), tea.WithFilter(func(_ tea.Model, msg tea.Msg) tea.Msg {
			if msg == wantMessage {
				sent <- struct{}{}
			}
			return msg
		}))
	done := make(chan error, 1)
	go func() {
		_, err := p.Run()
		done <- err
	}()
	p.Send(tea.KeyPressMsg{Code: 'u', Text: "u"})
	select {
	case <-sent:
		p.Send(tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl})
	case <-ctx.Done():
		<-done
		t.Fatal("no clipboard command with the original URL")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(terminal.String(), ansi.SetSystemClipboard(value)) {
		t.Fatal("Bubble Tea did not emit OSC 52 for the original URL")
	}
}
