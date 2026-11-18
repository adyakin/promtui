package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Connection struct{ Name, URL string }

type Config struct {
	General struct {
		Default string `toml:"default_connection"`
		History int    `toml:"history_limit"`
	}
	Query struct {
		Timeout   time.Duration
		Range     time.Duration `toml:"default_range"`
		MaxSeries int           `toml:"max_series"`
	}
	Connections []Connection
}

func Load(path string, optional bool) (Config, error) {
	var c Config
	c.General.History = 1000
	c.Query.Timeout, c.Query.Range, c.Query.MaxSeries = 30*time.Second, time.Hour, 500
	_, err := toml.DecodeFile(path, &c)
	if err != nil && !(optional && os.IsNotExist(err)) {
		return c, err
	}
	if len(c.Connections) == 0 {
		c.Connections = []Connection{{"local", "http://localhost:9090"}}
	}
	if c.General.Default == "" {
		c.General.Default = c.Connections[0].Name
	}
	seen := map[string]bool{}
	for _, conn := range c.Connections {
		u, err := url.Parse(conn.URL)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.TrimSpace(conn.Name) == "" || seen[conn.Name] {
			return c, fmt.Errorf("invalid or duplicate connection %q (expected a name and HTTP(S) base URL without credentials, query or fragment)", conn.Name)
		}
		seen[conn.Name] = true
	}
	if !seen[c.General.Default] || c.Query.MaxSeries < 1 || c.General.History < 0 || c.Query.Range <= 0 || c.Query.Timeout <= 0 {
		return c, fmt.Errorf("invalid default connection, query settings or history limit")
	}
	return c, nil
}
