package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"promtui/internal/editor"
)

func TestScopedMenuLayout(t *testing.T) {
	const expr = "node_cpu_seconds_total{instance=\""
	for _, size := range [][2]int{{24, 10}, {48, 14}, {110, 40}} {
		m := New(Options{}, nil)
		m.resize(size[0], size[1])
		m.editor.SetValue(expr)
		c := editor.At(expr, len(expr))
		k := cacheKey{c.Kind, c.Metric, c.Label, c.Prefix}
		m.cache = map[cacheKey]cached{k: {time.Now().Add(time.Minute), []string{"192.168.2.7"}}}
		m.analyzed(analysisMsg{0, context.Background(), c, editor.Diagnostic(expr), false})
		rows := strings.Split(ansi.Strip(m.screen()), "\n")
		if len(rows) != size[1] || !strings.Contains(rows[3], "╭") || !strings.Contains(rows[4], "instance values") || !strings.Contains(rows[5], "192.168.2.7") || !strings.Contains(rows[len(rows)-1], "Tab Accept") {
			t.Fatalf("%v:\n%s", size, strings.Join(rows, "\n"))
		}
		for _, row := range rows {
			if ansi.StringWidth(row) > size[0] {
				t.Fatalf("%v: overwide row %q", size, row)
			}
		}
		m.dismiss()
		if !strings.Contains(m.screen(), "Run a query") || !m.showDiagnostic() {
			t.Fatal("dismissing menu should restore the result panel and syntax feedback")
		}
	}
}
