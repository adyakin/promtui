package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"promtui/internal/config"
	"promtui/internal/programstatus"
)

func TestProgramStatusRapidQueries(t *testing.T) {
	m := New(Options{ProgramStatus: true, Timeout: time.Second}, nil)
	m.editor.SetValue("up")
	m.run()
	first := m.id
	working := m.flushProgramStatus(nil)()
	m.run()
	second := m.id
	if first == second {
		t.Fatal("replacement did not get its own execution ID")
	}
	m.Update(completed{id: first, err: errors.New("stale failure")})
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd != nil {
		t.Fatal("sent a packet before the in-flight packet was acknowledged")
	}
	m.Update(completed{id: second, err: context.Canceled})
	_, cmd = m.Update(working)
	want := programstatus.Report(programstatus.Working, "Executing PromQL query") + programstatus.Report(programstatus.Idle, "")
	if got := string(cmd().(tea.RawMsg).Msg.(statusPacket)); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if cleanup := m.CloseTerminal(); cleanup != programstatus.Clear() {
		t.Errorf("unexpected cleanup %q", cleanup)
	}
}

func TestNoProgramStatusForBackgroundOperations(t *testing.T) {
	m := New(Options{ProgramStatus: true}, nil)
	for _, msg := range []tea.Msg{tea.WindowSizeMsg{Width: 80, Height: 24}, saved{}, alertsCompleted{}, targetsCompleted{}, tea.KeyPressMsg{Code: tea.KeyF5}} {
		m.Update(msg)
		if s := m.programStatus; s.sending || s.pending != "" {
			t.Fatalf("%T reported status", msg)
		}
	}
	if m.CloseTerminal() != "" {
		t.Fatal("an unused record must not be cleared")
	}
	m = New(Options{}, nil)
	m.editor.SetValue("up")
	m.run()
	m.Update(completed{id: m.id})
	if m.programStatus != (terminalStatus{}) || m.CloseTerminal() != "" {
		t.Fatal("status reporting was disabled")
	}
}

func TestProgramStatusWithBubbleTeaAndKitty(t *testing.T) {
	start := time.Unix(1800000000, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.FormValue("query") != "up" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprint(w, `{"status":"error","errorType":"execution","error":"private backend detail"}`)
			return
		}
		fmt.Fprintf(w, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"__name__":"up"},"values":[[%d,"1"],[%d,"2"]]}]}}`, start.Unix(), start.Unix()+60)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var output bytes.Buffer
	events := make(chan tea.Msg, 16)
	m := New(Options{Graphics: true, ProgramStatus: true, Timeout: time.Second, Limit: 10, Connection: config.Connection{URL: server.URL},
		Ranged: true, Window: time.Minute, Step: time.Minute, End: start.Add(time.Minute)}, nil)
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(&output), tea.WithWindowSize(80, 24),
		tea.WithFilter(func(_ tea.Model, msg tea.Msg) tea.Msg {
			if _, ok := msg.(tea.RawMsg); ok {
				events <- msg
			}
			return msg
		}))
	finished := make(chan error, 1)
	go func() {
		_, err := p.Run()
		output.WriteString(m.CloseTerminal())
		finished <- err
	}()
	p.Send(tea.KeyPressMsg{Code: 'i', Text: "i"})
	p.Send(tea.PasteMsg{Content: "up"})
	p.Send(tea.KeyPressMsg{Code: tea.KeyF5})
	awaitPacket := func(match func(any) bool) any {
		t.Helper()
		var seen []string
		for {
			select {
			case msg := <-events:
				packet := msg.(tea.RawMsg).Msg
				if match(packet) {
					return packet
				}
				seen = append(seen, fmt.Sprintf("%T %.100q", packet, packet))
			case <-ctx.Done():
				<-finished
				t.Fatalf("timed out waiting for terminal output (saw %v): editor=%q size=%dx%d rows=%d graph=%+v",
					seen, m.editor.Value(), m.width, m.height, len(m.table.Rows), m.chart)
			}
		}
	}
	upload := awaitPacket(func(packet any) bool {
		graph, ok := packet.(graphPacket)
		return ok && strings.Contains(string(graph), "a=T")
	}).(graphPacket)
	p.Send(tea.KeyPressMsg{Code: 'i', Text: "i"})
	p.Send(tea.PasteMsg{Content: "x"})
	p.Send(tea.KeyPressMsg{Code: tea.KeyF5})
	awaitPacket(func(packet any) bool {
		return packet == statusPacket(programstatus.Report(programstatus.Error, "Query failed"))
	})
	p.Send(tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl})
	if err := <-finished; err != nil {
		t.Fatal(err)
	}

	stream := output.String()
	for _, want := range []string{
		programstatus.Report(programstatus.Working, "Executing PromQL query"),
		programstatus.Report(programstatus.Done, "Query completed"),
		string(upload),
		programstatus.Report(programstatus.Working, "Executing PromQL query"),
		programstatus.Report(programstatus.Error, "Query failed"),
		programstatus.Clear(),
	} {
		_, rest, ok := strings.Cut(stream, want)
		if !ok {
			t.Fatalf("missing ordered terminal output %q", want)
		}
		stream = rest
	}
	if m.editor.Value() != "upx" || len(m.table.Rows) != 0 || !strings.Contains(m.queryError.full, "private backend detail") {
		t.Fatal("unexpected editor input, stale results, or lost detailed query error")
	}
}

func TestEmptyResultReportsDone(t *testing.T) {
	m := New(Options{ProgramStatus: true, Timeout: time.Second}, nil)
	m.editor.SetValue("up")
	m.run()
	m.Update(m.flushProgramStatus(nil)())
	_, cmd := m.Update(completed{id: m.id})
	want := statusPacket(programstatus.Report(programstatus.Done, "Query completed"))
	if got := cmd().(tea.RawMsg).Msg; got != want || !strings.Contains(m.screen(), "No data") {
		t.Fatalf("unexpected empty result status: %q", got)
	}
}
