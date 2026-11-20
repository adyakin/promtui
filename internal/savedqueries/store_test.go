package savedqueries

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func mustOpen(t *testing.T, path string) Snapshot {
	t.Helper()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestCRUDPreservesOrderAndExpression(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "queries.yaml")
	s := mustOpen(t, path)
	if len(s.Queries()) != 0 {
		t.Fatal("missing file should mean an empty collection")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("read unexpectedly created the file: %v", err)
	}
	const expr = "\n sum(\n\tmetric{label=\"☃\"}  \n )\n\n"
	s, err := s.Add("  First  ", expr)
	if err != nil {
		t.Fatal(err)
	}
	s, err = s.Add("Second", "up")
	if err != nil {
		t.Fatal(err)
	}
	s, err = mustOpen(t, path).Rename(0, "  RENAMED  ")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := s.Queries(), []Query{{"RENAMED", expr}, {"Second", "up"}}; !slices.Equal(got, want) {
		t.Fatalf("rename lost formatting or order: %#v", got)
	}
	s, err = s.Update(1, " vector(1) ")
	if err != nil {
		t.Fatal(err)
	}
	if got := mustOpen(t, path).Queries()[1]; got != (Query{"Second", " vector(1) "}) {
		t.Fatalf("update = %#v", got)
	}
	const indented = "  up\n + 2\n"
	s, err = s.Update(1, indented)
	if err != nil {
		t.Fatal(err)
	}
	if got := mustOpen(t, path).Queries()[1].Expression; got != indented {
		t.Fatalf("lost first-line indentation: %q", got)
	}
	s, err = s.Delete(1)
	if err != nil {
		t.Fatal(err)
	}
	if got := mustOpen(t, path).Queries(); !slices.Equal(got, []Query{{"RENAMED", expr}}) {
		t.Fatalf("delete = %#v", got)
	}
	if _, err := s.Delete(0); err != nil {
		t.Fatal(err)
	}
	if len(mustOpen(t, path).Queries()) != 0 {
		t.Fatal("deleting last entry did not persist empty collection")
	}
}

func TestExpressionEncoding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queries.yaml")
	for _, expression := range []string{"42", "true", "up\n+ 1\n", "up\r\n+ 1\r\n", "up # \u0085\u2028\u2029", "up\n+ 1\n\n", "  up\n+ 1"} {
		s := mustOpen(t, path)
		var err error
		if len(s.Queries()) == 0 {
			_, err = s.Add("14", expression)
		} else {
			_, err = s.Update(0, expression)
		}
		if err != nil {
			t.Fatal(err)
		}
		if got := mustOpen(t, path).Queries()[0]; got != (Query{"14", expression}) {
			t.Errorf("wanted expression %q, got %#v", expression, got)
		}
	}
}

func TestRejectsInvalidChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queries.yaml")
	s, err := mustOpen(t, path).Add("Metrics", "up")
	if err != nil {
		t.Fatal(err)
	}
	s, err = s.Add("OTHER", "down")
	if err != nil {
		t.Fatal(err)
	}
	raw := mustRead(t, path)
	for _, tc := range []struct {
		name string
		edit func() (Snapshot, error)
	}{
		{"duplicate", func() (Snapshot, error) { return s.Add(" metrics ", "up") }},
		{"empty name", func() (Snapshot, error) { return s.Add(" \t", "up") }},
		{"empty expression", func() (Snapshot, error) { return s.Add("Valid", " \t\n") }},
		{"rename duplicate", func() (Snapshot, error) { return s.Rename(0, "other") }},
		{"rename blank", func() (Snapshot, error) { return s.Rename(0, " ") }},
		{"update blank", func() (Snapshot, error) { return s.Update(0, "\n") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.edit(); err == nil || mustRead(t, path) != raw {
				t.Fatalf("invalid edit was accepted or changed the file: %v", err)
			}
		})
	}
	if s, err = s.Rename(0, "METRICS"); err != nil || s.Queries()[0].Name != "METRICS" {
		t.Fatalf("case-only rename failed: %v", err)
	}
}

func TestRejectsCorruptYAML(t *testing.T) {
	for _, raw := range []string{
		"", "queries: [", "queries: null", "queries: {}", "{}",
		"queries: []\n---\nqueries: []\n",
		"queries: [{name: CPU, expression: up}, {name: ' cpu ', expression: up}]",
		"queries: [{name: CPU, expression: true}]",
		"queries: [{name: CPU, expression: up, extra: data}]",
	} {
		path := filepath.Join(t.TempDir(), "queries.yaml")
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(path); err == nil || !strings.Contains(err.Error(), "queries.yaml") {
			t.Errorf("corrupt data %q: %v", raw, err)
		}
		if mustRead(t, path) != raw {
			t.Fatal("reading changed corrupt data")
		}
	}
}

func TestWritesReplaceFileAndSkipIdenticalUpdates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("same-directory rename over an open file is tested on Unix")
	}
	path := filepath.Join(t.TempDir(), "queries.yaml")
	s, err := mustOpen(t, path).Add("CPU", "up")
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	oldInfo, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	check := func(same bool) {
		t.Helper()
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if os.SameFile(oldInfo, info) != same || info.Mode().Perm() != 0600 {
			t.Fatalf("unexpected replacement or permissions: %v", info.Mode())
		}
	}
	s, err = s.Update(0, "up")
	if err != nil {
		t.Fatal(err)
	}
	check(true)
	if _, err := s.Update(0, "down"); err != nil {
		t.Fatal(err)
	}
	check(false)
	if raw, err := io.ReadAll(f); err != nil || !strings.Contains(string(raw), "expression: up") {
		t.Fatalf("original open file should still have its old contents: %q, %v", raw, err)
	}
}

func TestFailedWriteLeavesOriginalAndCleansTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "queries.yaml")
	s, err := mustOpen(t, path).Add("CPU", "up")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a change between opening the dialog and confirming it.
	const external = "queries: ["
	if err := os.WriteFile(path, []byte(external), 0600); err != nil {
		t.Fatal(err)
	}
	next, err := s.Delete(0)
	if !errors.Is(err, ErrChanged) || mustRead(t, path) != external || !slices.Equal(next.Queries(), s.Queries()) {
		t.Fatalf("unexpected concurrent edit result: %v", err)
	}
	if files, err := os.ReadDir(dir); err != nil || len(files) != 1 {
		t.Fatalf("temporary file leaked: %v, %v", files, err)
	}

	// A destination that becomes a directory cannot be replaced.
	path = filepath.Join(dir, "other.yaml")
	s = mustOpen(t, path)
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add("CPU", "up"); err == nil {
		t.Fatal("expected a filesystem error")
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatalf("write changed the destination directory: %v, %v", info, err)
	}
}
