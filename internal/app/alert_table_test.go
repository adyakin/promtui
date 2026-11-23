package app

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/config"
)

func TestAlertTableFixedAndWeightedWidths(t *testing.T) {
	a := newAlertsState(config.TableView{Columns: []config.Column{
		{Field: "state", Width: 10},
		{Field: "labels.path", Title: "Path", Weight: 3},
		{Field: "annotations.summary", Title: "Info", Weight: 1},
	}})
	now := time.Now()
	for _, tc := range []struct {
		width int
		want  []int
	}{
		{64, []int{10, 34, 14}},
		{84, []int{10, 49, 19}},
	} {
		table := a.tableLayout(tc.width, now)
		if got := alertTableWidths(table); !slices.Equal(got, tc.want) {
			t.Errorf("width %d: got %v, want %v", tc.width, got, tc.want)
		}
	}
	a.view.Columns = []config.Column{{Field: "state", Width: 10}, {Field: "duration", Width: 8}}
	table := a.tableLayout(80, now)
	if got := alertTableWidths(table); !slices.Equal(got, []int{10, 8}) || table.trailing != 58 {
		t.Errorf("fixed-only table: widths %v, trailing %d", got, table.trailing)
	}
}

func TestAlertTableSamplingUsesLoadedRows(t *testing.T) {
	a := newAlertsState(config.TableView{Columns: []config.Column{
		{Field: "labels.short", Title: "Name"},
		{Field: "annotations.summary", Title: "Info"},
	}})
	now := time.Now()
	for i := range 2 * alertTableSamples {
		row := testAlert(fmt.Sprint(i), "Alert", "", "active", now)
		row.Labels["short"] = "short"
		summary := "short"
		if i >= alertTableSamples {
			summary = strings.Repeat("界", 11) // 22 cells, not 33 bytes.
		}
		row.Annotations = map[string]string{"summary": summary}
		a.rows = append(a.rows, newAlertRow("test", row))
	}
	a.visible = slices.Clone(a.rows[:3])
	columns := a.view.Columns
	if got := alertTableContentWidth(columns[1], a.rows, now); got != 22 {
		t.Fatalf("sampled width = %d, want 22 cells", got)
	}
	measured := alertTableWidths(a.tableLayout(88, now))
	if measured[1] <= measured[0] {
		t.Fatalf("a wider typical value should get more room: %v", measured)
	}
	a.visible = slices.Clone(a.rows[len(a.rows)-3:])
	slices.Reverse(a.visible)
	a.row = 2
	if got := alertTableWidths(a.tableLayout(88, now.Add(time.Hour))); !slices.Equal(got, measured) {
		t.Fatalf("visible rows, selection and time changed widths: %v, want %v", got, measured)
	}
	// The last row is always sampled; one huge value has no effect on the
	// typical width even if visible.
	a.rows[len(a.rows)-1].Annotations["summary"] = strings.Repeat("界", 5000)
	if got := alertTableWidths(a.tableLayout(88, now)); !slices.Equal(got, measured) {
		t.Fatalf("annotation outlier changed widths: %v, want %v", got, measured)
	}
}

