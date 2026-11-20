package app

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/prometheus/common/model"
	"promtui/internal/editor"
	"promtui/internal/graph"
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
		if !strings.Contains(m.screen(), "Run with Ctrl+Enter") || !m.showDiagnostic() {
			t.Fatal("dismissing menu should restore the result panel and syntax feedback")
		}
	}
}

func TestNativeCursorFollowsVisibleQueryPosition(t *testing.T) {
	m := New(Options{}, nil)
	m.resize(minWidth, minHeight)
	m.editor.SetValue("up\n" + strings.Repeat("界", 30))
	v := m.View()
	if v.Cursor == nil || v.Cursor.Y != 2 || v.Cursor.X != m.width-3 {
		t.Fatalf("cursor did not follow the scrolled query: %+v", v.Cursor)
	}
	queryRow := strings.Split(v.Content, "\n")[2]
	if strings.Contains(queryRow, "\x1b[7m") {
		t.Fatal("the editor still paints a block cursor")
	}
	m.editor.SetCursorColumn(1)
	if c := m.View().Cursor; c == nil || c.X != 9 || c.Y != 2 {
		t.Fatalf("cursor did not account for a wide character: %+v", c)
	}
	m.popup = true
	if m.View().Cursor != nil {
		t.Fatal("native cursor visible through the completion overlay")
	}
	press(m, tea.KeyEsc, tea.KeyTab)
	if m.View().Cursor != nil {
		t.Fatal("native cursor visible while Results is focused")
	}
	press(m, tea.KeyTab)
	if c := m.View().Cursor; c == nil || c.X != 9 || c.Y != 2 {
		t.Fatalf("query focus did not restore the native cursor: %+v", c)
	}
}

func TestFloatingDialogsFitAndKeepWorkspace(t *testing.T) {
	for _, size := range [][2]int{{24, 10}, {48, 14}, {100, 30}} {
		m := New(Options{Graphics: true, Ranged: true, Window: time.Hour}, nil)
		m.resize(size[0], size[1])
		m.editor.SetValue("up")
		r := sampleResult(true)
		for range 20 {
			r.Series = append(r.Series, r.Series[0])
		}
		m.Update(completed{result: r})
		press(m, tea.KeyTab)
		w := m.workspace()
		check := func(title string) {
			t.Helper()
			screen := ansi.Strip(m.screen())
			rows := strings.Split(screen, "\n")
			d := m.dialogBounds(w)
			if len(rows) != size[1] || !strings.Contains(rows[0], "PromTUI") ||
				!strings.Contains(rows[d.y], title) || !strings.Contains(rows[len(rows)-1], "Esc Back") {
				t.Fatalf("%v:\n%s", size, screen)
			}
			for i, line := range rows {
				if ansi.StringWidth(line) > size[0] {
					t.Fatalf("%v: overwide row %q", size, line)
				}
				part := ansi.Cut(line, d.x, d.x+d.width)
				switch {
				case i == d.y:
					if !strings.HasPrefix(part, "╭") || !strings.HasSuffix(part, "╮") {
						t.Fatal(part)
					}
				case i == d.y+d.height-1:
					if !strings.HasPrefix(part, "╰") || !strings.HasSuffix(part, "╯") {
						t.Fatal(part)
					}
				case i > d.y && i < d.y+d.height-1:
					if !strings.HasPrefix(part, "│") || !strings.HasSuffix(part, "│") {
						t.Fatal(part)
					}
				}
			}
			footer := m.footer()
			if ansi.StringWidth(footer) != size[0] || strings.Contains(footer, "\x1b[7m") || strings.Contains(footer, "\x1b[48;") {
				t.Fatalf("unexpected footer styling: %q", footer)
			}
		}
		press(m, 's', tea.KeyDown, tea.KeySpace)
		check("Series selector")
		swatch := lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("#465972")).
			Foreground(lipgloss.Color(graph.Color(1))).Render("■")
		if !strings.Contains(ansi.Strip(m.screen()), "› ■ [ ]") ||
			!strings.Contains(m.dialogLayer(w).GetContent(), swatch) {
			t.Fatal("selector lacks the selected checkbox or colored swatches")
		}
		press(m, '/', tea.KeyTab)
		m.Update(tea.PasteMsg{Content: "no matches expected"})
		check("Series selector")
		press(m, tea.KeyEsc, 'r')
		check("Time range")
		if d := m.dialogBounds(w); d.x != (m.width-d.width)/2 ||
			d.y != w.resultY+(w.bottomY-w.resultY+1-d.height)/2 {
			t.Fatal("range dialog is not centered")
		}
		press(m, tea.KeyEsc, 'l')
		if !strings.Contains(ansi.Strip(m.screen()), "Series labels") ||
			!strings.Contains(ansi.Strip(m.footer()), "Esc Close") {
			t.Fatal("labels inspector is not floating")
		}
	}
}

func TestLabelsInspectorScrollsAndKeepsUnabridgedLabels(t *testing.T) {
	m := New(Options{}, nil)
	m.resize(48, 14)
	r := sampleResult(false)
	metric := model.Metric{
		"instance": "example", "job": "node-exporter", "cluster": "production",
		"long":   model.LabelValue(strings.Repeat("quoted, \"value\" ", 20)),
		"region": "west", "zone": "one", "environment": "development",
	}
	r.Series[1].Metric, r.Series[1].Labels = metric, metric.String()
	m.Update(completed{result: r})
	press(m, tea.KeyTab, tea.KeyDown, 'l')
	if m.labelText != metric.String() || len(m.labelFields) != len(metric) ||
		m.labelFields[0].name != "cluster" || m.labelFields[6].name != "zone" {
		t.Fatal("labels were lost, reformatted for copying, or not sorted")
	}
	if !strings.Contains(ansi.Strip(m.screen()), "cluster") ||
		!strings.Contains(ansi.Strip(m.footer()), "y Copy labels") {
		t.Fatal(m.screen())
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil || m.scope() != labelKeys || m.table.Row != 1 {
		t.Fatal("copy unavailable or changed result selection")
	}
	press(m, tea.KeyPgDown, tea.KeyPgDown, tea.KeyPgDown)
	if !strings.Contains(ansi.Strip(m.screen()), "› zone") {
		t.Fatal("could not scroll to the last label")
	}
	press(m, tea.KeyEsc)
	if m.scope() != tableKeys || m.table.Row != 1 {
		t.Fatal("closing the inspector changed Results focus or selection")
	}
}

func TestGraphLegendAndFooterFitSmallScreen(t *testing.T) {
	m := New(Options{Graphics: true, Window: 6 * time.Hour, Ranged: true}, nil)
	m.resize(24, 10)
	m.editor.SetValue("up\nup")
	m.diagnostic, m.notice = "diagnostic", "notice"
	result := sampleResult(true)
	result.Notices = []string{"notice"}
	m.Update(completed{result: result})
	rows := strings.Split(ansi.Strip(m.screen()), "\n")
	if len(rows) != 10 || !strings.Contains(rows[0], "UTC") || !strings.Contains(rows[6], "› 1/2") || !strings.Contains(rows[9], "^Enter Run") {
		t.Fatal(strings.Join(rows, "\n"))
	}
	for _, row := range rows {
		if ansi.StringWidth(row) > 24 {
			t.Fatal(row)
		}
	}
}
