package alertmanager

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func mustInhibitRules(t *testing.T, yaml string) []InhibitRule {
	t.Helper()
	rules, err := parseInhibitRules(yaml)
	if err != nil {
		t.Fatal(err)
	}
	return rules
}

func inhibitAlert(id, severity, service string) Alert {
	return Alert{
		Fingerprint: id,
		Labels:      map[string]string{"alertname": "HighLatency", "severity": severity, "servicename": service},
	}
}

func TestIndexInhibitionRuntimeAndRules(t *testing.T) {
	rules := mustInhibitRules(t, `inhibit_rules:
- source_matchers: ['severity="critical"']
  target_matchers: ['severity!="critical"', 'severity=~"warn|warning"', 'zone!~"blocked.*"']
  equal: [alertname, servicename, region]
- source_match: {severity: critical}
  target_match_re: {severity: "warn.*"}
  equal: [alertname, servicename]
`)
	rows := []Alert{
		inhibitAlert("primary", "critical", "payments"),
		inhibitAlert("secondary", "critical", "payments"),
		inhibitAlert("one", "warning", "payments"),
		inhibitAlert("two", "warn", "payments"),
		inhibitAlert("other-service", "warning", "checkout"),
		inhibitAlert("potential", "warning", "payments"),
		inhibitAlert("wrong-regex", "prewarning", "payments"),
		inhibitAlert("wrong-name", "warning", "payments"),
		inhibitAlert("blocked-zone", "warning", "payments"),
		inhibitAlert("wrong-region", "warning", "payments"),
	}
	rows[0].Labels["region"] = ""
	rows[2].Status.InhibitedBy = []string{"primary", "primary", "secondary", "missing"}
	rows[3].Status.InhibitedBy = []string{"primary"}
	rows[7].Labels["alertname"] = "Different"
	rows[8].Labels["zone"] = "blocked-east"
	rows[9].Labels["region"] = "west"
	idx := IndexInhibition(rows, rules, time.Now())
	check := func(i int, want ...Inhibitor) {
		t.Helper()
		if got := idx.Sources(i); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %+v, want %+v", rows[i].Fingerprint, got, want)
		}
	}
	check(0)
	check(2, Inhibitor{0, "primary", true, []int{0, 1}}, Inhibitor{1, "secondary", true, []int{0, 1}}, Inhibitor{-1, "missing", true, nil})
	check(3, Inhibitor{0, "primary", true, []int{0, 1}}, Inhibitor{1, "secondary", false, []int{0, 1}})
	check(4)
	check(5, Inhibitor{0, "primary", false, []int{0, 1}}, Inhibitor{1, "secondary", false, []int{0, 1}})
	check(6)
	check(7)
	check(8, Inhibitor{0, "primary", false, []int{1}}, Inhibitor{1, "secondary", false, []int{1}})
	check(9, Inhibitor{0, "primary", false, []int{1}}, Inhibitor{1, "secondary", false, []int{1}})
	if !reflect.DeepEqual(idx.Targets(0), []int{2, 3}) || !reflect.DeepEqual(idx.Targets(1), []int{2}) {
		t.Fatal("reverse links must be deduplicated and runtime-confirmed only")
	}
}

func TestIndexInhibitionTwoSidedAndExpiredSources(t *testing.T) {
	rules := mustInhibitRules(t, `inhibit_rules:
- source_matchers: ['severity=~"critical|both"']
  target_matchers: ['severity=~"warning|both"']
`)
	now := time.Now()
	rows := []Alert{
		inhibitAlert("source-only", "critical", ""),
		inhibitAlert("both-a", "both", ""),
		inhibitAlert("both-b", "both", ""),
		inhibitAlert("target-only", "warning", ""),
		inhibitAlert("expired", "critical", ""),
	}
	rows[4].EndsAt = now.Add(-time.Second)
	rows[1].Status.InhibitedBy = []string{"both-a"}
	idx := IndexInhibition(rows, rules, now)
	if got := idx.Sources(1); !reflect.DeepEqual(got, []Inhibitor{
		{-1, "both-a", true, nil}, {0, "source-only", false, []int{0}},
	}) || len(idx.Targets(1)) != 0 {
		t.Fatalf("self reference or two-sided overlap resulted in a link: %+v", got)
	}
	if got := idx.Sources(3); !reflect.DeepEqual(got, []Inhibitor{
		{0, "source-only", false, []int{0}}, {1, "both-a", false, []int{0}}, {2, "both-b", false, []int{0}},
	}) {
		t.Fatalf("target-only alerts should accept two-sided sources but not expired sources: %+v", got)
	}
}

func TestInhibitionRuntimeWithoutRules(t *testing.T) {
	var rows [3]Alert
	rows[0].Status.State = "inhibited"
	rows[1].Status.State = "suppressed"
	rows[2].Status.InhibitedBy = []string{"missing"}
	if !rows[0].IsInhibited() || rows[1].IsInhibited() || !rows[2].IsInhibited() {
		t.Fatal("incorrect runtime interpretation")
	}
	idx := IndexInhibition(rows[:], nil, time.Now())
	if got := idx.Sources(2); !reflect.DeepEqual(got, []Inhibitor{{-1, "missing", true, nil}}) || len(idx.Sources(0)) != 0 {
		t.Fatalf("missing references must not be inferred: %+v", got)
	}
}

func TestClientInhibitionConfiguration(t *testing.T) {
	for _, tt := range []struct{ name, body, wantErr string }{
		{"no rules", `{"config":{"original":"route: {receiver: default}"}}`, ""},
		{"missing original", `{"config":{}}`, "missing config.original"},
		{"invalid YAML", `{"config":{"original":"receivers: [private-secret"}}`, "could not parse"},
		{"empty rule", `{"config":{"original":"inhibit_rules: [null]"}}`, "rule 1 is empty"},
		{"invalid matcher", `{"config":{"original":"inhibit_rules:\n- source_matchers: ['foo=~\"[\"']"}}`, "invalid source matcher in inhibit rule 1"},
		{"unavailable status", "", "HTTP 503"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.URL.Path != "/proxy/api/v2/status" || req.Method != http.MethodGet {
					t.Errorf("unexpected request %s %s", req.Method, req.URL)
				}
				if tt.body == "" {
					w.WriteHeader(http.StatusServiceUnavailable)
				}
				fmt.Fprint(w, tt.body)
			}))
			defer server.Close()
			rules, err := (Client{Base: server.URL + "/proxy"}).InhibitRules(t.Context())
			if tt.wantErr == "" {
				if err != nil || len(rules) != 0 {
					t.Fatalf("wanted an empty rule set, got %v, %v", rules, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) || strings.Contains(err.Error(), "private-secret") {
				t.Fatalf("wanted safe error %q, got %v", tt.wantErr, err)
			}
		})
	}
}
