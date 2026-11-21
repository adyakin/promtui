package config

import (
	"slices"
	"strings"
	"testing"
)

func TestRuleSources(t *testing.T) {
	const base = "connections:\n  local:\n    url: http://localhost:9090/prometheus\n    insecure_skip_verify: true\n"
	for _, tc := range []struct {
		name, extra, err string
		want             []RuleSource
	}{
		{name: "fallback", want: []RuleSource{{"local", "http://localhost:9090/prometheus"}}},
		{name: "one", extra: "    vmalert: [{name: east, url: https://east/vmalert}]\n", want: []RuleSource{{"east", "https://east/vmalert"}}},
		{name: "many", extra: "    vmalert: [{name: east, url: https://east}, {name: west, url: https://west}]\n", want: []RuleSource{{"east", "https://east"}, {"west", "https://west"}}},
		{name: "empty", extra: "    vmalert: []\n", err: "nonempty list"},
		{name: "null", extra: "    vmalert: null\n", err: "nonempty list"},
		{name: "duplicate", extra: "    vmalert: [{name: east, url: https://east}, {name: ' east ', url: https://west}]\n", err: "duplicate"},
		{name: "missing-name", extra: "    vmalert: [{url: https://east}]\n", err: "missing"},
		{name: "missing-url", extra: "    vmalert: [{name: east}]\n", err: "HTTP(S) base URL"},
		{name: "bad-url", extra: "    vmalert: [{name: east, url: 'https://alice:secret@east/path?token=secret'}]\n", err: "HTTP(S) base URL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := decode("config.yaml", []byte(base+tc.extra))
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) || strings.Contains(err.Error(), "secret") {
					t.Fatalf("error = %v", err)
				}
				return
			}
			if err != nil || len(c.Connections) != 1 {
				t.Fatal(c, err)
			}
			conn := c.Connections[0]
			if !conn.InsecureSkipVerify || !slices.Equal(conn.RuleSources(), tc.want) {
				t.Fatalf("connection = %+v, sources = %v", conn, conn.RuleSources())
			}
		})
	}
}
