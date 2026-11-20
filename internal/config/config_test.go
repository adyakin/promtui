package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func writeConfig(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadYAML(t *testing.T) {
	path := writeConfig(t, t.TempDir(), "custom.yml", `
general:
  history_limit: 0
query:
  default_range: 6h
  max_series: 3
connections:
  zebra:
    url: http://example.test/select/0/prometheus
    alertmanager_url: https://alerts.example.test/proxy
  alpha:
    url: https://example.test
`)
	c, err := Load(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if c.General.Default != "zebra" || c.General.History != 0 || c.Query.Range != 6*time.Hour ||
		c.Query.MaxSeries != 3 || c.Query.Timeout != 30*time.Second || len(c.Connections) != 2 ||
		c.Connections[0].AlertmanagerURL != "https://alerts.example.test/proxy" {
		t.Fatalf("unexpected configuration: %+v", c)
	}
}

func TestLoadDefault(t *testing.T) {
	dir := t.TempDir()
	check := func(want string) {
		t.Helper()
		c, err := LoadDefault(dir)
		if err != nil || c.General.Default != want {
			t.Fatalf("default connection = %q, err = %v; want %q", c.General.Default, err, want)
		}
	}
	check("local")
	tomlPath := writeConfig(t, dir, "config.toml", "[[connections]]\nname = \"toml\"\nurl = \"https://example.test\"\n")
	check("toml")
	writeConfig(t, dir, "config.yml", "connections:\n  yml:\n    url: https://example.test\n")
	check("yml")
	yamlPath := writeConfig(t, dir, "config.yaml", "connections:\n  yaml:\n    url: https://example.test\n")
	check("yaml")
	writeConfig(t, dir, "config.yaml", "connections: [")
	if _, err := LoadDefault(dir); err == nil || !strings.Contains(err.Error(), yamlPath) {
		t.Fatalf("expected error from preferred config, got %v", err)
	}
	if c, err := Load(tomlPath, false); err != nil || c.General.Default != "toml" {
		t.Fatalf("explicit config was not used: %+v, %v", c, err)
	}
}

func TestConfigValidation(t *testing.T) {
	for _, tc := range []struct{ name, file, body, want string }{
		{"syntax", "config.yaml", "connections: [", "yaml:"},
		{"type", "config.yaml", "query:\n  max_series: many", "cannot unmarshal"},
		{"duration", "config.yaml", "query:\n  timeout: later", "time.Duration"},
		{"wrong layout", "config.yaml", "connections:\n  - name: local", "must be a mapping"},
		{"wrong format", "config.yaml", "[general]\nhistory_limit = 0", "yaml:"},
		{"unknown extension", "config.txt", "", "unsupported configuration extension"},
		{"missing URL", "config.yaml", "connections:\n  test: {}", "test\": url"},
		{"invalid URL", "config.yaml", "connections:\n  test:\n    url: https://example.test?", "test\": url"},
		{"invalid alert URL", "config.yaml", "connections:\n  test:\n    url: https://example.test\n    alertmanager_url: ftp://example.test", "alertmanager_url"},
		{"invalid default", "config.yaml", "general:\n  default_connection: missing", "default_connection"},
		{"duplicate", "config.yaml", "connections:\n  test: {url: https://example.test}\n  test: {url: https://example.test}", "duplicate connection"},
		{"global TLS string", "config.yaml", `insecure_skip_verify: "maybe"`, "insecure_skip_verify"},
		{"global TLS null", "config.yaml", `insecure_skip_verify: null`, "insecure_skip_verify"},
		{"connection TLS string", "config.yaml", "connections:\n  test:\n    url: https://example.test\n    insecure_skip_verify: \"true\"", "insecure_skip_verify"},
		{"TOML TLS string", "config.toml", `insecure_skip_verify = "maybe"`, "insecure_skip_verify"},
		{"extra document", "config.yaml", "general: {}\n---\nquery: {}", "single YAML document"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeConfig(t, t.TempDir(), tc.file, tc.body)
			if _, err := Load(path, false); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestTLSPrecedence(t *testing.T) {
	for _, ext := range []string{"yaml", "toml"} {
		for _, global := range []string{"", "false", "true"} {
			for _, override := range []string{"", "false", "true"} {
				t.Run(fmt.Sprintf("%s/global=%s/override=%s", ext, global, override), func(t *testing.T) {
					var body string
					if ext == "yaml" {
						if global != "" {
							body = "insecure_skip_verify: " + global + "\n"
						}
						body += "connections:\n  test:\n    url: https://example.test\n"
						if override != "" {
							body += "    insecure_skip_verify: " + override + "\n"
						}
					} else {
						if global != "" {
							body = "insecure_skip_verify = " + global + "\n"
						}
						body += "[[connections]]\nname = \"test\"\nurl = \"https://example.test\"\n"
						if override != "" {
							body += "insecure_skip_verify = " + override + "\n"
						}
					}
					c, err := Load(writeConfig(t, t.TempDir(), "config."+ext, body), false)
					if err != nil {
						t.Fatal(err)
					}
					want := override == "true" || (override == "" && global == "true")
					if got := c.Connections[0].InsecureSkipVerify; got != want {
						t.Fatalf("insecure_skip_verify = %t, want %t", got, want)
					}
				})
			}
		}
	}
}
