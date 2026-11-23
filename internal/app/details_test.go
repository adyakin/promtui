package app

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"promtui/internal/api"
)

func TestDetailSemanticStyles(t *testing.T) {
	for _, tc := range []struct{ key, value, styled string }{
		{"Severity", "critical", detailError.Render("critical")},
		{"Health", "DOWN", detailError.Render("DOWN")},
		{"Health", "UP", detailSuccess.Render("UP")},
		{"State", "pending", detailWarning.Render("pending")},
		{"State", "ordinary", "ordinary"},
		{"Last scrape error", "", detailMissing.Render("—")},
	} {
		want := detailKey.Render(tc.key+": ") + tc.styled
		if got := detailField(tc.key, ": ", tc.value, true); got != want {
			t.Errorf("%q: got %q, want %q", tc.key, got, want)
		}
	}
	if got := detailLine("Overview", true, "Overview"); got != title.Render("Overview") {
		t.Errorf("heading is not accented: %q", got)
	}
}

func TestDetailMultilineWrapping(t *testing.T) {
	m := New(Options{}, nil)
	m.resize(35, 13)
	lastError := "failed to connect\r\nline two\n" + strings.Repeat("wide界", 30)
	m.targets.dialog = targetDetails
	m.targets.detail = targetRow{Target: api.Target{LastError: lastError}, health: targetDown}
	_, rows, width, _ := m.targetTextDialog()
	text := strings.Join(rows, "\n")
	if !strings.Contains(ansi.Strip(text), "line two") ||
		!strings.Contains(text, detailError.Render("line two")) {
		t.Fatalf("lost multiline error styling: %q", text)
	}
	for _, row := range rows {
		if got := ansi.StringWidth(row); got > width-4 {
			t.Errorf("row has width %d, limit %d: %q", got, width-4, row)
		}
	}
	a := testAlert("x", "name", "warning", "active", time.Now())
	a.Annotations = map[string]string{"description": "first\n\n" + strings.Repeat("annotation-界", 20)}
	m.alerts.openAlertDetails(newAlertRow("local", a))
	wrapped := m.alerts.wrappedAlertDetails(time.Now(), 27)
	plain := ansi.Strip(strings.Join(wrapped, "\n"))
	if !strings.Contains(plain, "description: first\n\n") || strings.Count(plain, "界") != 20 {
		t.Fatalf("lost annotation contents: %q", plain)
	}
}

func TestDetailHyperlinkSafetyAndWrapping(t *testing.T) {
	link := "https://example.org/" + strings.Repeat("測-", 30) + "?a=b%20c#anchor"
	rows := wrappedStyled(detailField("Scrape URL", ": ", link, true), 22, false)
	var text strings.Builder
	linkedRows := 0
	for _, row := range rows {
		opener := ansi.SetHyperlink(link)
		_, after, linked := strings.Cut(row, opener)
		if !linked {
			continue
		}
		span, _, closed := strings.Cut(after, ansi.ResetHyperlink())
		if !closed {
			t.Fatalf("unterminated hyperlink in %q", row)
		}
		text.WriteString(ansi.Strip(span))
		linkedRows++
	}
	if linkedRows < 2 || text.String() != link {
		t.Fatalf("wrapped display changed the original URL: %q", text.String())
	}
	for _, unsafe := range []string{
		"https://example.org/\x1b]52;c;payload\x07", "https://example.org/\u009c",
		"https://example.org/\r\nhi", "https://example.org/\xff", "file:///etc/passwd", "javascript:example",
	} {
		got := detailField("Scrape URL", ": ", unsafe, true)
		if webLink(unsafe) || strings.Contains(ansi.Strip(got), "\x07") || strings.Contains(got, "\x1b]") {
			t.Errorf("unsafe hyperlink: %q rendered as %q", unsafe, got)
		}
	}
}
