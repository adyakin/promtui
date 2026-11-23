package alertmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestRoutingStatusAndInheritance(t *testing.T) {
	const config = `
route:
  receiver: default
  group_by: [alertname, instance]
  group_interval: 2m
  routes:
    - receiver: team
      continue: true
      group_wait: 0s
      match: {team: payments}
      match_re: {service: "api.*"}
      matchers: ['severity!="info"', 'env!~"dev|test"']
      routes:
        - matchers: ['severity="critical"']
          group_by: []
          routes:
            - repeat_interval: 1h
    - match: {team: payments}
    - match: {team: payments}
      group_by: ['...']
receivers: [{name: default}, {name: team}, {name: unused}]
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.RequestURI() != "/prefix/api/v2/status" {
			t.Errorf("unexpected request: %s %s", req.Method, req.URL)
		}
		json.NewEncoder(w).Encode(map[string]any{"config": map[string]string{"original": config}})
	}))
	defer server.Close()
	r, err := (Client{Base: server.URL + "/prefix", HTTP: server.Client()}).Routing(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	order := []string{"0", "0/0", "0/0/0", "0/0/0/0", "0/1", "0/2"}
	if !slices.Equal(r.Order, order) || r.Routes["0/0/0/0"].Parent != "0/0/0" || r.Routes["0/0/0/0"].Depth != 3 {
		t.Fatalf("bad route tree: %+v", r.Order)
	}
	if !slices.Equal(r.References["default"], []string{"0", "0/1", "0/2"}) ||
		!slices.Equal(r.References["team"], order[1:4]) || len(r.References["unused"]) != 0 || len(r.Receivers) != 3 {
		t.Fatalf("bad receiver index: %+v", r.References)
	}
	root, parent, leaf := r.Routes["0"], r.Routes["0/0"], r.Routes["0/0/0/0"]
	if !parent.Continue || leaf.Continue || root.GroupWait.Source != "" || root.GroupWait.Value.String() != "30s" ||
		leaf.Receiver != (Setting[string]{"team", "0/0"}) || leaf.GroupWait.Source != "0/0" || leaf.GroupWait.Value != 0 ||
		leaf.GroupInterval.Source != "0" || leaf.GroupInterval.Value.String() != "2m" ||
		leaf.RepeatInterval.Source != leaf.ID || leaf.RepeatInterval.Value.String() != "1h" ||
		leaf.GroupBy.Source != "0" || !slices.Equal(leaf.GroupBy.Value, []string{"alertname", "instance"}) ||
		!slices.Equal(r.Routes["0/2"].GroupBy.Value, []string{"..."}) {
		t.Fatalf("bad inheritance: root=%+v parent=%+v leaf=%+v", root, parent, leaf)
	}
	var matchers []string
	for _, m := range parent.Matchers {
		matchers = append(matchers, m.String())
	}
	if !slices.Equal(matchers, []string{`env!~"dev|test"`, `service=~"api.*"`, `severity!="info"`, `team="payments"`}) ||
		r.Routes["0/1"].Matchers[0].String() != r.Routes["0/2"].Matchers[0].String() {
		t.Fatalf("unexpected matchers: %v", matchers)
	}
}

func TestRoutingReceiverYAML(t *testing.T) {
	const jira = `- name: jira-msdevops-bug
  jira_configs:
    - send_resolved: false
      http_config:
        authorization:
          type: Bearer
          credentials: <secret>
        follow_redirects: true
        enable_http2: true
      api_url: https://sd-jira.demo.env/rest/api/2/
`
	const mixed = `- name: default
  webhook_configs:
    - url: <secret>
    - url_file: /run/secrets/webhook
  telegram_configs: [{chat_id: -100123, bot_token: '<secret>'}]
  future_configs:
    - enabled: false
      unknown_options:
        z_first: {more: [one, null, 3]}
        a_second:
          nested: "false"
  extra_field: [1, 2]
`
	r, err := parseRouting(`
route: {receiver: default}
receivers:
` + mixed + jira + "- name: unused\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Receivers[0].Integrations; !slices.Equal(got, []string{"future", "Telegram", "Webhook", "Webhook"}) {
		t.Fatalf("unexpected integration list: %v", got)
	}
	for i, want := range []string{mixed, jira, "- name: unused\n"} {
		if got := r.Receivers[i].YAML; got != want {
			t.Errorf("receiver %d YAML:\n%s\nwant:\n%s", i, got, want)
		}
	}
}

func TestRoutingReceiverFlowYAML(t *testing.T) {
	r, err := parseRouting("route: {receiver: first}\nreceivers: [{name: first}, {future_configs: [{field: yes}], name: second}]")
	if err != nil {
		t.Fatal(err)
	}
	second := r.Receivers[1]
	if second.Name != "second" || second.YAML != "- {future_configs: [{field: yes}], name: second}\n" {
		t.Fatalf("unexpected receiver: %+v", second)
	}
}

func TestRoutingStatusErrors(t *testing.T) {
	tests := []struct {
		body, want string
		code       int
	}{
		{`{}`, "missing config.original", 200},
		{`{"config":{"original":"route: [private-yaml"}}`, "configuration YAML", 200},
		{`{"config":{"original":"receivers: []"}}`, "missing root route", 200},
		{`{"config":{"original":"route: {receiver: default}"}}`, "missing receivers field", 200},
		{`{"config":{"original":"route: {receiver: default}\nreceivers: {}"}}`, "receivers must be a sequence", 200},
		{`{"config":{"original":"route: {receiver: default}\nreceivers: [{}]"}}`, "invalid or duplicate receiver", 200},
		{`private-body`, "malformed JSON", 200},
		{`private-body`, "HTTP 503", 503},
	}
	for _, test := range tests {
		t.Run(test.want, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(test.code)
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			_, err := (Client{Base: server.URL}).Routing(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.want) || strings.Contains(err.Error(), "private-") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
	r, err := parseRouting("route: {receiver: default}\nreceivers: []")
	if err != nil || len(r.Receivers) != 0 || len(r.Routes) != 1 {
		t.Fatalf("expected root-only snapshot without receivers: %v, %v", r, err)
	}
}
