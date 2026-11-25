package query

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"promtui/internal/api"
)

func TestAlertThreshold(t *testing.T) {
	for _, tc := range []struct {
		expr, want string
		vm         bool
	}{
		{`up == 0`, `up`, false},
		{`up`, `up`, false},
		{`-2 < (up + 1)`, `(up + 1)`, false},
		{`rate(foo[5m] offset 1h) > 100`, `rate(foo[5m] offset 1h)`, false},
		{`max_over_time(rate(foo[5m])[1h:1m]) >= bool 3`, `max_over_time(rate(foo[5m])[1h:1m])`, false},
		{`foo > on(job) group_left(pod) bar`, `foo > on(job) group_left(pod) bar`, false},
		{`(foo > 5) or (bar < 1)`, `(foo > 5) or (bar < 1)`, false},
		{`abs(foo > 5)`, `abs(foo > 5)`, false},
		{`foo > scalar(sum(bar))`, `foo > scalar(sum(bar))`, false},
		{`WITH (f = rate(foo[5m])) f > 2`, `rate(foo[5m])`, true},
		{`WITH (f = rate(foo[5m])) f > 2`, `WITH (f = rate(foo[5m])) f > 2`, false},
		{`foo nonsense`, `foo nonsense`, false},
	} {
		got, removed, _, _ := alertExpression(tc.expr, tc.vm)
		if got != tc.want || removed != (tc.expr != tc.want) {
			t.Errorf("%s -> %q (%v), want %q", tc.expr, got, removed, tc.want)
		}
	}
}

func TestAlertOutputScopingRejectsAmbiguity(t *testing.T) {
	labels := map[string]string{"job": "nodes", "instance": "target"}
	for _, vector := range []model.Vector{
		{{Metric: model.Metric{"job": "nodes"}}, {Metric: model.Metric{"job": "nodes", "instance": "other"}}},
		{{Metric: model.Metric{"job": "nodes", "instance": "target", "device": "sda"}},
			{Metric: model.Metric{"job": "nodes", "instance": "target", "device": "sdb"}}},
	} {
		if got := alertOutputLabels(vector, labels); got != nil {
			t.Fatalf("unexpected scope %v for %v", got, vector)
		}
	}
}

func TestAlertGraphRetriesCappedProbeWithIdentity(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			sample := `{"metric":{"instance":"another"},"value":[100,"1"]}`
			fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[%s]}}`, strings.Repeat(sample+",", alertProbeLimit-1)+sample)
			return
		}
		if !strings.Contains(r.FormValue("query"), `and on (instance)`) {
			t.Errorf("retry did not narrow the output probe: %s", r.FormValue("query"))
		}
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"instance":"target","site":"my-site"},"value":[100,"1"]}]}}`)
	}))
	defer srv.Close()
	graph := BuildAlertGraph(context.Background(), api.Client{Base: srv.URL, HTTP: srv.Client()}, `up == 0`,
		map[string]string{"instance": "target", "site": "my-site"}, time.Unix(100, 0), false)
	if !strings.Contains(graph.Expression, `instance="target",site="my-site"`) {
		t.Fatal(graph)
	}
}

func TestAlertGraphScoping(t *testing.T) {
	const haproxy = `haproxy_backend_active_servers{proxy!="stats"} + haproxy_backend_backup_servers`
	for _, tc := range []struct {
		name, input, samples string
		labels               map[string]string
		want                 []string
		reject               []string
	}{
		{"selector", `up{job="node",instance!="other"} == 0`,
			`{"job":"node","instance":"target"}`,
			map[string]string{"job": "node", "instance": "target", "external": "unverified"},
			[]string{`job="node"`, `instance!="other"`, `instance="target"`}, []string{"external", `== 0`, `and on`}},
		{"HAProxy", haproxy + ` == 0`,
			`{"proxy":"minio","instance":"node.example:7000","servicename":"minio-01"}`,
			map[string]string{"proxy": "minio", "instance": "node.example:7000", "servicename": "minio-01"},
			[]string{haproxy, `and on (instance, proxy, servicename)`, `"minio-01"`}, []string{`== 0`}},
		{"aggregation", `sum by (servicename) (rate(http_requests_total[5m])) > 100`,
			`{"servicename":"svc"}`, map[string]string{"servicename": "svc", "instance": "node"},
			[]string{`sum by (servicename) (rate(http_requests_total[5m]))`, `and on (servicename)`}, []string{`instance`, `> 100`}},
		{"group_left and label manipulation", `label_replace(foo + on(job) group_left(team) bar, "service", "$1", "job", "(.*)") > 3`,
			`{"service":"a.$b","team":"test"}`, map[string]string{"service": "a.$b", "team": "test", "pod": "irrelevant"},
			[]string{`group_left`, `and on (service, team)`, `"a.$$b"`}, []string{`pod`, `> 3`}},
		{"conflicting", `up{instance="other"} == 0`, `{"instance":"other"}`,
			map[string]string{"instance": "target"}, []string{`up{instance="other"}`}, []string{"target"}},
		{"missing", `up == 0`, `{"job":"node"}`, map[string]string{"instance": "target"}, []string{`up`}, []string{"target", "=="}},
		{"escaping", `up`, `{"instance":"x\"\\\ny"}`, map[string]string{"instance": "x\"\\\ny"},
			[]string{`instance="x\"\\\ny"`}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at := time.Unix(100, 0)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/query" || r.FormValue("time") != at.UTC().Format(time.RFC3339Nano) {
					t.Errorf("unexpected probe: %s %v", r.URL, r.Form)
				}
				fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":%s,"value":[100,"1"]}]}}`, tc.samples)
			}))
			defer srv.Close()
			got := BuildAlertGraph(context.Background(), api.Client{Base: srv.URL, HTTP: srv.Client()}, tc.input, tc.labels, at, true)
			for _, want := range tc.want {
				if !strings.Contains(got.Expression, want) {
					t.Errorf("expected %q in %+v", want, got)
				}
			}
			for _, reject := range tc.reject {
				if strings.Contains(got.Expression, reject) {
					t.Errorf("unexpected %q in %+v", reject, got)
				}
			}
		})
	}
}
