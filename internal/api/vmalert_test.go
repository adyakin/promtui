package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const vmAlertFixture = `{
 "state":"firing","name":"HaproxyHasNoAliveBackends","value":"0",
 "labels":{"alertgroup":"haproxy","alertname":"HaproxyHasNoAliveBackends","instance":"S-NSK90-MS00143.t-global.bcs:7000","job":"consul-sd","proxy":"minio","servicename":"bcs-minio-clicard-01","severity":"critical","sourceservicename":"bcs-vic"},
 "activeAt":"2026-11-24T07:20:00+03:00","id":"4564276932026302966","rule_id":"3452656140727507818","group_id":"2485214638202814937",
 "expression":"haproxy_backend_active_servers{proxy!=\"stats\"} + haproxy_backend_backup_servers == 0"
}`

func TestResolveVMAlert(t *testing.T) {
	const ids = "?group_id=2485214638202814937&alert_id=4564276932026302966"
	bases := []string{"https://example.com/proxy/east", "https://example.com/proxy/west/vmalert/api/v1"}
	for _, tc := range []struct {
		url     string
		source  int
		wantErr string
	}{
		{"https://EXAMPLE.com:443/proxy/east/vmalert/alert" + ids, 0, ""},
		{"https://example.com/proxy/west/vmalert/alert" + ids, 1, ""},
		{"https://other.example/proxy/east/vmalert/alert" + ids, -1, "configured"},
		{"http://example.com/proxy/east/vmalert/alert" + ids, -1, "configured"},
		{"https://example.com/proxy/eastern/vmalert/alert" + ids, -1, "configured"},
		{"https://example.com/proxy/east/alert" + ids + "&alert_id=3", -1, "IDs"},
		{"https://example.com/proxy/east/alert?group_id=abc&alert_id=3", -1, "IDs"},
	} {
		ref, got, err := ResolveVMAlert(tc.url, bases)
		if got != tc.source || (tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr))) ||
			(tc.wantErr == "" && (err != nil || ref != (VMAlertRef{"2485214638202814937", "4564276932026302966"}))) {
			t.Fatalf("%s: %v %d %v", tc.url, ref, got, err)
		}
	}
	_, _, err := ResolveVMAlert("https://example.com/proxy/east/alert"+ids, append(bases, bases[0]))
	if err == nil || !strings.Contains(err.Error(), "Ambiguous") {
		t.Fatalf("duplicate sources: %v", err)
	}
}

func TestVMAlertAPI(t *testing.T) {
	ref := VMAlertRef{"2485214638202814937", "4564276932026302966"}
	for _, tc := range []struct {
		name, body, wantErr string
		code                int
	}{
		{"production fixture", vmAlertFixture, "", 200},
		{"missing fields", `{}`, "IDs", 200},
		{"numeric id", strings.Replace(vmAlertFixture, `"4564276932026302966"`, `4564276932026302966`, 1), "Invalid", 200},
		{"mismatched id", strings.Replace(vmAlertFixture, `"4564276932026302966"`, `"1"`, 1), "mismatched", 200},
		{"invalid time", strings.Replace(vmAlertFixture, `"2026-11-24T07:20:00+03:00"`, `"yesterday"`, 1), "Invalid", 200},
		{"resolved", "", "no longer available", 404},
		{"redirect refused", "", "HTTP 302", 302},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/proxy/vmalert/api/v1/alert" ||
					r.URL.Query().Get("group_id") != ref.GroupID || r.URL.Query().Get("alert_id") != ref.AlertID {
					t.Errorf("unexpected request: %v", r.URL)
				}
				w.Header().Set("Location", "/unexpected-redirect")
				w.WriteHeader(tc.code)
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			c := Client{Base: srv.URL + "/proxy/vmalert/api/v1", HTTP: srv.Client()}
			got, err := c.VMAlert(context.Background(), ref)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("got %v, want %s", err, tc.wantErr)
				}
				return
			}
			if err != nil || got.RuleID != "3452656140727507818" || !strings.Contains(got.Expression, "haproxy_backend_backup_servers == 0") ||
				got.ActiveAt.UTC().Format(time.RFC3339) != "2026-11-24T04:20:00Z" {
				t.Fatalf("got %+v %v", got, err)
			}
		})
	}
}
