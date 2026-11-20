package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/prometheus/common/model"
	"promtui/internal/query"
)

func TestResizeOverlayAndSwitchDuringGraphUpload(t *testing.T) {
	m := New(Options{Graphics: true}, nil)
	r := query.Result{Start: time.Unix(0, 0), End: time.Unix(3, 0), Step: time.Second, Series: []query.Series{{Name: "cpu", Samples: []model.SamplePair{{Timestamp: 1000, Value: 1}}}}}
	_, render := m.Update(completed{result: r})
	f := render().(graphFrame)
	if f.err != nil || f.key.row != 7 || f.key.col != 2 || f.key.cols != 78 || f.key.rows != 14 {
		t.Fatal(f.key, f.err)
	}
	_, upload := m.Update(f)
	// Switching before the raw message arrives must still delete the frame.
	m.Update(completed{result: query.Result{Series: r.Series}})
	_, clear := m.Update(upload())
	if clear == nil || !strings.Contains(string(clear().(tea.RawMsg).Msg.(graphPacket)), "a=d") {
		t.Fatal("missing cleanup")
	}
	m.Update(clear())
	_, render = m.Update(completed{result: r})
	_, next := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if next != nil {
		t.Fatal("concurrent render")
	}
	_, next = m.Update(render())
	newFrame := next().(graphFrame)
	if newFrame.err != nil || newFrame.key.cols != 98 || newFrame.key.rows != 20 {
		t.Fatal(newFrame.key, newFrame.err)
	}
	press(m, '?')
	_, cmd := m.Update(newFrame)
	if cmd != nil || m.chart.displayed.cols != 0 {
		t.Fatal("stale graph covered help")
	}
}

func TestDialogsKeepGraphPlacementDuringUploadAndResize(t *testing.T) {
	m := New(Options{Graphics: true}, nil)
	_, render := m.Update(completed{result: sampleResult(true)})
	_, upload := m.Update(render())
	press(m, tea.KeyDown, 's')
	initial := m.chart.displayed
	if _, cmd := m.Update(upload()); cmd != nil || m.graphPlace() != initial || m.chart.selected != 1 {
		t.Fatal("opening the selector disturbed the in-flight placement or selection")
	}
	_, render = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !m.chart.visible[0] || m.chart.visible[1] || render == nil {
		t.Fatal("Space did not update exactly one series and schedule a redraw")
	}
	f := render().(graphFrame)
	initial.revision++
	if f.err != nil || f.key != initial {
		t.Fatal("updated plot moved or resized", f.key, f.err)
	}
	_, upload = m.Update(f)
	m.Update(upload())
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc}); cmd != nil || m.graphPlace() != initial {
		t.Fatal("closing the selector requested an image change")
	}
	checkDialogs := func() {
		t.Helper()
		initial, w, selected := m.graphPlace(), m.workspace(), m.chart.selected
		for _, key := range []rune{'r', 's', 'l'} {
			_, cmd := m.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
			d := m.dialogBounds(w)
			if cmd != nil || m.dialogLayer(w) == nil || m.workspace() != w ||
				m.graphPlace() != initial || m.chart.displayed != initial || m.chart.selected != selected ||
				d.y >= initial.row-1+initial.rows || d.y+d.height <= initial.row-1 {
				t.Fatalf("%c disturbed the plot or did not overlay it: %+v vs %+v", key, d, initial)
			}
			if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc}); cmd != nil || m.graphPlace() != initial || m.scope() != graphKeys {
				t.Fatalf("closing %c disturbed the plot or Results focus", key)
			}
		}
	}
	checkDialogs()
	press(m, 'r')
	_, clear := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	_, render = m.Update(clear())
	f = render().(graphFrame)
	if f.err != nil || f.key.cols != 98 || f.key.rows != 20 || f.key.row != initial.row || f.key.col != initial.col {
		t.Fatal("resizing under the dialog did not use the full graph viewport", f.key, f.err)
	}
	_, upload = m.Update(f)
	m.Update(upload())
	press(m, tea.KeyEsc)
	checkDialogs()
}
