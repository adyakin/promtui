package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestRulesCompatibility(t *testing.T) {
	for _, backend := range []string{"prometheus", "vmalert"} {
		t.Run(backend, func(t *testing.T) {
			fixture, err := os.ReadFile("testdata/rules_" + backend + ".json")
			if err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.RequestURI() != "/prefix/api/v1/rules" {
					t.Errorf("request = %s %s", r.Method, r.URL.RequestURI())
				}
				w.Write(fixture)
			}))
			defer srv.Close()
			base := srv.URL + "/prefix"
			if backend == "vmalert" {
				base += "/api/v1/"
			}
			got, err := (Client{base, srv.Client()}).Rules(context.Background())
			if err != nil || got.Warning != "" || len(got.Groups) == 0 {
				t.Fatalf("Rules = %+v, %v", got, err)
			}
			g := got.Groups[0]
			if g.Interval == nil || g.LastEvaluation.IsZero() || len(g.Rules) < 2 {
				t.Fatalf("group = %+v", g)
			}
			a, r := g.Rules[0], g.Rules[1]
			if a.Type != AlertingRule || r.Type != RecordingRule || a.Position != 0 || r.Position != 1 ||
				a.Health != "ok" || a.Expression == "" || len(a.Alerts) == 0 || len(a.Annotations) == 0 ||
				len(a.Labels) == 0 || a.For == nil || *a.For != 300 {
				t.Fatalf("rules = %+v", g.Rules)
			}
			instance := a.Alerts[0]
			if instance.ActiveAt.IsZero() || instance.Value == "" || len(instance.Annotations) == 0 {
				t.Fatalf("instance = %+v", instance)
			}
			if backend == "prometheus" {
				if len(got.Groups) != 2 || len(got.Groups[1].Rules) != 0 || g.EvaluationTime == nil ||
					g.Limit == nil || *g.Limit != 0 || a.State != "firing" || len(a.Alerts) != 2 ||
					a.Alerts[1].State != "pending" || instance.KeepFiringSince.IsZero() ||
					a.KeepFiringFor == nil || *a.KeepFiringFor != 60 || r.LastError != "evaluation timed out" ||
					r.Health != "err" || g.Rules[2].State != "inactive" || g.Rules[2].For != nil ||
					a.Expression != "sum by (instance) (\n  up == 0\n)" {
					t.Fatalf("Prometheus data = %+v", got)
				}
			} else if g.ID != "18000000000000000001" || g.Limit != nil || g.EvaluationTime != nil ||
				g.Concurrency == nil || *g.Concurrency != 2 || g.EvalDelay == nil || *g.EvalDelay != 10 ||
				a.State != "pending" || a.KeepFiringFor == nil || *a.KeepFiringFor != 120 ||
				a.LastSeriesFetched == nil || *a.LastSeriesFetched != 2 ||
				instance.ID != "98" || instance.Restored == nil || !*instance.Restored {
				t.Fatalf("vmalert data = %+v", got)
			}
		})
	}
}

func TestRulesPartialJSONAndSafeErrors(t *testing.T) {
	got, err := decodeRules([]byte(`{"status":"success","data":{"groups":[null,
		{"name":"g", "lastEvaluation":"invalid", "rules":[false, {"name":"new","type":"future",
		"state":"recovering","health":42,"query":"up","lastError":"GET https://id:secret@host/path?token=hidden failed\nAuthorization: Bearer secret"}]}]}}`))
	if err != nil || got.Warning == "" || len(got.Groups) != 1 || len(got.Groups[0].Rules) != 1 {
		t.Fatalf("partial response = %+v, %v", got, err)
	}
	r := got.Groups[0].Rules[0]
	if r.Position != 1 || r.Type != "future" || r.State != "recovering" || r.Expression != "up" ||
		r.LastError != "GET [URL redacted] failed\nAuthorization: [redacted]" || !got.Groups[0].LastEvaluation.IsZero() {
		t.Fatalf("partial rule = %+v", r)
	}
	for _, tc := range []struct {
		status     int
		body, want string
	}{
		{200, `{"status":"error","error":"token=hidden"}`, "unsuccessful"},
		{200, `{"status":"success","data":{}}`, "missing rule groups"},
		{200, `{"status":"success","data":{"groups":[{}]}}`, "no readable"},
		{200, `<html>token=hidden</html>`, "invalid"},
		{503, `token=hidden`, "HTTP 503"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			_, err := (Client{srv.URL, srv.Client()}).Rules(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "hidden") {
				t.Fatalf("safe error = %v", err)
			}
		})
	}
}
