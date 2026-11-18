package query

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"promtui/internal/api"
)

func TestRangeThroughClusterPrefix(t *testing.T) {
	end := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/select/0/prometheus/api/v1/query_range" || r.FormValue("query") != "rate(foo[5m])" || r.FormValue("start") != "2025-12-31T23:00:00Z" || r.FormValue("end") != "2026-01-01T00:00:00Z" || r.FormValue("step") != "15" || r.FormValue("limit") != "2" {
			t.Errorf("unexpected request: %s %s %v", r.Method, r.URL, r.Form)
		}
		fmt.Fprint(w, `{"status":"success","isPartial":true,"warnings":["delayed"],"data":{"resultType":"matrix","result":[{"metric":{"__name__":"foo","job":"api"},"values":[[1.25,"NaN"],[2,"+Inf"]]}]}}`)
	}))
	defer s.Close()
	res, err := Execute(t.Context(), api.Client{Base: s.URL + "/select/0/prometheus/api/v1/", HTTP: s.Client()}, Request{"rate(foo[5m])", end, time.Hour, 0, 2})
	if err != nil {
		t.Fatal(err)
	}
	row := res.Series[0]
	if row.Name != "foo" || row.Labels != `foo{job="api"}` || row.Samples[0].Timestamp != 1250 || row.Samples[1].Value.String() != "+Inf" || len(res.Notices) != 2 {
		t.Fatalf("%+v", res)
	}
}

func TestBackendFailures(t *testing.T) {
	for _, tc := range []struct {
		code       int
		body, want string
	}{
		{422, `{"status":"error","errorType":"bad_data","error":"parse error"}`, "parse error"},
		{503, "unavailable", "503"},
		{200, `{"status":"success","data":{"resultType":"matrix","result":[null]}}`, "null"},
		{200, `{"status":"success","data":{"resultType":"unknown"}}`, "unsupported"},
	} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.code); fmt.Fprint(w, tc.body) }))
		_, err := Execute(t.Context(), api.Client{Base: s.URL, HTTP: s.Client()}, Request{Limit: 10})
		s.Close()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("got %v, want %s", err, tc.want)
		}
	}
}
func TestLimitIgnoredByBackend(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		row := `{"metric":{},"value":[0,"1"]}`
		fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[%s%s]}}`, strings.Repeat(row+",", 9999), row)
	}))
	defer s.Close()
	out, err := Execute(t.Context(), api.Client{Base: s.URL, HTTP: s.Client()}, Request{Limit: 5})
	if err != nil || len(out.Series) != 5 || len(out.Notices) == 0 {
		t.Fatalf("rows=%d notices=%v error=%v", len(out.Series), out.Notices, err)
	}
}
func TestDeadline(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { r.ParseForm(); <-r.Context().Done() }))
	defer s.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	_, err := Execute(ctx, api.Client{Base: s.URL, HTTP: s.Client()}, Request{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

func TestLivePrometheus(t *testing.T) {
	base := os.Getenv("PROMTUI_TEST_URL")
	if base == "" {
		t.Skip("set PROMTUI_TEST_URL for a live Prometheus-compatible server")
	}
	for _, tc := range []struct {
		expr   string
		window time.Duration
		rows   int
		value  string
	}{
		{"1+2", 0, 1, "3"}, {`"hello"`, 0, 1, "hello"}, {"vector(5)", 0, 1, "5"}, {"vector(5)", time.Minute, 1, "5"}, {"vector(0) > 1", 0, 0, ""}, {"sum(", 0, -1, ""},
	} {
		res, err := Execute(t.Context(), api.Client{Base: base, HTTP: &http.Client{Timeout: 3 * time.Second}}, Request{tc.expr, time.Now(), tc.window, time.Second, 10})
		if tc.rows < 0 {
			if err == nil {
				t.Error("invalid query accepted")
			}
			continue
		}
		if err != nil || len(res.Series) != tc.rows {
			t.Fatalf("%s: %v, rows=%d", tc.expr, err, len(res.Series))
		}
		if tc.rows == 0 {
			continue
		}
		row := res.Series[0]
		value := row.Samples[0].Value.String()
		if row.Text != nil {
			value = *row.Text
		}
		if value != tc.value || (tc.window > 0 && len(row.Samples) != 61) {
			t.Errorf("%s: %+v", tc.expr, row)
		}
	}
}
