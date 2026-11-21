package app

import (
	"image/color"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/config"
)

func TestStatusRowTextColors(t *testing.T) {
	for _, tt := range []struct {
		name, label string
		row         func(bool) string
		tint        color.Color
	}{
		{"target down", "DOWN", func(selected bool) string {
			return targetColumns(22, selected, [6]string{"DOWN", "node"})
		}, ansi.Red},
		{"target unknown", "UNKNOWN", func(selected bool) string {
			return targetColumns(60, selected, [6]string{"UNKNOWN", "node"})
		}, ansi.Yellow},
		{"target up", "UP", func(selected bool) string {
			return targetColumns(60, selected, [6]string{"UP", "node"})
		}, nil},
		{"alert critical", "critical", func(selected bool) string {
			return alertColumns(60, selected, "Active", "critical", "DiskFull", "node", "1m")
		}, ansi.Red},
		{"alert high", "HIGH", func(selected bool) string {
			return alertColumns(80, selected, "Active", "HIGH", "DiskFull", "node", "1m")
		}, ansi.Yellow},
		{"alert warning", "warning", func(selected bool) string {
			return alertColumns(60, selected, "Active", "warning", "DiskFull", "node", "1m")
		}, nil},
		{"custom alert severity", "critical", func(selected bool) string {
			table := layoutViewTable(config.AlertsView, []config.Column{{Field: "labels.servicename"}, {Field: "labels.severity"}}, 60)
			return table.row(selected, func(field string, _ int) (string, color.Color) {
				if field == "labels.severity" {
					return "critical", alertSeverityTint("critical")
				}
				return "billing", nil
			})
		}, ansi.Red},
	} {
		t.Run(tt.name, func(t *testing.T) {
			plain := ansi.Strip(tt.row(false))
			start := strings.Index(plain, tt.label)
			if start < 0 {
				t.Fatalf("missing label %q in %q", tt.label, plain)
			}
			for _, selected := range []bool{false, true} {
				row := tt.row(selected)
				prefix := "  "
				if selected {
					prefix = "› "
				}
				if got := strings.TrimRight(ansi.Strip(row), " "); got != prefix+strings.TrimRight(plain[2:], " ") {
					t.Fatalf("unexpected row text: %q", got)
				}
				width := ansi.StringWidth(row)
				buf := uv.NewScreenBuffer(width, 1)
				uv.NewStyledString(row).Draw(buf, buf.Bounds())
				for x := 0; x < width; x++ {
					var want uv.Style
					if selected {
						want.Attrs = uv.AttrReverse
					}
					if x >= start && x < start+len(tt.label) {
						if selected {
							want.Bg = tt.tint
						} else {
							want.Fg = tt.tint
						}
					}
					if cell := buf.CellAt(x, 0); !cell.Style.Equal(&want) {
						t.Fatalf("selected=%t column=%d: style %+v, want %+v", selected, x, cell.Style, want)
					}
				}
			}
		})
	}
}