func TestAlertTableRendersExtractedValuesWithoutChangingSortOrSearch(t *testing.T) {
	now := time.Now()
	one := testAlert("one", "Cert", "critical", "suppressed", now)
	one.Status.SilencedBy, one.Status.InhibitedBy = []string{"silence"}, []string{"inhibit"}
	one.Annotations = map[string]string{"summary": "a certificate expires in 9 days."}
	one.Labels["path"] = "/certs/界"
	two := testAlert("two", "Cert", "high", "active", now)
	two.Annotations = map[string]string{"summary": "b certificate expires in 1 day."}
	missing := testAlert("missing", "Cert", "warning", "active", now)
	missing.Annotations = map[string]string{"summary": "something else"}
	a := newAlertsState(config.TableView{
		Columns: []config.Column{
			{Field: "state"},
			{Field: "annotations.summary", Title: "Expires in", Extract: regexp.MustCompile(`expires in (.+?)\.$`)},
			{Field: "labels.path", Extract: regexp.MustCompile(`/certs/(.*)`)},
		},
		Search: []string{"annotations.summary"},
	})
	for _, raw := range []alertRow{newAlertRow("am", one), newAlertRow("am", two), newAlertRow("am", missing)} {
		a.rows = append(a.rows, raw)
	}
	table := a.tableLayout(78, now)
	if header := ansi.Strip(table.header()); !strings.Contains(header, "Expires in") || !strings.Contains(header, "PATH") {
		t.Fatalf("unexpected header %q", header)
	}
	first := ansi.Strip(table.row(true, a.rows[0], now))
	for _, want := range []string{"› ", "Silenced + Inhibited", "9 days", "界"} {
		if !strings.Contains(first, want) {
			t.Errorf("missing %q in %q", want, first)
		}
	}
	for _, unwanted := range []string{"certificate", "/certs"} {
		if strings.Contains(first, unwanted) {
			t.Errorf("unexpected %q in %q", unwanted, first)
		}
	}
	line := ansi.Strip(table.row(false, a.rows[2], now))
	if strings.Count(line, "—") != 2 || strings.Contains(line, "something") {
		t.Errorf("non-matching or missing values should show placeholders: %q", line)
	}
	if !a.matches(a.rows[0], newFuzzyFilter("certificate"), now) {
		t.Error("search should still match the raw annotation")
	}
	if compareAlertField(a.rows[0], a.rows[1], "annotations.summary", now) >= 0 {
		t.Error("sort should compare raw annotations rather than extracts")
	}
}

func TestAlertTableNarrowWidths(t *testing.T) {
	now := time.Now()
	raw := testAlert("one", "界界界界界界界界", "critical", "suppressed", now)
	raw.Status.SilencedBy, raw.Status.InhibitedBy = []string{"silence"}, []string{"inhibit"}
	row := newAlertRow("am", raw)
	a := newAlertsState(config.TableView{})
	a.rows = []alertRow{row}
	narrow := a.tableLayout(22, now)
	if len(narrow.cols) != 2 || narrow.cols[0].Field != "state" || narrow.cols[1].Field != "alertname" {
		t.Fatalf("narrow default lost state or alert name: %v", narrow.cols)
	}
	if line := ansi.Strip(narrow.row(true, row, now)); !strings.Contains(line, "Sil.+Inhib.") || !strings.Contains(line, "界…") {
		t.Fatalf("expected abbreviated state and safely truncated wide text: %q", line)
	}
	for _, selected := range []bool{false, true} {
		line := narrow.row(selected, row, now)
		if selected && (line == ansi.Strip(line) || !strings.HasPrefix(ansi.Strip(line), "› ")) {
			t.Errorf("missing selected row marker or style: %q", line)
		}
	}
	for range 12 {
		a.view.Columns = append(a.view.Columns, config.Column{Field: "labels.alertname", Title: "多字 Heading"})
	}
	for w := 0; w <= 42; w++ {
		table := a.tableLayout(w, now)
		for _, line := range []string{table.header(), table.row(false, row, now), table.row(true, row, now)} {
			if got := ansi.StringWidth(line); got != w {
				t.Fatalf("width %d: got %d cells for %q", w, got, line)
			}
		}
	}
}

func TestAlertTablePreservesSeverityTintOnSelection(t *testing.T) {
	now := time.Now()
	row := newAlertRow("am", testAlert("am", "Cert", "critical", "active", now))
	a := newAlertsState(config.TableView{Columns: []config.Column{
		{Field: "alertname"}, {Field: "labels.severity"}, {Field: "duration"},
	}})
	a.rows = []alertRow{row}
	table := a.tableLayout(60, now)
	for _, selected := range []bool{false, true} {
		line := table.row(selected, row, now)
		buf := uv.NewScreenBuffer(60, 1)
		uv.NewStyledString(line).Draw(buf, buf.Bounds())
		text := ansi.Strip(line)
		x := strings.Index(text, "critical")
		if x < 0 {
			t.Fatalf("severity missing from row: %q", text)
		}
		want := uv.Style{Fg: ansi.Red}
		if selected {
			want = uv.Style{Bg: ansi.Red, Attrs: uv.AttrReverse}
		}
		if cell := buf.CellAt(x, 0); !cell.Style.Equal(&want) {
			t.Errorf("selected=%t: severity style %+v, want %+v", selected, cell.Style, want)
		}
	}
}

func alertTableWidths(table alertTable) []int {
	widths := make([]int, 0, len(table.cols))
	for _, c := range table.cols {
		widths = append(widths, c.width)
	}
	return widths
}
