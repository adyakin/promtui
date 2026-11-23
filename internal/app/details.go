package app

import (
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/ui"
)

var (
	detailKey     = title.Bold(false)
	detailMissing = lipgloss.NewStyle().Faint(true)
	detailSuccess = lipgloss.NewStyle().Foreground(statusGreen)
	detailError   = lipgloss.NewStyle().Foreground(statusRed)
	detailWarning = lipgloss.NewStyle().Foreground(statusYellow)
	detailLink    = title.Bold(false).Underline(true)
)

func detailText(s string) string {
	return ui.Multiline(s)
}

func detailHeading(s string) string { return title.Render(detailText(s)) }

// A nested detail span resets the pen when it ends. Restore the dialog
// surface rather than letting the rest of that row inherit terminal defaults.
var detailDialogRow = func() func(string) string {
	prefix, suffix, _ := strings.Cut(dialogSurface.Render("x"), "x")
	reset := strings.NewReplacer("\x1b[m", "\x1b[m"+prefix, ansi.ResetStyle, ansi.ResetStyle+prefix)
	return func(s string) string { return prefix + reset.Replace(s) + suffix }
}()

// Only emit hyperlinks for intact web URLs. In particular, never turn a URL
// containing terminal controls into a different, clickable destination.
func webLink(s string) bool {
	if !utf8.ValidString(s) || strings.ContainsFunc(s, func(r rune) bool {
		return unicode.IsControl(r) || unicode.IsSpace(r)
	}) {
		return false
	}
	u, err := url.Parse(s)
	return err == nil && u.Hostname() != "" && (u.Scheme == "http" || u.Scheme == "https")
}

func detailField(key, separator, value string, semantic bool) string {
	style := lipgloss.NewStyle()
	if value == "" || value == "—" {
		value, style = "—", detailMissing
	} else if semantic {
		switch key {
		case "Error", "Last scrape error", "Evaluation errors":
			style = detailError
		case "Warning", "Pending":
			style = detailWarning
		case "Severity", "Health", "State", "Source status":
			switch strings.ToLower(value) {
			case "critical", "down", "error", "err", "failed", "firing":
				style = detailError
			case "warning", "pending", "unknown", "inhibited", "silenced", "suppressed",
				"silenced + inhibited", "unprocessed", "stale", "partial":
				style = detailWarning
			case "up", "ok", "healthy", "normal":
				style = detailSuccess
			}
		case "Generator URL", "Scrape URL", "Global URL":
			if webLink(value) {
				return detailKey.Render(detailText(key)+separator) + detailLink.Hyperlink(value).Render(value)
			}
		}
	}
	return detailKey.Render(detailText(key)+separator) + style.Render(detailText(value))
}

func detailLine(line string, semantic bool, headings ...string) string {
	for _, heading := range headings {
		if line == heading {
			return title.Render(line)
		}
	}
	if key, value, ok := strings.Cut(line, ": "); ok && !strings.HasPrefix(line, "  ") {
		return detailField(key, ": ", value, semantic)
	}
	if line == "—" {
		return detailMissing.Render(line)
	}
	return detailText(line)
}

// Break ANSI spans at line boundaries, so scrolling can render each row
// independently and no style or link spills onto a border or another row.
func wrappedStyled(s string, width int, hard bool) []string {
	width = max(1, width)
	if !hard {
		return strings.Split(lipgloss.Wrap(s, width, ""), "\n")
	}
	var b strings.Builder
	w := lipgloss.NewWrapWriter(&b)
	_, _ = w.Write([]byte(ansi.Hardwrap(s, width, true)))
	_ = w.Close()
	return strings.Split(b.String(), "\n")
}

func wrapDetails(lines []string, width int, hard bool, headings ...string) (rows []string) {
	for _, line := range lines {
		rows = append(rows, wrappedStyled(detailLine(line, true, headings...), width, hard)...)
	}
	return rows
}

var yamlStyles = ui.YAMLStyles{
	Key: detailKey, String: lipgloss.NewStyle(), Number: detailWarning,
	Boolean: detailSuccess, Null: detailMissing, Comment: detailMissing, List: title, Quoted: lipgloss.NewStyle().Italic(true),
}

type urlFeedbackExpired uint64

func (m *Model) copyDetailURL(value string) tea.Cmd {
	if !webLink(value) {
		return nil
	}
	m.urlFeedback++
	id := m.urlFeedback
	m.showURLFeedback = true
	return tea.Sequence(tea.SetClipboard(value), tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return urlFeedbackExpired(id)
	}))
}
