package history

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestBoundedMultilineRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "history.jsonl")
	var entries []Entry
	for i, conn := range []string{"old", "a", "b", "a"} {
		q := []string{"old", "up", "sum(\nup)", "sum(\nup)"}[i]
		entries = Add(entries, Entry{conn, q, time.Unix(int64(i), 0).UTC()}, 2)
	}
	if len(entries) != 2 || entries[0].At.Unix() != 3 || entries[1].Query != "up" {
		t.Fatal(entries)
	}
	if err := Save(path, entries); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path, 2)
	if err != nil || !slices.Equal(got, entries) {
		t.Fatal(got, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("unexpected file mode: %v %v", info, err)
	}
}

func TestLoadDeduplicatesLegacyConnectionsBeforeApplyingLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	entries := []Entry{{Connection: "b", Query: "up"}, {Connection: "a", Query: "up"}, {Connection: "a", Query: "down"}}
	if err := Save(path, entries); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path, 2)
	if err != nil || !slices.Equal(got, []Entry{entries[0], entries[2]}) {
		t.Fatal(got, err)
	}
}
