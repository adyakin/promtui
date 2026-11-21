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

func TestRoutingDestinations(t *testing.T) {
	r, err := parseRouting(`
route: {receiver: default}
receivers:
  - name: default
    webhook_configs:
      - url: https://private-user:private-password@hooks.example.org:8080/private-path?token=private-query#private-fragment
        http_config: {authorization: {credentials: private-authorization}}
      - url_file: /private-file
    slack_configs:
      - {channel: "#ops", api_url: https://example.org/private-token}
      - {channel: '<secret>'}
    telegram_configs: [{chat_id: -100123, bot_token: private-bot}]
    email_configs:
      - {to: ops@example.org, auth_password: private-email}
      - {to: "{{ private-template }}"}
    future_configs: [{destination: private-unknown}]
`)
	if err != nil {
		t.Fatal(err)
	}
	list := r.Receivers[0].Integrations
	want := []Integration{
		{"Email", "To", "ops@example.org"}, {"Email", "To", ""}, {"future", "", ""},
		{"Slack", "Channel", "#ops"}, {"Slack", "Channel", ""}, {"Telegram", "Chat ID", "-100123"},
		{"Webhook", "Host", "hooks.example.org:8080"}, {"Webhook", "Host", ""},
	}
	if !slices.Equal(list, want) || strings.Contains(fmt.Sprint(list), "private-") {
		t.Fatalf("unexpected safe model: %+v", list)
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
