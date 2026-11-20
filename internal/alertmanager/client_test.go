package alertmanager

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

func TestAlertsResponseAndProxyPrefix(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/proxy/alertmanager/api/v2/alerts" || r.Header.Get("Accept") != "application/json" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		q := r.URL.Query()
		if len(q) != 4 || q.Get("active") != "true" || q.Get("inhibited") != "true" ||
			q.Get("silenced") != "true" || q.Get("unprocessed") != "true" {
			t.Errorf("unexpected query: %v", q)
		}
		fmt.Fprint(w, `[{
			"fingerprint":"abc", "labels":{"alertname":"Down"}, "annotations":{"summary":"Down"},
			"startsAt":"2026-01-01T02:00:00Z", "endsAt":"2026-01-01T03:00:00Z", "updatedAt":"2026-01-01T02:20:00Z",
			"generatorURL":"https://example.test/graph", "receivers":[{"name":"email"}],
			"status":{"state":"suppressed","silencedBy":["silence"],"inhibitedBy":["source"]}
		},{}]`)
	}))
	defer s.Close()
	rows, err := (Client{Base: s.URL + "/proxy/alertmanager/", HTTP: s.Client()}).Alerts(t.Context())
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	a := rows[0]
	if a.Fingerprint != "abc" || a.Labels["alertname"] != "Down" || a.Annotations["summary"] != "Down" ||
		a.StartsAt.Hour() != 2 || a.EndsAt.Hour() != 3 || a.UpdatedAt.Minute() != 20 ||
		a.GeneratorURL != "https://example.test/graph" || len(a.Receivers) != 1 || a.Receivers[0].Name != "email" ||
		a.Status.State != "suppressed" || strings.Join(a.Status.SilencedBy, "") != "silence" ||
		strings.Join(a.Status.InhibitedBy, "") != "source" {
		t.Fatalf("unexpected decoded alert: %+v", a)
	}
	if b := rows[1]; b.Labels != nil || !b.StartsAt.IsZero() || b.Status.State != "" {
		t.Fatalf("unexpected defaults: %+v", b)
	}
}

func TestAlertsHTTPResults(t *testing.T) {
	for _, tt := range []struct {
		name  string
		code  int
		body  string
		err   error
		match string
	}{
		{"empty", 200, "[]", nil, ""},
		{"malformed", 200, "[", nil, "invalid Alertmanager response"},
		{"null", 200, "null", nil, "invalid Alertmanager response"},
		{"failed", 503, "private response", nil, "HTTP 503"},
		{"unauthorized", 401, "private response", ErrAuth, "HTTP 401"},
		{"forbidden", 403, "private response", ErrAuth, "HTTP 403"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.code)
				fmt.Fprint(w, tt.body)
			}))
			defer s.Close()
			rows, err := (Client{Base: s.URL}).Alerts(t.Context())
			if tt.match == "" {
				if err != nil || rows == nil || len(rows) != 0 {
					t.Fatalf("expected a successful empty list: %v, %v", rows, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.match) ||
				strings.Contains(err.Error(), "private response") || tt.err != nil && !errors.Is(err, tt.err) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestAlertsCancellationAndTimeout(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		started := make(chan struct{})
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(started)
			<-r.Context().Done()
		}))
		ctx, cancel := context.WithCancel(t.Context())
		want := context.Canceled
		if timeout {
			cancel()
			ctx, cancel = context.WithTimeout(t.Context(), 50*time.Millisecond)
			want = context.DeadlineExceeded
		} else {
			go func() {
				select {
				case <-started:
				case <-ctx.Done():
				}
				cancel()
			}()
		}
		_, err := (Client{Base: s.URL}).Alerts(ctx)
		cancel()
		s.Close()
		if !errors.Is(err, want) {
			t.Fatalf("timeout=%v: expected %v; got %v", timeout, want, err)
		}
	}
}

func TestInvalidEndpointDoesNotExposeCredentials(t *testing.T) {
	for _, base := range []string{"", "ftp://host", "https://user:secret@host", "https://host?token=secret", "https://host#secret", "https://%secret"} {
		if _, err := (Client{Base: base}).Alerts(t.Context()); !errors.Is(err, ErrConfig) || strings.Contains(err.Error(), "secret") {
			t.Fatalf("unexpected config error: %v", err)
		}
	}
}
