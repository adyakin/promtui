package app

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/config"
)

func TestRefreshErrorsWrapAndScrollInsidePane(t *testing.T) {
	for _, tab := range []tabID{alertsTab, targetsTab} {
		t.Run(tabs[tab], func(t *testing.T) {
			m := New(Options{Connection: config.Connection{AlertmanagerURL: "http://example.test",
				Targets: []config.TargetSource{{Name: "default", URL: "http://example.test"}}}}, nil)
			m.activeTab = tab
			m.resize(minWidth, minHeight)
			err := errors.New("Response exceeds 32 MiB.\n" + strings.Repeat("x", 90) + "\n\x1b[31mTail marker\x1b[0m")
			if tab == alertsTab {
				m.alerts.setError(err, false)
			} else {
				m.targets.sources[0].err = err
			}
			body := func() []string {
				rows := strings.Split(ansi.Strip(m.screen()), "\n")
				if len(rows) != m.height {
					t.Fatalf("invalid screen height:\n%s", m.screen())
				}
				for _, row := range rows {
					if ansi.StringWidth(row) > m.width {
						t.Fatalf("row exceeds terminal width: %q", row)
					}
				}
				return rows[2 : m.height-3]
			}
			if screen := strings.Join(body(), "\n"); !strings.Contains(screen, "32") || !strings.Contains(screen, "MiB.") ||
				strings.Contains(screen, "Tail marker") {
				t.Fatalf("error not wrapped in pane:\n%s", screen)
			}
			press(m, tea.KeyEnd)
			if screen := strings.Join(body(), "\n"); !strings.Contains(screen, "Tail marker") {
				t.Fatalf("end of error not reachable:\n%s", screen)
			}
			press(m, tea.KeyHome)
			if screen := strings.Join(body(), "\n"); strings.Contains(screen, "Tail marker") {
				t.Fatalf("Home didn't restore the first page:\n%s", screen)
			}
		})
	}
}
