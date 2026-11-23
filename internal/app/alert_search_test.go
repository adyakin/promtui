package app

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/config"
)

func TestAlertSearchSuggestionsUseLoadedNames(t *testing.T) {
	a := newAlertsState(config.TableView{Search: []string{"severity"}})
	now := time.Now()
	for i, name := range []string{"CertExpiry", "", "CertCheck", "CertExpiry", "DiskFull"} {
		a.rows = append(a.rows, newAlertRow("source", testAlert(fmt.Sprint(i), name, "critical", "active", now)))
	}
	a.structured.match = func(labels map[string]string) bool { return labels["alertname"] == "DiskFull" }
	a.rebuild("", 0, now)
	a.refreshAlertNames()
	a.input.SetValue("cRT")
	a.refreshSearchSuggestions() // Opening may refresh before focusing the input.
	a.input.Focus()
	if got := a.suggestions.matches; !slices.Equal(got, []string{"CertCheck", "CertExpiry"}) {
		t.Fatalf("matches = %v, want distinct fuzzy matches from all loaded rows", got)
	}
	if len(a.visible) != 1 || a.searchSuggestionKey("enter") || a.input.Value() != "cRT" {
		t.Fatal("suggestions changed table filtering or applied an implicit selection")
	}
	if !a.searchSuggestionKey("down") || a.suggestions.selectedName() != "CertCheck" || a.input.Value() != "cRT" {
		t.Fatal("down did not highlight the first match without editing the input")
	}
	if !a.searchSuggestionKey("enter") || a.input.Value() != "CertCheck" || !a.input.Focused() || a.filter != "" || a.structured.match == nil {
		t.Fatal("Enter did not fill only the search input and report the explicit selection")
	}
	for _, value := range []string{"  ", "nothing matches", "critic"} {
		a.input.SetValue(value)
		a.refreshSearchSuggestions()
		if len(a.suggestions.matches) != 0 || a.searchSuggestionKey("down") || a.searchSuggestionKey("enter") || a.input.Value() != value {
			t.Fatalf("unexpected completion for %q", value)
		}
	}
}

func TestAlertSearchSuggestionSelectionOnRefresh(t *testing.T) {
	a := newAlertsState(config.TableView{})
	a.input.Focus()
	a.input.SetValue("Cert")
	row := func(name string) alertRow {
		return newAlertRow("source", testAlert(name, name, "critical", "active", time.Now()))
	}
	a.rows = []alertRow{row("CertBeta"), row("CertGamma")}
	a.refreshAlertNames()
	a.searchSuggestionKey("down")
	a.rows = append(a.rows, row("CertAlpha"))
	a.refreshAlertNames()
	if got := a.suggestions.selectedName(); got != "CertBeta" {
		t.Fatalf("data refresh moved selection to %q", got)
	}
	a.rows = a.rows[1:]
	a.refreshAlertNames()
	if got := a.suggestions.selectedName(); got != "" {
		t.Fatalf("a removed selection was replaced with %q", got)
	}
	a.searchSuggestionKey("up")
	a.input.SetValue("CertG")
	a.refreshSearchSuggestions()
	if got := a.suggestions.selectedName(); got != "" || a.searchSuggestionKey("esc") {
		t.Fatalf("text edit kept a selection (%q), or autocomplete intercepted Esc", got)
	}
}

func TestAlertSuggestionsLayerFitsAndScrolls(t *testing.T) {
	m := New(Options{}, nil)
	m.alerts.input.Focus()
	m.alerts.input.SetValue("cert")
	for i := range 12 {
		name := fmt.Sprintf("Cert%02d%s", i, strings.Repeat("界", 30))
		m.alerts.rows = append(m.alerts.rows, newAlertRow("source", testAlert(name, name, "critical", "active", time.Now())))
	}
	m.alerts.refreshAlertNames()
	check := func(width, height, visible int, want string) {
		t.Helper()
		m.resize(width, height)
		baseRow := strings.Repeat(".", width)
		base := strings.TrimSuffix(strings.Repeat(baseRow+"\n", height), "\n")
		layer := m.alertSuggestionsLayer()
		if layer == nil {
			t.Fatal("missing suggestions layer")
		}
		screen := ansi.Strip(lipgloss.NewCompositor(lipgloss.NewLayer(base), layer).Render())
		rows := strings.Split(screen, "\n")
		if len(rows) != height || rows[3] != baseRow || rows[height-3] != baseRow || !strings.Contains(rows[4], "Alert names") {
			t.Fatalf("dropdown obscured the input or extended beyond the table pane:\n%s", screen)
		}
		if !strings.Contains(screen, want) || strings.Count(screen, "Cert") != visible {
			t.Fatalf("wrong suggestion viewport, want %d rows including %q:\n%s", visible, want, screen)
		}
		for _, row := range rows {
			if ansi.StringWidth(row) != width {
				t.Fatalf("dropdown changed screen width: %q", row)
			}
		}
	}
	check(80, 24, alertSuggestionLimit, "Cert00")
	for range 12 {
		m.alerts.searchSuggestionKey("down")
	}
	check(80, 24, alertSuggestionLimit, "› Cert11")
	check(minWidth, minHeight, 1, "› Cert11")
	m.alerts.input.Blur()
	if m.alertSuggestionsLayer() != nil {
		t.Fatal("dropdown stayed open after leaving search")
	}
}
