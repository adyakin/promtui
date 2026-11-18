package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestScopedSuggestions(t *testing.T) {
	for _, tc := range []struct{ kind, metric, label, prefix, path, selector string }{
		{"expr", "", "", "node_", "/label/__name__/values", `{__name__=~"node_.*"}`},
		{"label", "up", "", "jo", "/labels", "up"},
		{"value", "up", "instance", "192.", "/label/instance/values", `up{instance=~"192\\..*"}`},
	} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/tenant/api/v1"+tc.path || r.URL.Query().Get("match[]") != tc.selector || r.URL.Query().Get("limit") != "80" {
				t.Error(r.URL)
			}
			fmt.Fprint(w, `{"status":"success","data":["abc"]}`)
		}))
		got, err := (Client{s.URL + "/tenant", s.Client()}).Suggestions(t.Context(), tc.kind, tc.metric, tc.label, tc.prefix)
		s.Close()
		if err != nil || !slices.Equal(got, []string{"abc"}) {
			t.Fatal(got, err)
		}
	}
	// A nil HTTP transport ensures that these cases never perform an HTTP call.
	for _, kind := range []string{"expr", "value", "duration"} {
		v, err := (Client{}).Suggestions(t.Context(), kind, "", "job", "")
		if err != nil || len(v) != 0 {
			t.Fatal(kind, v, err)
		}
	}
}
