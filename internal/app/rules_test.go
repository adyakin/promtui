package app

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/api"
	"promtui/internal/config"
)

const multilineRule = "default_rollup(\n  up\n) > 0"

func testRulesSnapshot(group string) api.Rules {
	return api.Rules{Groups: []api.RuleGroup{{Name: group, File: "rules.yml", Rules: []api.Rule{
		{Name: "Shared", Type: api.AlertingRule, State: "firing", Expression: multilineRule, Health: "ok", Position: 0,
			Alerts: []api.RuleAlert{
				{State: "firing", Value: "7", Labels: map[string]string{"instance": "backend01"}, Annotations: map[string]string{"summary": "Backend overloaded"}},
				{State: "pending", Labels: map[string]string{"instance": "backend02"}},
			}},
		{Name: "Shared", Type: api.RecordingRule, Expression: "sum(up)", Position: 1, LastError: strings.Repeat("long evaluation error ", 100)},
		{Name: "Quiet", Type: api.AlertingRule, State: "inactive", Expression: "up > 100", Position: 2},
	}}}}
}

func TestRulesTreeSearchAndPromQL(t *testing.T) {
	m := New(Options{Connection: config.Connection{Name: "main", URL: "http://unused"}, Ranged: true, Window: time.Hour}, nil)
	m.Update(completed{result: sampleResult(true)})
	m.editor.SetValue("old draft")
	press(m, '5')
	m.onRulesLoaded(rulesLoaded{m.rules.requestID, 0, testRulesSnapshot("nodes"), nil}, time.Now())
	s := &m.rules
	if s.selection().title != "nodes" || len(s.visible) != 4 {
		t.Fatalf("single-source tree = %+v", s.visible)
	}
	press(m, 'j')
	ruleID := s.selected
	press(m, 'l', 'j')
	if n := s.selection(); n.alert == nil || n.alert.Value != "7" {
		t.Fatalf("alert selection = %+v", n)
	}
	for _, text := range []string{"Rule: Shared", "Value: 7", "Backend overloaded"} {
		if details := strings.Join(rulesDetails(s.selection(), 60), "\n"); !strings.Contains(details, text) {
			t.Fatalf("missing %q in %q", text, details)
		}
	}
	press(m, 'h', 'h')
	if s.selected != ruleID || len(s.visible) != 4 {
		t.Fatal("did not collapse alert instances")
	}
	press(m, 'h', 'h', '/')
	m.Update(tea.PasteMsg{Content: "bcknd02"})
	if n := s.selection(); n == nil || n.alert == nil || n.title != "backend02" || len(s.visible) != 3 {
		t.Fatalf("search within collapsed group = %+v", n)
	}
	press(m, tea.KeyEnter, 'p')
	if m.activeTab != promQLTab || !m.editor.Focused() || m.editor.Value() != multilineRule ||
		m.cancel != nil || len(m.entries) != 0 || !m.opt.Ranged || m.opt.Window != time.Hour ||
		m.opt.Connection.Name != "main" || len(m.table.Rows) == 0 {
		t.Fatalf("load into editor = %q; tab=%v results=%+v", m.editor.Value(), m.activeTab, m.table.Rows)
	}
	id := s.selected
	pressAlt(m, '5')
	if s.selected != id || !strings.Contains(s.filter, "bcknd02") {
		t.Fatal("tab switch lost search or selection")
	}
	press(m, tea.KeyEsc, 'f')
	if ruleTypeFilters[s.ruleType] != api.AlertingRule || s.selected != id {
		t.Fatal("alerting filter lost the active alert")
	}
	press(m, 'f')
	if n := s.selection(); n == nil || len(s.visible) != 2 {
		t.Fatalf("recording filter = %+v", s.visible)
	}
	press(m, 'j', 'e')
	if s.overlayTitle == "" || !strings.Contains(ansi.Strip(m.screen()), "long evaluation error") {
		t.Fatal("no error overlay")
	}
	press(m, tea.KeyPgDown)
	if s.overlayTop == 0 {
		t.Fatal("could not scroll error overlay")
	}
	press(m, tea.KeyEsc, 's')
	if ruleStateFilters[s.state] != "firing" || slicesContainRule(s.visible, api.RecordingRule) {
		t.Fatalf("state filter = %+v", s.visible)
	}
	press(m, 's')
	if len(s.visible) != 0 {
		t.Fatal("instance pending state incorrectly matched a firing rule")
	}
	press(m, 's')
	if len(s.visible) != 2 || s.visible[1].title != "Quiet" {
		t.Fatal("inactive filter did not match Quiet")
	}
}

func slicesContainRule(nodes []*ruleNode, kind string) bool {
	for _, n := range nodes {
		if n.rule != nil && n.rule.Type == kind {
			return true
		}
	}
	return false
}

