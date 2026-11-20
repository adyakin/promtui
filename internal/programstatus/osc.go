// Package programstatus encodes PromTUI's OSC 7501 reports.
package programstatus

import (
	"encoding/base64"
	"strings"
	"unicode"
	"unicode/utf8"
)

type State string

const (
	Idle    State = "idle"
	Working State = "working"
	Done    State = "done"
	Error   State = "error"
)

const (
	prefix = "\x1b]7501;state="
	record = ":id=promtui"
	st     = "\x1b\\"
)

// Report returns an empty string for an unsupported state or invalid message.
func Report(state State, message string) string {
	switch state {
	case Idle, Working, Done, Error:
	default:
		return ""
	}
	if len(message) > 2048 || !utf8.ValidString(message) || strings.ContainsFunc(message, unicode.IsControl) {
		return ""
	}
	report := prefix + string(state) + record + ":app=promtui"
	if message != "" {
		report += ":msg=" + base64.StdEncoding.EncodeToString([]byte(message))
	}
	return report + st
}

// Clear removes only PromTUI's record, not the terminal's root record.
func Clear() string { return prefix + "clear" + record + st }
