package app

import (
	"context"
	"errors"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"promtui/internal/ui"
)

type queryPhase uint8

const (
	queryReady queryPhase = iota
	queryRunning
	querySucceeded
	queryFailed
	queryCancelled
)

const executingQuery = "Executing query..."

type queryError struct {
	full, cause string
	elapsed     time.Duration
	timeout     bool
	wrapWidth   int
	wrapped     []string
	inlineWidth int
	inlineLines []string
}

var httpURL = regexp.MustCompile(`(?i)https?://[^\s"'<>]+`)

func newQueryError(err error, elapsed time.Duration) queryError {
	q := queryError{full: err.Error(), elapsed: elapsed.Round(time.Millisecond)}
	var networkError net.Error
	q.timeout = errors.Is(err, context.DeadlineExceeded) || errors.As(err, &networkError) && networkError.Timeout()
	if q.timeout {
		q.cause = "context deadline exceeded"
		return q
	}
	var request *url.Error
	if errors.As(err, &request) && request.Err != nil {
		err = request.Err
	}
	q.cause = strings.TrimSpace(displayErrorText(httpURL.ReplaceAllString(err.Error(), "[URL]")))
	return q
}

func (q *queryError) heading() string {
	if q.elapsed > 0 {
		return "Query failed after " + q.elapsed.String()
	}
	return "Query failed"
}

func (q *queryError) summary(width int) string {
	heading := q.heading()
	if ansi.StringWidth(heading)+8 > width && q.elapsed > 0 {
		heading = "Failed (" + q.elapsed.String() + ")"
	}
	if q.timeout && ansi.StringWidth(heading+": "+q.cause) > width {
		heading = "Query timed out"
		if q.elapsed > 0 {
			heading = "Timed out (" + q.elapsed.String() + ")"
		}
		return ansi.Truncate(heading, width, "…")
	}
	if q.cause != "" {
		heading += ": " + strings.Join(strings.Fields(q.cause), " ")
	}
	return ansi.Truncate(heading, width, "…")
}

func (q *queryError) renderInline(rows []string, width int, editing bool) {
	if len(rows) == 1 {
		rows[0] = "  " + q.summary(width)
		return
	}
	if q.inlineWidth != width {
		text := q.heading()
		if q.cause != "" {
			text += ": " + q.cause
		}
		q.inlineLines = strings.Split(ansi.Wrap(text, width, ""), "\n")
		q.inlineWidth = width
	}

	room := len(rows)
	if editing && len(q.inlineLines) > room {
		// Results advertises e in its footer; while editing e is ordinary text.
		rows[room-1] = "  Tab, e: Error details"
		room--
	}
	for i, line := range q.inlineLines[:min(room, len(q.inlineLines))] {
		if i == room-1 && len(q.inlineLines) > room {
			line = ansi.Truncate(strings.TrimRight(line, " ")+"…", width, "…")
		}
		rows[i] = "  " + line
	}
}

func displayErrorText(text string) string {
	text = strings.NewReplacer("\r\n", "\n", "\t", "    ").Replace(text)
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = ui.Plain(lines[i])
	}
	return strings.Join(lines, "\n")
}

func (q *queryError) lines(width int) []string {
	if q.wrapWidth != width {
		// Sanitize for display only. Keep the original bytes for Copy.
		text := q.heading() + "\n\n" + displayErrorText(q.full)
		q.wrapWidth = width
		q.wrapped = strings.Split(ansi.Hardwrap(text, width, true), "\n")
	}
	return q.wrapped
}

// Keep the existing clipboard writer, and keep the original message separate
// from the text sanitized and wrapped for the terminal.
type copyQueryError string