func TestRulesConcurrentSourcesAndTLS(t *testing.T) {
	for _, skip := range []bool{true, false} {
		t.Run(fmt.Sprint(skip), func(t *testing.T) {
			started := make(chan struct{}, 2)
			release := make(chan struct{})
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				started <- struct{}{}
				<-release
				switch r.URL.Path {
				case "/east/api/v1/rules":
					fmt.Fprint(w, `{"status":"success","data":{"groups":[{"name":"east-group","rules":[]}]}}`)
				case "/west/api/v1/rules":
					fmt.Fprint(w, `{"status":"success","data":{"groups":[{"name":"west-group","rules":[]}]}}`)
				default:
					t.Errorf("unexpected request to %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer srv.Close()
			m := New(Options{Timeout: 3 * time.Second, Connection: config.Connection{
				URL: srv.URL + "/primary", InsecureSkipVerify: skip,
				VMAlert: []config.RuleSource{{Name: "east", URL: srv.URL + "/east"}, {Name: "west", URL: srv.URL + "/west"}},
			}}, nil)
			m.activeTab = rulesTab
			batch := m.refreshRules()().(tea.BatchMsg)
			results := make(chan tea.Msg, len(batch))
			for _, cmd := range batch {
				go func() { results <- cmd() }()
			}
			if skip {
				for range batch {
					select {
					case <-started:
					case <-time.After(2 * time.Second):
						close(release)
						t.Fatal("rules requests did not reach both endpoints concurrently")
					}
				}
			}
			close(release)
			for range batch {
				m.Update(<-results)
			}
			want := "2/2 sources current"
			if !skip {
				want = "2 failed · 0/2 sources current"
			}
			if got := m.rules.status(); !strings.Contains(got, want) {
				t.Fatalf("status = %q", got)
			}
		})
	}
}

func TestRulesSourceFailureRefreshAndConnectionSwitch(t *testing.T) {
	conn := config.Connection{Name: "first", VMAlert: []config.RuleSource{{Name: "east"}, {Name: "west"}}}
	next := config.Connection{Name: "second", URL: "http://unused"}
	m := New(Options{Connection: conn, Connections: []config.Connection{conn, next}}, nil)
	press(m, '5')
	s := &m.rules
	id := s.requestID
	m.Update(rulesLoaded{id, 1, testRulesSnapshot("node"), nil})
	if s.pending != 1 || !strings.Contains(s.status(), "1/2 sources current") {
		t.Fatal("successful source did not appear while other source was loading")
	}
	m.Update(rulesLoaded{id, 0, api.Rules{}, errors.New("Rules request timed out")})
	if !strings.Contains(s.status(), "1 failed") || s.sources[1].data == nil {
		t.Fatal("failed source hid successful data")
	}
	press(m, tea.KeyEnd)
	selected := s.selected
	press(m, 'r')
	press(m, 'r')
	m.Update(rulesLoaded{s.requestID - 2, 1, testRulesSnapshot("obsolete"), nil})
	m.Update(rulesLoaded{s.requestID, 1, api.Rules{}, errors.New("Rules API returned HTTP 503")})
	m.Update(rulesLoaded{s.requestID, 0, testRulesSnapshot("node"), nil})
	if !strings.Contains(s.status(), "1 stale") || s.selected != selected ||
		s.sources[1].data.Groups[0].Name != "node" {
		t.Fatalf("stale refresh = %s, selected %s", s.status(), s.selected)
	}
	press(m, 'r')
	id = s.requestID
	press(m, 'c', tea.KeyDown, tea.KeyEnter)
	m.Update(rulesLoaded{id, 1, testRulesSnapshot("obsolete"), nil})
	if m.opt.Connection.Name != "second" || s.pending != 1 || len(s.sources) != 1 || s.sources[0].data != nil {
		t.Fatalf("connection switch = %+v, %+v", m.opt.Connection, s)
	}
}

func TestRulesScreenFitsResize(t *testing.T) {
	m := New(Options{Connection: config.Connection{Name: "local"}}, nil)
	press(m, '5')
	m.Update(rulesLoaded{m.rules.requestID, 0, testRulesSnapshot("nodes"), nil})
	press(m, 'j')
	for _, size := range [][2]int{{24, 10}, {67, 18}, {68, 10}, {120, 30}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, modal := range []bool{false, true} {
			if modal {
				press(m, '?')
			}
			rows := strings.Split(ansi.Strip(m.screen()), "\n")
			if len(rows) != size[1] || rows[len(rows)-1] != ansi.Strip(m.footer()) {
				t.Fatalf("resize at %v: %q", size, rows)
			}
			for _, line := range rows {
				if ansi.StringWidth(line) > size[0] {
					t.Fatalf("resize at %v produced wide line %q", size, line)
				}
			}
			if modal {
				press(m, tea.KeyEsc)
			}
		}
	}
}
