package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"
)

func TestSavedQueriesPath(t *testing.T) {
	configDir, dataDir := defaultDirs(t.TempDir())
	current, legacy := filepath.Join(configDir, "queries.yaml"), filepath.Join(dataDir, "queries.yaml")
	if got := savedQueriesPath(configDir, dataDir); got != current {
		t.Fatalf("new collection path = %q, want %q", got, current)
	}
	for _, path := range []string{legacy, current} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("queries: []\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if got := savedQueriesPath(configDir, dataDir); got != path {
			t.Fatalf("existing collection path = %q, want %q", got, path)
		}
	}
}

func TestDefaultDirs(t *testing.T) {
	home := filepath.Join("home", "test")
	configDir, dataDir := defaultDirs(home)
	if want := filepath.Join(home, ".config", "promtui"); configDir != want {
		t.Errorf("config dir = %q, want %q", configDir, want)
	}
	if want := filepath.Join(home, ".local", "share", "promtui"); dataDir != want {
		t.Errorf("data dir = %q, want %q", dataDir, want)
	}
}

func TestVersionCommands(t *testing.T) {
	previous := version
	version = "v1.2.3"
	t.Cleanup(func() { version = previous })
	t.Setenv("HOME", "")

	for _, args := range [][]string{{"--version"}, {"-version"}, {"version"}, {"-config", "missing.yaml", "--version"}} {
		var out bytes.Buffer
		if err := run(args, &out); err != nil {
			t.Fatalf("run(%v): %v", args, err)
		}
		if got, want := out.String(), "promtui v1.2.3\n"; got != want {
			t.Errorf("run(%v) printed %q, want %q", args, got, want)
		}
	}
}

func TestBuildVersion(t *testing.T) {
	info := &debug.BuildInfo{}
	if got := buildVersion("", info); got != "devel" {
		t.Errorf("build without metadata = %q, want devel", got)
	}
	info.Main.Version = "(devel)"
	info.Settings = []debug.BuildSetting{
		{Key: "vcs.revision", Value: "0123456789abcdef"},
		{Key: "vcs.modified", Value: "true"},
	}
	if got := buildVersion("", info); got != "devel+0123456789ab-dirty" {
		t.Errorf("dirty build = %q, want devel+0123456789ab-dirty", got)
	}
	info.Main.Version = "v1.2.2"
	if got := buildVersion("", info); got != "v1.2.2" {
		t.Errorf("versioned module = %q, want v1.2.2", got)
	}
}
