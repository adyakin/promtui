package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/api"
	"promtui/internal/config"
)

func targetCommands(cmd tea.Cmd) []tea.Cmd {
	if cmd == nil {
		return nil
	}
	if msg, ok := cmd().(tea.BatchMsg); ok {
		return msg
	}
	panic("expected a batch")
}

func TestTargetsIndependentRefresh(t *testing.T) {
	var phase atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := phase.Load()
		b := strings.HasPrefix(r.URL.Path, "/b/")
		if p == 2 || p == 1 && b {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		item := "shared"
		if p == 3 && b {
			item = "recovered"
		}
		fmt.Fprintf(w, `{"status":"success","data":{"activeTargets":[{"health":"down","labels":{"instance":%q}},{"health":"up","labels":{"instance":"healthy"}}]}}`, item)
	}))
	defer server.Close()
	conn := config.Connection{URL: "http://unused.test", Targets: []config.TargetSource{
		{Name: "a", URL: server.URL + "/a"}, {Name: "b", URL: server.URL + "/b"},
	}}
	m := New(Options{Connection: conn, Timeout: time.Second}, nil)
	a := &m.targets
	now := time.Now()
	m.activeTab = targetsTab
	for p, statuses := range [][2]string{{"OK", "OK"}, {"OK", "Stale"}, {"Stale", "Stale"}, {"OK", "OK"}} {
		phase.Store(int32(p))
		batch := targetCommands(m.refreshTargets())
		if len(batch) != 2 || m.refreshTargets() != nil || a.sources[0].status() != "Loading" || a.sources[1].status() != "Loading" {
			t.Fatalf("phase %d: refresh should start once for both sources", p)
		}
		if p > 0 && len(a.visible) != 4 {
			t.Fatal("refresh hid cached rows")
		}
		for _, cmd := range batch {
			m.onTargetsCompleted(cmd().(targetsCompleted), now)
		}
		if a.sources[0].status() != statuses[0] || a.sources[1].status() != statuses[1] || len(a.visible) != 4 {
			t.Fatalf("phase %d: statuses %s %s, %d rows", p, a.sources[0].status(), a.sources[1].status(), len(a.visible))
		}
		screen := ansi.Strip(m.screen())
		if p == 0 {
			a.moveTo(1)
		}
		if p == 1 && (!strings.Contains(screen, "Incomplete · 1/2 OK") || !strings.Contains(screen, "│  b:") ||
			strings.Contains(screen, "shared") || a.visible[a.row].source != "b") {
			t.Fatal(screen)
		}
		if p == 1 {
			press(m, 'f', tea.KeyDown, tea.KeyEnter)
			if screen := ansi.Strip(m.screen()); !strings.Contains(screen, "shared") || strings.Contains(screen, "│  b:") {
				t.Fatal(screen)
			}
			press(m, 'f', tea.KeyUp, tea.KeyEnter)
		}
		if p == 2 && (!strings.Contains(screen, "│  All sources unavailable") || !strings.Contains(screen, "│  a:") ||
			!strings.Contains(screen, "│  b:") || strings.Contains(screen, "shared")) {
			t.Fatal(screen)
		}
		if p == 3 && a.sources[1].rows[0].instance != "recovered" {
			t.Fatal(screen)
		}
		now = now.Add(targetRefreshInterval)
	}
}

func TestTargetsBoundedQueueAndCancellation(t *testing.T) {
	var sources []config.TargetSource
	for i := range targetConcurrency + 2 {
		sources = append(sources, config.TargetSource{Name: fmt.Sprint(i), URL: "http://example.test"})
	}
	m := New(Options{Connection: config.Connection{Targets: sources}}, nil)
	m.activeTab = targetsTab
	a := &m.targets
	start := targetCommands(m.refreshTargets())
	if len(start) != targetConcurrency || a.next != targetConcurrency {
		t.Fatal("queue must only dispatch up to the concurrency bound")
	}
	id := a.requestID
	for i := range len(sources) {
		cmd := m.onTargetsCompleted(targetsCompleted{id: id, source: i}, time.Now())
		if (i < 2 || i == len(sources)-1) != (cmd != nil) {
			t.Fatalf("unexpected command after %d", i)
		}
	}
	if cmd := m.onTargetsTick(a.lastDone.Add(targetRefreshInterval)); cmd == nil {
		t.Fatal("auto-refresh did not run at the interval")
	}
	ctx, id := a.ctx, a.requestID
	press(m, '0')
	if ctx.Err() == nil || m.onTargetsCompleted(targetsCompleted{id: id, source: 0}, time.Now()) != nil {
		t.Fatal("tab switch failed to cancel or ignore old requests")
	}
	if m.onTargetsTick(time.Now()) != nil {
		t.Fatal("inactive tab refreshed")
	}
	press(m, '2')
	if a.cancel == nil || a.requestID == id {
		t.Fatal("returning to an interrupted refresh did not restart it")
	}
}

