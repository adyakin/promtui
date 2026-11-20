package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func paneErrorRows(width int, heading string, details ...string) []string {
	width = max(1, width)
	rows := strings.Split(ansi.Wrap(heading, width, ""), "\n")
	for _, detail := range details {
		text := ansi.Wrap(displayErrorText(detail), width, "")
		rows = append(rows, strings.Split(ansi.Hardwrap(text, width, true), "\n")...)
	}
	return rows
}

func renderPaneError(body, rows []string, offset int) string {
	start := min(offset, max(0, len(rows)-len(body)))
	end := min(start+len(body), len(rows))
	for i, line := range rows[start:end] {
		body[i] = "  " + line
	}
	if end-start < len(rows) {
		return fmt.Sprintf("%d–%d/%d", start+1, end, len(rows))
	}
	return ""
}

func scrollPaneError(key string, offset, total, room int) int {
	limit := max(0, total-room)
	return targetNavigate(key, min(offset, limit), limit+1, room)
}
