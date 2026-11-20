package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTargetsAPI(t *testing.T) {
	for _, tc := range []struct {
		name, prefix, body string
		status             int
		wantErr            error
	}{
		{"Prometheus", "/proxy", `{"status":"success","data":{"activeTargets":[{"discoveredLabels":{"__address__":"a"},"labels":{"job":"prom"},"scrapePool":"prom","scrapeUrl":"http://a/metrics","globalUrl":"http://a.example/metrics","lastError":"failed","lastScrape":"2026-01-01T12:00:00Z","lastScrapeDuration":0.04,"health":"down","scrapeInterval":"30s","scrapeTimeout":"10s"}],"droppedTargets":[{"discoveredLabels":{"__address__":"b"}}]}}`, 200, nil},
		{"vmagent", "/proxy/api/v1", `{"status":"success","data":{"activeTargets":[{"labels":{"instance":"agent"},"health":"up"}],"droppedTargets":[]}}`, 200, nil},
		{"malformed", "/proxy", `{`, 200, nil},
		{"missing data", "/proxy", `{"status":"success"}`, 200, nil},
		{"error envelope", "/proxy", `{"status":"error","error":"server detail"}`, 200, nil},
		{"not found", "/proxy", "", 404, ErrTargetsUnsupported},
		{"HTML fallback", "/proxy", "<!doctype html>", 200, ErrTargetsUnsupported},
		{"unauthorized", "/proxy", "", 401, ErrTargetsAuth},
		{"forbidden", "/proxy", "", 403, ErrTargetsAuth},
		{"server error", "/proxy", "", 503, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/proxy/api/v1/targets" || r.URL.RawQuery != "state=any" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer s.Close()
			out, err := (Client{s.URL + tc.prefix + "/", s.Client()}).Targets(t.Context())
			switch tc.name {
			case "Prometheus":
				if err != nil || len(out.Active) != 1 || len(out.Dropped) != 1 {
					t.Fatalf("data=%+v, error=%v", out, err)
				}
				a, d := out.Active[0], out.Dropped[0]
				if a.Labels["job"] != "prom" || a.DiscoveredLabels["__address__"] != "a" || a.Health != "down" ||
					a.LastError != "failed" || a.LastScrape != "2026-01-01T12:00:00Z" || a.LastScrapeDuration == nil ||
					*a.LastScrapeDuration != 0.04 || a.ScrapePool != "prom" || a.ScrapeURL != "http://a/metrics" ||
					a.GlobalURL != "http://a.example/metrics" || a.ScrapeInterval != "30s" || a.ScrapeTimeout != "10s" ||
					d.DiscoveredLabels["__address__"] != "b" || d.Health != "" {
					t.Fatalf("active=%+v, dropped=%+v", a, d)
				}
			case "vmagent":
				if err != nil || len(out.Active) != 1 || out.Active[0].Labels["instance"] != "agent" ||
					out.Active[0].LastScrapeDuration != nil || len(out.Dropped) != 0 {
					t.Fatalf("data=%+v, error=%v", out, err)
				}
			default:
				if err == nil || tc.wantErr != nil && !errors.Is(err, tc.wantErr) ||
					tc.name == "error envelope" && !strings.Contains(err.Error(), "server detail") {
					t.Fatalf("error = %v (want %v)", err, tc.wantErr)
				}
			}
		})
	}
}

func TestTargetsCancellation(t *testing.T) {
	started := make(chan struct{}, 1)
	s := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-r.Context().Done()
	}))
	defer s.Close()
	c := Client{s.URL, s.Client()}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go func() {
		select {
		case <-started:
			cancel()
		case <-ctx.Done():
		}
	}()
	if _, err := c.Targets(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error = %v", err)
	}
	ctx, deadline := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer deadline()
	if _, err := c.Targets(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v", err)
	}
}
