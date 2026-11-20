package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	c, err := Load(path, true)
	if err != nil || c.Connections[0].URL != "http://localhost:9090" || c.Connections[0].AlertmanagerURL != "" {
		t.Fatal(c, err)
	}
	if _, err := Load(path, false); err == nil {
		t.Fatal("missing explicit config accepted")
	}
	body := `[general]
default_connection="vm"
history_limit=0
[query]
default_range="6h"
max_series=3
[[connections]]
name="vm"
url="http://example.test/select/0/prometheus"
alertmanager_url="https://alerts.example.test/proxy"`
	if err = os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	c, err = Load(path, false)
	if err != nil || c.General.History != 0 || c.Query.Range != 6*time.Hour || c.Query.MaxSeries != 3 || c.Query.Timeout != 30*time.Second {
		t.Fatal(c, err)
	}
	if got := c.Connections[0].AlertmanagerURL; got != "https://alerts.example.test/proxy" {
		t.Fatal(got)
	}
}
