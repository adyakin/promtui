package main

import (
	"path/filepath"
	"testing"
)

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
