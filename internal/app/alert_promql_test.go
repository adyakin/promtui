package app

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/prometheus/common/model"
	"promtui/internal/config"
	"promtui/internal/editor"
	"promtui/internal/history"
	"promtui/internal/query"
)

func TestAlertGeneratorExpression(t *testing.T) {
	const base = "https://prometheus.example.com/graph"
	const selector = `up{job="nodes"}`
	const multiline = "  sum(\n\tup{job=\"one+two&three=four%26\"}\n) + 2  \n"
	for _, tc := range []struct {
		name, link, expression, errorText string
	}{
		{"standard", base + "?g0.expr=up%7Bjob%3D%22nodes%22%7D&g0.tab=1", selector, ""},
		{"legacy", base + "?expr=up%7Bjob%3D%22nodes%22%7D", selector, ""},
		{"precedence", base + "?expr=down&g0.expr=up", "up", ""},
		{"multiline and special characters", base + "?g0.expr=" + url.QueryEscape(multiline), multiline, ""},
		{"path prefix", "http://prometheus.example.com/prometheus/graph?g0.expr=up", "up", ""},
		{"empty preferred parameter", base + "?g0.expr=&expr=up", "", "Generator query is empty"},
		{"whitespace only", base + "?expr=+%0A%09", "", "Generator query is empty"},
		{"missing parameter", base + "?g0.tab=1", "", "No PromQL in generator URL"},
		{"missing URL", "", "", "No generator URL"},
		{"malformed URL", "http://[invalid/graph?expr=up", "", "Invalid generator URL"},
		{"malformed escape", base + "?g0.expr=%QQ&expr=up", "", "Invalid generator URL query"},
		{"unsupported route", "https://vmalert.example.com/vmalert/alert?expr=up", "", "Not a Prometheus graph URL"},
		{"unsupported scheme", "ftp://prometheus.example.com/graph?expr=up", "", "Not a Prometheus graph URL"},
		{"no host", "https:///graph?expr=up", "", "Not a Prometheus graph URL"},
		{"path is not an expression", "https://prometheus.example.com/vector(1)", "", "Not a Prometheus graph URL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := alertGeneratorExpression(tc.link)
			if tc.errorText != "" {
				if err == nil || err.Error() != tc.errorText {
					t.Fatalf("got expression %q and error %v, want error %q", got, err, tc.errorText)
				}
			} else if got != tc.expression || err != nil {
				t.Fatalf("got %q, %v; want %q", got, err, tc.expression)
			}
		})
	}
}

func addAlertWithGenerator(m *Model, link string) {
	now := time.Now()
	a := testAlert("node-down", "NodeDown", "critical", "active", now)
	a.GeneratorURL = link
	m.alerts.rows = []alertRow{newAlertRow("alertmanager", a)}
	m.alerts.rebuild("", 0, now)
}

func TestLoadAlertQuery(t *testing.T) {
	const expression = "  sum(\n  up{job=\"a+b&c=d%26\"}\n) + 2  \n"
	options := Options{
		Connection: config.Connection{Name: "current", URL: "https://current.example.com"},
		Ranged:     true, Timeout: time.Second, Window: time.Hour, Step: time.Minute,
		End: time.Unix(42, 0), HistoryLimit: 5,
		SavedQueriesPath: t.TempDir() + "/saved.yaml",
	}
	entries := []history.Entry{{Query: "vector(1)", Connection: options.Connection.Name}}
	m := New(options, entries)
	result := query.Result{Start: time.Unix(1, 0), Series: []query.Series{{
		Name: "previous", Samples: []model.SamplePair{{Timestamp: 1000, Value: 1}},
	}}}
	m.Update(completed{result: result})
	m.editor.SetValue("old draft")
	m.cursor = 0
	press(m, '?') // Leave an old PromQL panel open.
	pressAlt(m, '1')
	addAlertWithGenerator(m, "https://alert-source.example.com/graph?g0.expr="+url.QueryEscape(expression))

	selection := m.alerts.selection()
	press(m, 'p')
	if m.activeTab != alertsTab || m.alerts.selection() != selection || m.editor.Value() != "old draft" {
		t.Fatal("p changed state from the Alerts list")
	}
	press(m, tea.KeyEnter)
	if footer := ansi.Strip(m.footer()); !strings.Contains(footer, "p Load in PromQL") {
		t.Fatalf("details footer did not advertise the action: %q", footer)
	}
	press(m, '?', 'p')
	if m.activeTab != alertsTab || !m.alerts.help || m.editor.Value() != "old draft" {
		t.Fatal("p reached Alert Details underneath Help")
	}
	press(m, tea.KeyEsc, 'p')
	if m.activeTab != promQLTab || m.alerts.dialog != alertNoDialog || m.scope() != editKeys || m.View().Cursor == nil {
		t.Fatal("p did not close Alert Details and focus the PromQL editor")
	}
	if m.editor.Value() != expression || editor.Position(m.editor) != len(expression) || m.cursor != -1 {
		t.Fatalf("incorrect draft or cursor after loading: %q at %d", m.editor.Value(), editor.Position(m.editor))
	}
	if m.id != 0 || m.cancel != nil || m.queryPhase != querySucceeded || !reflect.DeepEqual(m.entries, entries) {
		t.Fatal("loading changed query execution or history state")
	}
	if !reflect.DeepEqual(m.chart.result, result) || !reflect.DeepEqual(m.table.Rows, result.Series) {
		t.Fatal("loading changed previous results")
	}
	if !reflect.DeepEqual(m.opt, options) {
		t.Fatal("loading changed query settings")
	}

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	defer m.stop()
	if cmd == nil || m.queryPhase != queryRunning || m.entries[0].Query != strings.TrimSpace(expression) {
		t.Fatal("Ctrl+Enter did not run the loaded query")
	}
}

func TestLoadAlertQueryFailure(t *testing.T) {
	for _, tc := range []struct{ link, status string }{
		{"", "No generator URL"},
		{"https://vmalert.example.com/alert?expr=up", "Not a Prometheus graph URL"},
		{"https://prometheus.example.com/graph?g0.expr=%XX", "Invalid generator URL query"},
		{"https://prometheus.example.com/graph?g0.expr=" + strings.Repeat("a", 65537), "exceeds editor limits"},
		{"https://prometheus.example.com/graph?g0.expr=%09up", "contains unsupported characters"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			m := New(Options{}, nil)
			m.editor.SetValue("old draft\n  + 2")
			m.editor.SetCursorColumn(1)
			pos := editor.Position(m.editor)
			press(m, '1')
			addAlertWithGenerator(m, tc.link)
			press(m, tea.KeyEnter, 'p')
			if m.activeTab != alertsTab || m.alerts.dialog != alertDetails || m.alerts.err != nil ||
				!strings.Contains(ansi.Strip(m.screen()), tc.status) {
				t.Fatalf("expected an error in Alert Details: %q", m.alerts.detailStatus)
			}
			if m.editor.Value() != "old draft\n  + 2" || editor.Position(m.editor) != pos || m.editor.Focused() {
				t.Fatal("failed load changed the PromQL editor")
			}
			press(m, tea.KeyEsc, 'q')
			if !m.quitting {
				t.Fatal("q no longer quits from the Alerts list")
			}
		})
	}
}
