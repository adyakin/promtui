package app

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/kitty"
	"promtui/internal/query"
)

func TestQueryErrorOverlay(t *testing.T) {
	m := New(Options{Graphics: true, Ranged: true}, nil)
	m.resize(minWidth, minHeight)
	raw := &url.Error{
		Op: "Post", URL: "https://prometheus.example.com/api/v1/query_range",
		Err: context.DeadlineExceeded,
	}
	press(m, 'e')
	if m.panel != 0 || strings.Contains(m.footer(), "Error details") {
		t.Fatal("Error details is available without a failure")
	}
	m.Update(completed{err: raw, elapsed: 30*time.Second + time.Millisecond})
	screen := ansi.Strip(m.screen())
	if !strings.Contains(screen, "Timed out (30.001s)") ||
		!strings.Contains(screen, "e Error details") || strings.Contains(screen, raw.URL) {
		t.Fatal(screen)
	}

	w, scope := m.workspace(), m.mainScope()
	press(m, 'e', tea.KeyTab)
	screen = ansi.Strip(m.screen())
	if m.panel != queryErrorKeys || m.workspace() != w || !strings.Contains(screen, "Query Error") ||
		!strings.Contains(screen, "Esc Close") || !strings.Contains(screen, "y Copy") || !strings.Contains(screen, "↑↓") {
		t.Fatal(screen)
	}
	text, room, limit := m.errorViewport()
	if room < 1 || limit < 1 || !strings.Contains(strings.Join(text, ""), raw.URL) {
		t.Fatalf("narrow dialog lost text or cannot scroll: %v", text)
	}
	press(m, tea.KeyDown)
	if m.offset != 1 {
		t.Fatal("Down did not scroll one line")
	}
	press(m, tea.KeyPgDown)
	if m.offset != min(1+room, limit) {
		t.Fatal("PgDn did not scroll one page")
	}
	press(m, tea.KeyPgUp, tea.KeyUp)
	if m.offset != 0 {
		t.Fatal("PgUp and Up did not return to the start")
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'y'})
	if got := cmd().(copyQueryError); string(got) != raw.Error() {
		t.Fatalf("copy changed the error: %q", got)
	}
	press(m, tea.KeyEsc)
	if m.panel != 0 || m.scope() != scope || m.queryError.full != raw.Error() {
		t.Fatal("Esc changed focus or discarded the error")
	}
}

func TestQueryErrorMultilineAndControlCharacters(t *testing.T) {
	full := "first line\r\n\r\n" + strings.Repeat("長", 40) + "\n\x1b[2Jlast line"
	q := newQueryError(errors.New(full), 0)
	lines := q.lines(20)
	wrapped := strings.Join(lines, "\n")
	if !strings.Contains(wrapped, "first line\n\n") || !strings.Contains(wrapped, "last line") ||
		strings.Contains(wrapped, "\x1b") || strings.Count(wrapped, "長") != 40 || q.full != full {
		t.Fatalf("lost multiline text or emitted a control: %q", wrapped)
	}
	for _, line := range lines {
		if ansi.StringWidth(line) > 20 {
			t.Fatalf("line exceeds overlay width: %q", line)
		}
	}
	q = newQueryError(errors.New(`backend could not reach https://prom.example/query?token=secret`), 0)
	if got := q.summary(80); strings.Contains(got, "secret") || !strings.Contains(got, "backend could not reach") {
		t.Fatal(got)
	}
}

func TestQueryErrorWrapsInResultsAndKeepsDetailsAccessible(t *testing.T) {
	m := New(Options{}, nil)
	m.resize(72, 24)
	const cause = "HTTP 422 Unprocessable Entity: execution: vector cannot contain metrics with the same labelset"
	const tail = "check whether the query removes the metric name"
	m.Update(completed{err: errors.New(cause + "\n" + tail), elapsed: 3919 * time.Millisecond})
	screen := ansi.Strip(m.screen())
	for _, want := range []string{"Query failed after 3.919s:", "same labelset", tail, "e Error details"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("missing %q in Results:\n%s", want, screen)
		}
	}

	m.resize(40, 11)
	screen = ansi.Strip(m.screen())
	if strings.Contains(screen, tail) || !strings.Contains(screen, "…") || !strings.Contains(screen, "e Error details") {
		t.Fatalf("short layout should show that more details are available:\n%s", screen)
	}
	press(m, 'e')
	lines, _, limit := m.errorViewport()
	text := strings.Join(strings.Fields(strings.Join(lines, "")), "")
	if limit == 0 || !strings.Contains(text, "removesthemetricname") {
		t.Fatal("the complete error should remain scrollable in the dialog")
	}
}

func TestQueryResultsFollowLatestExecution(t *testing.T) {
	m := New(Options{Graphics: true, Ranged: true, Window: time.Minute, Timeout: time.Second}, nil)
	m.editor.SetValue("up")
	m.run()
	m.Update(completed{id: m.id, result: sampleResult(true)})
	press(m, 'i')
	m.Update(tea.PasteMsg{Content: " + 1"})
	if len(m.table.Rows) != 2 {
		t.Fatal("editing the query cleared the result")
	}
	press(m, tea.KeyTab)
	m.openLabels()
	m.notice = "old warning"
	run := func() int {
		if m.run() == nil || len(m.table.Rows)+len(m.chart.result.Series)+len(m.chart.visible) != 0 ||
			m.chart.selected != 0 || m.notice != "" || m.labelText != "" || m.queryError.full != "" ||
			!strings.Contains(m.screen(), executingQuery) {
			t.Fatal("starting a query retained stale state")
		}
		return m.id
	}
	old := run()
	latest := run()
	m.Update(completed{id: latest, err: errors.New("latest failed")})
	m.Update(completed{id: old, result: sampleResult(true)})
	if m.queryError.full != "latest failed" || len(m.table.Rows) != 0 {
		t.Fatal("stale response replaced the latest failure")
	}
	press(m, 't')
	if m.id != latest || m.opt.Ranged || !strings.Contains(m.screen(), "Results · Table") {
		t.Fatal("switching to Table retried the failed query")
	}
	press(m, 'g', '1', '0')
	if m.id != latest || !m.opt.Ranged || m.queryError.full != "latest failed" ||
		!strings.Contains(m.screen(), "Results · Graph") {
		t.Fatal("switching views or tabs discarded the error")
	}
	m.Update(completed{id: run(), result: sampleResult(true)})
	m.Update(completed{id: latest, err: errors.New("stale failure")})
	if m.queryError.full != "" || len(m.table.Rows) != 2 {
		t.Fatal("stale failure displaced a successful result")
	}
	m.Update(completed{id: run(), result: query.Result{Start: time.Now()}})
	if !strings.Contains(m.screen(), "No data") || m.chart.enabled || len(m.table.Rows) != 0 {
		t.Fatal("empty response exposed stale data")
	}
}

func TestStartingQueryDeletesInFlightKittyPlacement(t *testing.T) {
	m := New(Options{Graphics: true, Timeout: time.Second}, nil)
	m.editor.SetValue("up")
	_, render := m.Update(completed{result: sampleResult(true)})
	_, upload := m.Update(render())
	press(m, tea.KeyF5)
	// A previous upload already queued in Tea must be acknowledged before
	// sending the delete, or it could repaint a stale image afterward.
	_, clear := m.Update(upload())
	if clear == nil || clear().(tea.RawMsg).Msg != graphPacket(kitty.Clear()) || m.chart.displayed.cols != 0 {
		t.Fatal("starting a query did not remove the previous Kitty placement")
	}
	m.stop()
}
