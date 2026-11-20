package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Connection struct {
	Name, URL          string
	AlertmanagerURL    string
	InsecureSkipVerify bool
}

type generalSettings struct {
	Default string `toml:"default_connection" yaml:"default_connection"`
	History int    `toml:"history_limit" yaml:"history_limit"`
}

type querySettings struct {
	Timeout   time.Duration `toml:"timeout" yaml:"timeout"`
	Range     time.Duration `toml:"default_range" yaml:"default_range"`
	MaxSeries int           `toml:"max_series" yaml:"max_series"`
}

type settings struct {
	General generalSettings `toml:"general" yaml:"general"`
	Query   querySettings   `toml:"query" yaml:"query"`
}

type Config struct {
	settings
	Connections []Connection
}

type rawConnection struct {
	Name               string `toml:"name"`
	URL                string `toml:"url"`
	AlertmanagerURL    string `toml:"alertmanager_url"`
	InsecureSkipVerify *bool  `toml:"insecure_skip_verify"`
}

func defaults() settings {
	return settings{
		General: generalSettings{History: 1000},
		Query:   querySettings{Timeout: 30 * time.Second, Range: time.Hour, MaxSeries: 500},
	}
}

// LoadDefault reads the first available configuration in dir.
func LoadDefault(dir string) (Config, error) {
	for _, name := range []string{"config.yaml", "config.yml", "config.toml"} {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return Config{}, err
		}
		return decode(path, data)
	}
	return resolve(defaults(), false, nil)
}

func Load(path string, optional bool) (Config, error) {
	data, err := os.ReadFile(path)
	if optional && os.IsNotExist(err) {
		return resolve(defaults(), false, nil)
	}
	if err != nil {
		return Config{}, err
	}
	return decode(path, data)
}

func decode(path string, data []byte) (Config, error) {
	s := defaults()
	var global bool
	var connections []rawConnection
	var err error
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		s, global, connections, err = decodeYAML(data, s)
	case ".toml":
		raw := struct {
			settings
			InsecureSkipVerify bool            `toml:"insecure_skip_verify"`
			Connections        []rawConnection `toml:"connections"`
		}{settings: s}
		_, err = toml.Decode(string(data), &raw)
		s, global, connections = raw.settings, raw.InsecureSkipVerify, raw.Connections
	default:
		err = fmt.Errorf("unsupported configuration extension (expected .yaml, .yml, or .toml)")
	}
	var c Config
	if err == nil {
		c, err = resolve(s, global, connections)
	}
	if err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}
	return c, nil
}

func resolve(s settings, insecure bool, raw []rawConnection) (Config, error) {
	if len(raw) == 0 {
		raw = []rawConnection{{Name: "local", URL: "http://localhost:9090"}}
	}
	c := Config{settings: s}
	if c.General.Default == "" {
		c.General.Default = raw[0].Name
	}
	seen := map[string]bool{}
	for _, conn := range raw {
		if strings.TrimSpace(conn.Name) == "" || seen[conn.Name] {
			return c, fmt.Errorf("missing or duplicate connection name %q", conn.Name)
		}
		if !validBaseURL(conn.URL) {
			return c, fmt.Errorf("connection %q: url must be an HTTP(S) base URL without credentials, query or fragment", conn.Name)
		}
		if conn.AlertmanagerURL != "" && !validBaseURL(conn.AlertmanagerURL) {
			return c, fmt.Errorf("connection %q: alertmanager_url must be an HTTP(S) base URL without credentials, query or fragment", conn.Name)
		}
		skipVerify := insecure
		if conn.InsecureSkipVerify != nil {
			skipVerify = *conn.InsecureSkipVerify
		}
		c.Connections = append(c.Connections, Connection{
			Name: conn.Name, URL: conn.URL, AlertmanagerURL: conn.AlertmanagerURL, InsecureSkipVerify: skipVerify,
		})
		seen[conn.Name] = true
	}
	switch {
	case !seen[c.General.Default]:
		return c, fmt.Errorf("general.default_connection %q is not configured", c.General.Default)
	case c.General.History < 0:
		return c, fmt.Errorf("general.history_limit must not be negative")
	case c.Query.MaxSeries < 1:
		return c, fmt.Errorf("query.max_series must be positive")
	case c.Query.Range <= 0:
		return c, fmt.Errorf("query.default_range must be positive")
	case c.Query.Timeout <= 0:
		return c, fmt.Errorf("query.timeout must be positive")
	}
	return c, nil
}

func validBaseURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Hostname() != "" && (u.Scheme == "http" || u.Scheme == "https") &&
		u.User == nil && !u.ForceQuery && u.RawQuery == "" && !strings.Contains(value, "#")
}
