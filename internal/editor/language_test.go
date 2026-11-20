package editor

import (
	"slices"
	"strings"
	"testing"
)

func TestMetricPhrase(t *testing.T) {
	const phrase = "node  cpu tot"
	s := "up or rate(" + phrase + "[5m])"
	c := At(s, strings.IndexByte(s, '['))
	values := c.Filter([]string{"node_cpu_guest_seconds_total", "node_total_cpu", "node_cpu_seconds_total"})
	if c.Prefix != phrase || !slices.Equal(values, []string{"node_cpu_seconds_total", "node_cpu_guest_seconds_total"}) || s[:c.Start]+values[0]+s[c.End:] != "up or rate(node_cpu_seconds_total[5m])" {
		t.Fatal(c, values)
	}
	for _, s := range []string{"up or node", "up + node", "up\nnode"} {
		if c := At(s, len(s)); c.Prefix != "node" {
			t.Fatal(s, c)
		}
	}
}

func TestMetricPhraseAfterTrailingSpace(t *testing.T) {
	const phrase = "node cpu load"
	const suffix = " \t[5m])"
	s := "rate(" + phrase + suffix
	c := At(s, strings.IndexByte(s, '['))
	got := c.Filter([]string{"node_cpu_load_average", "node_load1", "node_load_cpu"})
	if c.Prefix != phrase || !slices.Equal(got, []string{"node_cpu_load_average"}) ||
		s[:c.Start]+got[0]+s[c.End:] != "rate(node_cpu_load_average"+suffix {
		t.Fatal(c, got)
	}
	for _, input := range []string{"node ", "up or node ", "node cpu\n", "node cpu # note "} {
		if c := At(input, len(input)); c.Prefix != "" {
			t.Fatalf("unexpected phrase for %q: %+v", input, c)
		}
	}
}

func TestIncompleteContext(t *testing.T) {
	for _, tc := range []struct{ input, kind, metric, label, prefix string }{
		{`sum(rate(nod|e_cpu[5m]))`, "expr", "", "", "nod"},
		{`sum by (inst|ance) (up)`, "label", "", "", "inst"},
		{`rate(cpu{path="a},b", inst|ance="x"}[5m])`, "label", "cpu", "", "inst"},
		{`cpu{instance=~"сер|вер"}`, "value", "cpu", "instance", "сер"},
		{`cpu{instance="|"}`, "value", "cpu", "instance", ""},
		{`cpu{job="a\"|`, "value", "cpu", "job", `a\"`},
		{`rate(cpu[5|])`, "duration", "", "", "5"},
		{"up # cpu{inst|", "", "", "", ""},
		{`label_replace(up, "a|", "", "", "")`, "", "", "", ""},
	} {
		i := strings.IndexByte(tc.input, '|')
		s := strings.Replace(tc.input, "|", "", 1)
		c := At(s, i)
		if c.Kind != tc.kind || c.Metric != tc.metric || c.Label != tc.label || c.Prefix != tc.prefix {
			t.Errorf("%s: %+v", tc.input, c)
		}
	}
}
func TestBracketInsideStringIgnored(t *testing.T) {
	s := `sum(up{label=")"})`
	colors := styles(s, 3)
	if colors[3] != 14 || colors[len(s)-1] != 14 || colors[strings.Index(s, `")`)+1] == 14 {
		t.Fatal(colors)
	}
	if Diagnostic(s) != "" || !strings.Contains(Diagnostic("sum(\nup"), "parse error") {
		t.Fatal("missing diagnostic")
	}
}
