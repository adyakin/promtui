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
	"promtui/internal/api"
	"promtui/internal/config"
)

func alertAction(m *Model, key rune) tea.Cmd {
	_, cmd := m.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
	return cmd
}

func TestVMAlertNavigation(t *testing.T) {
	const original = "  up{job=\"nodes\"}\n\t== 0  "
	active := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	var alertRequests atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/west/vmalert/api/v1/alert":
			alertRequests.Add(1)
			fmt.Fprintf(w, `{"group_id":"2485214638202814937","id":"4564276932026302966","rule_id":"3452656140727507818","activeAt":%q,"expression":%q}`, active.Format(time.RFC3339), original)
		case "/west/vmalert/api/v1/rules":
			fmt.Fprint(w, `{"status":"success","data":{"groups":[{"id":"2485214638202814937","name":"nodes","rules":[
				{"type":"alerting","name":"NodeDown","id":"3452656140727507819","query":"up{job=\"nodes\"} == 0"},
				{"type":"alerting","name":"NodeDown","id":"3452656140727507818","query":"up{job=\"nodes\"} == 0","labels":{"severity":"critical","team":"oncall"}}
			]}]}}`)
		case "/select/0/prometheus/api/v1/query":
			if r.FormValue("time") != active.UTC().Format(time.RFC3339) || r.FormValue("query") != `up{job="nodes"}` {
				t.Errorf("wrong activation probe: %v", r.Form)
			}
			fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"job":"nodes","instance":"node-1"},"value":[100,"0"]}]}}`)
		case "/select/0/prometheus/api/v1/query_range":
			start, err := time.Parse(time.RFC3339Nano, r.FormValue("start"))
			if err != nil || start.After(active.Add(-time.Hour)) || time.Since(start) > 24*time.Hour ||
				!strings.Contains(r.FormValue("query"), `instance="node-1"`) || strings.Contains(r.FormValue("query"), "==") {
				t.Errorf("wrong graph request: %v", r.Form)
			}
			fmt.Fprint(w, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"instance":"node-1"},"values":[[98,"1"],[100,"0"]]}]}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	m := New(Options{Graphics: true, Timeout: time.Second, Limit: 20, Connection: config.Connection{
		URL: srv.URL + "/select/0/prometheus", InsecureSkipVerify: true,
		VMAlert: []config.RuleSource{{Name: "east", URL: srv.URL + "/east/vmalert"}, {Name: "west", URL: srv.URL + "/west/vmalert"}},
	}}, nil)
	const ids = "?group_id=2485214638202814937&alert_id=4564276932026302966"
	press(m, '1')
	addAlertWithGenerator(m, srv.URL+"/west/vmalert/alert"+ids)
	row := &m.alerts.rows[0]
	row.Labels["instance"], row.Labels["job"] = "node-1", "nodes"
	m.alerts.filter = "NodeDown"
	m.alerts.rebuild(row.id, 0, time.Now())
	selection := m.alerts.selection()

	press(m, tea.KeyEnter)
	cmd := alertAction(m, 'p')
	if m.activeTab != alertsTab || m.alertNavigation.cancel == nil || alertRequests.Load() != 0 ||
		!strings.Contains(m.alerts.detailStatus, "Resolving") {
		t.Fatal("navigation did not defer network I/O")
	}
	_, run := m.Update(cmd())
	if m.activeTab != promQLTab || !m.opt.Ranged || m.opt.Window != 6*time.Hour || m.queryPhase != queryRunning || !m.graphView() {
		t.Fatal("p did not start a historical graph on PromQL")
	}
	m.Update(run())
	if !m.chart.enabled || len(m.table.Rows) != 1 || m.table.Rows[0].Samples[0].Value != 1 ||
		!strings.Contains(m.notice, "Scoped by instance, job · Threshold removed") {
		t.Fatalf("graph did not retain healthy values or transformation feedback: %s", m.notice)
	}

	press(m, '1', tea.KeyEnter)
	if m.alerts.selection() != selection || m.alerts.filter != "NodeDown" {
		t.Fatal("navigation changed the alert selection or search")
	}
	m.Update(alertAction(m, 'P')())
	if m.activeTab != promQLTab || m.editor.Value() != original || alertRequests.Load() != 1 || m.queryPhase != querySucceeded {
		t.Fatalf("P did not reuse metadata and preserve the exact query: %q", m.editor.Value())
	}
	pressAlt(m, '1')
	press(m, tea.KeyEnter)
	m.Update(alertAction(m, 'r')())
	node := m.rules.selection()
	if m.activeTab != rulesTab || node == nil || node.rule == nil || node.rule.ID != "3452656140727507818" || node.source.Name != "west" {
		t.Fatalf("not on the exact generating rule: %+v", node)
	}
}

func TestAlertNavigationIgnoresSupersededMessages(t *testing.T) {
	conn := config.Connection{Name: "one", URL: "https://unused.example"}
	next := config.Connection{Name: "two", URL: "https://other.example"}
	m := New(Options{Connection: conn, Connections: []config.Connection{conn, next}}, nil)
	press(m, '1')
	addAlertWithGenerator(m, "https://unused.example/graph?g0.expr=up")
	press(m, tea.KeyEnter)
	alertAction(m, 'p')
	stale := alertNavigationDone{serial: m.alertNavigation.serial, action: "P", meta: api.VMAlert{Expression: "stale"}}
	alertAction(m, 'p')
	m.Update(stale)
	if m.activeTab != alertsTab || m.alertNavigation.cancel == nil {
		t.Fatal("superseded response changed tabs or cancelled current navigation")
	}
	stale.serial = m.alertNavigation.serial
	press(m, 'c', tea.KeyDown, tea.KeyEnter)
	m.Update(stale)
	if m.opt.Connection.Name != "two" || m.activeTab != alertsTab || m.alertNavigation.cancel != nil || m.editor.Value() != "" {
		t.Fatal("response applied after connection switch")
	}
}

func TestResolvedVMAlertUsesOnlyUniqueCachedRuleInstance(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	m := New(Options{Connection: config.Connection{
		VMAlert: []config.RuleSource{{Name: "rules", URL: srv.URL + "/vmalert"}},
	}}, nil)
	press(m, '1')
	addAlertWithGenerator(m, srv.URL+"/vmalert/alert?group_id=2&alert_id=4")
	rule := api.Rule{Name: "NodeDown", Type: api.AlertingRule, Expression: "up == 0", Alerts: []api.RuleAlert{{ID: "4"}}}
	m.rules.sources[0].data = &api.Rules{Groups: []api.RuleGroup{{ID: "2", Rules: []api.Rule{rule, rule}}}}
	press(m, tea.KeyEnter)
	m.Update(alertAction(m, 'P')())
	if m.activeTab != alertsTab || !strings.Contains(m.alerts.detailStatus, "no longer available") {
		t.Fatal("ambiguous cache unexpectedly opened a rule expression")
	}
	m.rules.sources[0].data = &api.Rules{Groups: []api.RuleGroup{{ID: "2", Rules: []api.Rule{rule}}}}
	m.Update(alertAction(m, 'P')())
	if m.activeTab != promQLTab || m.editor.Value() != rule.Expression {
		t.Fatal("did not use unique cached rule expression")
	}
}

func TestPrometheusAlertRuleUsesInstanceNotName(t *testing.T) {
	row := newAlertRow("alertmanager", testAlert("node", "NodeDown", "critical", "active", time.Now()))
	row.Labels["instance"] = "wanted"
	expression := "up == 0"
	other := api.Rule{Name: "NodeDown", Type: api.AlertingRule, Expression: expression,
		Alerts: []api.RuleAlert{{Labels: map[string]string{"instance": "other"}}}}
	wanted := other
	wanted.Alerts = []api.RuleAlert{{Labels: map[string]string{"instance": "wanted"}}}
	data := &api.Rules{Groups: []api.RuleGroup{{Rules: []api.Rule{other, wanted}}}}
	exact, _ := matchAlertRules(data, row, api.VMAlert{Expression: expression}, api.VMAlertRef{})
	if len(exact) != 1 || exact[0].rule != &data.Groups[0].Rules[1] {
		t.Fatalf("unexpected matches: %+v", exact)
	}
	wanted.Alerts = nil
	data.Groups[0].Rules[1] = wanted
	exact, candidates := matchAlertRules(data, row, api.VMAlert{Expression: expression}, api.VMAlertRef{})
	if len(exact) != 0 || len(candidates) != 2 {
		t.Fatalf("duplicate rules should be search candidates: %+v %+v", exact, candidates)
	}
}
