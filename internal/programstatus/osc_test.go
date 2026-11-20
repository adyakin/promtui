package programstatus

import (
	"strings"
	"testing"
)

func TestReport(t *testing.T) {
	for _, state := range []State{Idle, Working, Done, Error} {
		want := "\x1b]7501;state=" + string(state) + ":id=promtui:app=promtui\x1b\\"
		if got := Report(state, ""); got != want {
			t.Errorf("%q: got %q, want %q", state, got, want)
		}
	}
	// UTF-8 and delimiters are encoded as standard (padded) base64.
	if got := Report(Working, "é:;?"); got != "\x1b]7501;state=working:id=promtui:app=promtui:msg=w6k6Oz8=\x1b\\" {
		t.Errorf("unexpected encoded message: %q", got)
	}
	if got := Clear(); got != "\x1b]7501;state=clear:id=promtui\x1b\\" {
		t.Errorf("unexpected clear report: %q", got)
	}
}

func TestInvalidReports(t *testing.T) {
	for _, message := range []string{"\n", "\t", "\x1b\\", "\x7f", "\u009c", "\xff", strings.Repeat("x", 2049)} {
		if Report(Working, message) != "" {
			t.Errorf("accepted invalid message %q", message)
		}
	}
	if Report("running", "test") != "" || Report("clear", "test") != "" {
		t.Error("accepted an unsupported report state")
	}
	if got := Report(Working, strings.Repeat("x", 2048)); got == "" || len(got) > 4096 {
		t.Error("did not encode a message at the protocol size limit")
	}
}
