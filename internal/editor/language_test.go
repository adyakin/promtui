package editor

import (
	"strings"
	"testing"
)

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