func TestTargetsUI(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"success","data":{"activeTargets":[{"health":"up","labels":{"instance":"alpha","job":"beta"},"lastScrapeDuration":0.13},{"health":"down","labels":{"instance":"zeta","job":"alpha","env":"PROD"},"discoveredLabels":{"__meta_region":"North"},"lastError":"`+strings.Repeat("long", 30)+`"},{"labels":{"instance":"unknown"}}],"droppedTargets":[{"discoveredLabels":{"__address__":"dropped","job":"unused"}}]}}`)
	}))
	defer s.Close()
	m := New(Options{Connection: config.Connection{URL: s.URL}, Timeout: time.Second}, nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	msg := cmd().(targetsCompleted)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	m.Update(msg)
	a := &m.targets
	if len(a.visible) != 3 || a.visible[0].health != targetDown || a.visible[1].health != targetUnknown ||
		a.visible[2].health != targetUp || a.visible[0].source != "default" || !a.sources[0].fallback {
		t.Fatal("wrong starting view, default source, or ordering")
	}
	for _, filter := range []string{"DOW", "dwn", "mt_rgn", "nrth", "lnglng"} {
		press(m, '/')
		m.Update(tea.PasteMsg{Content: filter})
		if len(a.visible) != 1 || a.visible[0].health != targetDown {
			t.Fatalf("filter %q did not find the down target:\n%s", filter, m.screen())
		}
		press(m, tea.KeyEsc)
	}
	press(m, tea.KeyEnd)
	id := a.selection()
	press(m, 's', 's', 's', 's')
	if a.sort != sortTargetDuration || a.selection() != id || a.row != 0 {
		t.Fatal("duration sorting lost the selected target or placed unavailable durations first")
	}
	press(m, 'f', tea.KeyDown, tea.KeyEsc)
	if a.source != 0 {
		t.Fatal("cancel applied source selection")
	}
	press(m, 'f', tea.KeyDown, tea.KeyEnter, '/')
	m.Update(tea.PasteMsg{Content: "env=prod"})
	if len(a.visible) != 1 || a.visible[0].instance != "zeta" || !strings.Contains(ansi.Strip(m.footer()), "Esc Cancel") {
		t.Fatal(m.screen())
	}
	press(m, tea.KeyEnter, 'h') // DOWN
	if len(a.visible) != 1 || a.visible[0].health != targetDown {
		t.Fatal("DOWN filter did not retain the matching target")
	}
	press(m, 'h') // UP
	if len(a.visible) != 0 || !strings.Contains(m.screen(), "No targets matching the current filter") {
		t.Fatal(m.screen())
	}
	press(m, '0', '2', '/')
	if a.source != 1 || a.filter != "env=prod" || a.sort != sortTargetDuration || targetHealthOrder[a.health] != targetUp {
		t.Fatal("tab switch lost the filter, source, health or sort")
	}
	a.input.SetValue("")
	a.rebuild("", 0)
	press(m, tea.KeyEnter, 'd')
	if len(a.visible) != 1 || a.visible[0].health != targetDropped || a.visible[0].instance != "dropped" {
		t.Fatal("dropped view was not isolated")
	}
	if text := strings.Join(targetDetailLines(*a.visible[0]), "\n"); strings.Contains(text, "Last scrape") || !strings.Contains(text, "__address__") {
		t.Fatal(text)
	}
	press(m, 'd', 'h', 'h', 'h') // UP → UNKNOWN → DROPPED → Active
	if len(a.visible) != 3 {
		t.Fatal("active view did not return")
	}
	press(m, tea.KeyDown, tea.KeyEnter)
	for _, size := range [][2]int{{80, 24}, {24, 10}} {
		m.resize(size[0], size[1])
		for _, key := range []rune{tea.KeyPgDown, tea.KeyEnd} {
			press(m, key)
		}
		rows := strings.Split(ansi.Strip(m.screen()), "\n")
		if a.dialog != targetDetails || len(rows) != size[1] ||
			strings.TrimRight(rows[len(rows)-1], " ") != strings.TrimRight(ansi.Strip(m.footer()), " ") ||
			!strings.Contains(ansi.Strip(m.footer()), "Esc Close") || strings.Contains(ansi.Strip(m.footer()), "r Refresh") {
			t.Fatalf("invalid details layout or footer:\n%s", m.screen())
		}
		for _, row := range rows {
			if ansi.StringWidth(row) > size[0] {
				t.Fatalf("row too long: %q", row)
			}
		}
	}
	press(m, tea.KeyEsc)
	if !strings.Contains(m.screen(), "Instance") || strings.Contains(m.screen(), "Duration") {
		t.Fatal(m.screen())
	}
}

func TestTargetSelectionSurvivesChangedScrapeAndSourceOrdering(t *testing.T) {
	m := New(Options{Connection: config.Connection{Targets: []config.TargetSource{{Name: "a"}, {Name: "b"}}}}, nil)
	m.activeTab = targetsTab
	a := &m.targets
	makeData := func(rank string) api.Targets {
		return api.Targets{Active: []api.Target{{Health: rank, Labels: map[string]string{"instance": "same"}}}}
	}
	targetCommands(m.refreshTargets())
	m.onTargetsCompleted(targetsCompleted{a.requestID, 0, makeData("up"), nil}, time.Now())
	m.onTargetsCompleted(targetsCompleted{a.requestID, 1, makeData("down"), nil}, time.Now())
	a.moveTo(0)
	id := a.selection()
	targetCommands(m.refreshTargets())
	m.onTargetsCompleted(targetsCompleted{a.requestID, 1, makeData("up"), nil}, time.Now())
	m.onTargetsCompleted(targetsCompleted{a.requestID, 0, makeData("down"), nil}, time.Now())
	if a.selection() != id || a.row != 1 || a.visible[a.row].source != "b" {
		t.Fatal("refresh lost the source-scoped selection when health changed")
	}
}
