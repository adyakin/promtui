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
	Targets            []TargetSource
	VMAlert            []RuleSource
	Alerts             TableView
	AlertDefaultView   string
	TargetsView        TableView
}

type EndpointSource struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

type TargetSource = EndpointSource
type RuleSource = EndpointSource

func (c Connection) RuleSources() []RuleSource {
	if len(c.VMAlert) != 0 {
		return c.VMAlert
	}
	return []RuleSource{{Name: c.Name, URL: c.URL}}
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

type terminalSettings struct {
	ProgramStatus bool `toml:"program_status" yaml:"program_status"`
}

type uiSettings struct {
	Mouse bool `toml:"mouse" yaml:"mouse"`
}

type settings struct {
	SilenceCreator string           `toml:"silence_creator" yaml:"silence_creator"`
	General        generalSettings  `toml:"general" yaml:"general"`
	Query          querySettings    `toml:"query" yaml:"query"`
	Terminal       terminalSettings `toml:"terminal" yaml:"terminal"`
	UI             uiSettings       `toml:"ui" yaml:"ui"`
}

type Config struct {
	settings
	Connections []Connection
	AlertViews  map[string]TableView
}

type rawConnection struct {
	Name               string         `toml:"name"`
	URL                string         `toml:"url"`
	AlertmanagerURL    string         `toml:"alertmanager_url"`
	InsecureSkipVerify *bool          `toml:"insecure_skip_verify"`
	Targets            []TargetSource `toml:"-"`
	VMAlert            []RuleSource   `toml:"-"`
	Alerts             TableView      `toml:"-"`
	AlertDefaultView   string         `toml:"-"`
	TargetsView        TableView      `toml:"-"`
}

func defaults() settings {
	return settings{
		General:  generalSettings{History: 1000},
		Query:    querySettings{Timeout: 30 * time.Second, Range: time.Hour, MaxSeries: 500},
		Terminal: terminalSettings{ProgramStatus: true},
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
	return resolveConfig(dir, defaults(), false, nil, alertViewRegistry{})
}

func Load(path string, optional bool) (Config, error) {
	data, err := os.ReadFile(path)
	if optional && os.IsNotExist(err) {
		return resolveConfig(filepath.Dir(path), defaults(), false, nil, alertViewRegistry{})
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
	var views alertViewRegistry
	var err error
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		s, global, connections, err = decodeYAML(path, data, s, &views)
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
		c, err = resolveConfig(filepath.Dir(path), s, global, connections, views)
	}
	if err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}
	return c, nil
}

func resolveConfig(dir string, s settings, insecure bool, raw []rawConnection, views alertViewRegistry) (Config, error) {
	if err := views.loadDir(filepath.Join(dir, "alert_views.d")); err != nil {
		return Config{}, err
	}
	if len(raw) == 0 {
		raw = []rawConnection{{Name: "local", URL: "http://localhost:9090"}}
	}
	c := Config{settings: s, AlertViews: views.views}
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
		if err := validateSources(conn.Targets, "target"); err != nil {
			return c, fmt.Errorf("connection %q: %w", conn.Name, err)
		}
		if err := validateSources(conn.VMAlert, "vmalert"); err != nil {
			return c, fmt.Errorf("connection %q: %w", conn.Name, err)
		}
		if name := conn.AlertDefaultView; name != "" {
			if _, ok := c.AlertViews[name]; !ok {
				return c, fmt.Errorf("connection %q: alerts.default_view %q is not defined in alerts.views or alert_views.d", conn.Name, name)
			}
		}
		skipVerify := insecure
		if conn.InsecureSkipVerify != nil {
			skipVerify = *conn.InsecureSkipVerify
		}
		c.Connections = append(c.Connections, Connection{
			Name: conn.Name, URL: conn.URL, AlertmanagerURL: conn.AlertmanagerURL, InsecureSkipVerify: skipVerify, Targets: conn.Targets,
			Alerts: conn.Alerts, AlertDefaultView: conn.AlertDefaultView, TargetsView: conn.TargetsView, VMAlert: conn.VMAlert,
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

func validateSources(sources []EndpointSource, role string) error {
	names := make(map[string]bool, len(sources))
	for i := range sources {
		source := &sources[i]
		source.Name = strings.TrimSpace(source.Name)
		if source.Name == "" || names[source.Name] {
			return fmt.Errorf("missing or duplicate %s source name %q", role, source.Name)
		}
		if !validBaseURL(source.URL) {
			return fmt.Errorf("%s source %q url must be an HTTP(S) base URL without credentials, query or fragment", role, source.Name)
		}
		names[source.Name] = true
	}
	return nil
}

func validBaseURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Hostname() != "" && (u.Scheme == "http" || u.Scheme == "https") &&
		u.User == nil && !u.ForceQuery && u.RawQuery == "" && !strings.Contains(value, "#")
}
