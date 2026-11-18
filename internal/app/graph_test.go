package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/prometheus/common/model"
	"promtui/internal/query"
	"strings"
	"testing"
	"time"
)

func TestResizeOverlayAndSwitchDuringGraphUpload(t *testing.T) {
	m := New(Options{Graphics: true}, nil)
	r := query.Result{Start: time.Unix(0, 0), End: time.Unix(3, 0), Step: time.Second, Series: []query.Series{{Name: "cpu", Samples: []model.SamplePair{{Timestamp: 1000, Value: 1}}}}}
	_, render := m.Update(completed{result: r})
	f := render().(graphFrame)
	if f.err != nil || f.key.row != 5 || f.key.rows != 17 {
		t.Fatal(f.key, f.err)
	}
	_, upload := m.Update(f)
	// Switching before the raw message arrives must still delete the frame.
	m.Update(tea.KeyPressMsg{Code: tea.KeyF4})
	_, clear := m.Update(upload())
	if clear == nil || !strings.Contains(string(clear().(tea.RawMsg).Msg.(graphPacket)), "a=d") {
		t.Fatal("missing cleanup")
	}
	m.Update(clear())
	_, render = m.Update(tea.KeyPressMsg{Code: tea.KeyF4})
	_, next := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if next != nil {
		t.Fatal("concurrent render")
	}
	_, next = m.Update(render())
	newFrame := next().(graphFrame)
	if newFrame.err != nil || newFrame.key.cols != 100 || newFrame.key.rows != 23 {
		t.Fatal(newFrame.key, newFrame.err)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyF1})
	_, cmd := m.Update(newFrame)
	if cmd != nil || m.chart.displayed.cols != 0 {
		t.Fatal("stale graph covered help")
	}
}
