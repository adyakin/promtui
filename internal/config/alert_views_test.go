package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func makeAlertViewsDir(t *testing.T, configDir string) string {
	t.Helper()
	dir := filepath.Join(configDir, "alert_views.d")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadNamedAlertViews(t *testing.T) {
	dir := t.TempDir()
	viewsDir := makeAlertViewsDir(t, dir)
	path := writeConfig(t, dir, "custom.yml", `
alerts:
  views:
    default:
      columns:
        - {source: state, width: 10}
        - {source: labels.app.service, weight: 3, title: Service}
        - {source: duration}
connections:
  prometheus:
    url: https://prom.test
    alerts:
      default_view: default
      columns: [alertname, {field: labels.instance, title: Host}]
      search: [labels.alertname, labels.app.service]
  victoria:
    url: https://vm.test/select/0/prometheus
    alerts:
      default_view: certificates
  plain:
    url: https://plain.test
`)
	writeConfig(t, viewsDir, "20-certs.yaml", `
views:
  certificates:
    columns:
      - source: annotations.summary
        title: Expires in
        extract: 'expires in (.+?)\.?$'
        weight: 2
      - source: annotations.summary
        title: Subject
        extract: '^(.+) expires in'
`)
	writeConfig(t, viewsDir, "10-postgres.yaml", `views: {postgresql: {columns: [{source: labels.instance}]}}`)
	writeConfig(t, viewsDir, "ignored.yml", "not: [yaml")
	nestedDir := filepath.Join(viewsDir, "nested.yaml")
	if err := os.Mkdir(nestedDir, 0700); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, nestedDir, "ignored.yaml", "not: [yaml")
	saved := writeConfig(t, dir, "alerts.yaml", "saved filters, not view config")

	cfg, err := Load(path, false)
	if err != nil {
		t.Fatal(err)
	}
	wantDefault := TableView{Columns: []Column{
		{Field: "state", Width: 10}, {Field: "labels.app.service", Weight: 3, Title: "Service"}, {Field: "duration"},
	}}
	if len(cfg.AlertViews) != 3 || !reflect.DeepEqual(cfg.AlertViews["default"], wantDefault) ||
		!reflect.DeepEqual(cfg.AlertViews["postgresql"], TableView{Columns: []Column{{Field: "labels.instance"}}}) {
		t.Fatalf("unexpected registry: %#v", cfg.AlertViews)
	}
	certs := cfg.AlertViews["certificates"].Columns
	if len(certs) != 2 || certs[0].Field != "annotations.summary" || certs[0].Title != "Expires in" ||
		certs[0].Weight != 2 || certs[0].Extract == nil {
		t.Fatalf("unexpected certificate columns: %#v", certs)
	}
	if got := certs[0].Extract.FindStringSubmatch("Certificate expires in 2 weeks."); !reflect.DeepEqual(got, []string{"expires in 2 weeks.", "2 weeks"}) {
		t.Fatalf("unexpected extract match: %v", got)
	}
	if prom, vm, plain := cfg.Connections[0], cfg.Connections[1], cfg.Connections[2]; prom.AlertDefaultView != "default" ||
		vm.AlertDefaultView != "certificates" || plain.AlertDefaultView != "" ||
		!reflect.DeepEqual(prom.Alerts, TableView{
			Columns: []Column{{Field: "alertname"}, {Field: "labels.instance", Title: "Host"}},
			Search:  []string{"labels.alertname", "labels.app.service"},
		}) || vm.Alerts.Columns != nil || vm.Alerts.Search != nil {
		t.Fatalf("unexpected connections: %#v", cfg.Connections)
	}
	if data, err := os.ReadFile(saved); err != nil || string(data) != "saved filters, not view config" {
		t.Fatalf("saved alerts affected: %q, %v", data, err)
	}
}

func TestAlertViewExternalDiscoveryFallbacks(t *testing.T) {
	dir := t.TempDir()
	viewsDir := makeAlertViewsDir(t, dir)
	if cfg, err := LoadDefault(dir); err != nil || len(cfg.AlertViews) != 0 {
		t.Fatalf("empty directory: %#v, %v", cfg, err)
	}
	writeConfig(t, viewsDir, "only.yaml", `views: {external: {columns: [{source: state}]}}`)
	for _, load := range []func() (Config, error){
		func() (Config, error) { return LoadDefault(dir) },
		func() (Config, error) { return Load(filepath.Join(dir, "missing.yml"), true) },
	} {
		cfg, err := load()
		if err != nil || cfg.General.Default != "local" || len(cfg.AlertViews) != 1 {
			t.Fatalf("default config: %#v, %v", cfg, err)
		}
	}
	writeConfig(t, dir, "config.toml", "[[connections]]\nname = 'toml'\nurl = 'https://example.test'\n")
	cfg, err := LoadDefault(dir)
	if err != nil || cfg.General.Default != "toml" || len(cfg.AlertViews) != 1 || cfg.Connections[0].AlertDefaultView != "" {
		t.Fatalf("TOML fallback: %#v, %v", cfg, err)
	}
}

func TestAlertViewDuplicateOrigins(t *testing.T) {
	const definition = `same: {columns: [{source: state}]}`
	for _, tc := range []struct{ name, main, first, second string }{
		{"inline and external", "alerts: {views: {" + definition + "}}", "config.yaml:1 (alerts.views)", "a.yaml:1 (views)"},
		{"external order", "", "a.yaml:1 (views)", "z.yaml:1 (views)"},
		{"same file", "alerts:\n  views:\n    " + definition + "\n    " + definition, "config.yaml:3 (alerts.views)", "config.yaml:4 (alerts.views)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			viewsDir := makeAlertViewsDir(t, dir)
			path := writeConfig(t, dir, "config.yaml", tc.main)
			writeConfig(t, viewsDir, "z.yaml", "views: {"+definition+"}")
			writeConfig(t, viewsDir, "a.yaml", "views: {"+definition+"}")
			_, err := Load(path, false)
			if err == nil || !strings.Contains(err.Error(), `duplicate alert view "same"`) {
				t.Fatalf("unexpected error: %v", err)
			}
			first, second := strings.Index(err.Error(), tc.first), strings.Index(err.Error(), tc.second)
			if first < 0 || second <= first {
				t.Fatalf("expected duplicate origins %q then %q, got %v", tc.first, tc.second, err)
			}
		})
	}
}

func TestNamedAlertViewValidation(t *testing.T) {
	for _, tc := range []struct{ value, want string }{
		{`null`, "expected a mapping"},
		{`{}`, "missing columns"},
		{`{columns: []}`, "columns: expected a nonempty list"},
		{`{columns: [state]}`, "columns[0]: expected a mapping with a source"},
		{`{columns: [{title: Name}]}`, "columns[0]: missing source"},
		{`{columns: [{source: 42}]}`, "source must be a string"},
		{`{columns: [{source: ''}]}`, "source must not be empty"},
		{`{columns: [{source: health}]}`, `unknown source "health"`},
		{`{columns: [{source: labels.}]}`, "must specify a name after labels."},
		{`{columns: [{source: annotations.}]}`, "must specify a name after annotations."},
		{`{columns: [{source: state, title: ' '}]}`, "title must not be empty"},
		{`{columns: [{source: state, width: 5, weight: 2}]}`, "width and weight are mutually exclusive"},
		{`{columns: [{source: state, width: 0}]}`, "width must be a positive integer"},
		{`{columns: [{source: state, width: 5.1}]}`, "width must be a positive integer"},
		{`{columns: [{source: state, weight: -2}]}`, "weight must be a positive integer"},
		{`{columns: [{source: state, weight: '2'}]}`, "weight must be a positive integer"},
		{`{columns: [{source: state, weight: 99999999999999999999}]}`, "weight must be a positive integer"},
		{`{columns: [{source: labels.instance, extract: '('}]}`, "invalid extract regex"},
		{`{columns: [{source: labels.instance, extract: '[0-9]+'}]}`, "exactly one capture group"},
		{`{columns: [{source: labels.instance, extract: '(.+):(.+)'}]}`, "exactly one capture group"},
		{`{columns: [{source: state, width: 3, width: 4}]}`, `duplicate column property "width"`},
		{`{columns: [{source: state, field: duration}]}`, `unknown column property "field"`},
		{`{columns: [{source: state}], search: [state]}`, `unknown property "search"`},
	} {
		t.Run(tc.value, func(t *testing.T) {
			path := writeConfig(t, t.TempDir(), "config.yaml", "alerts:\n  views:\n    bad: "+tc.value+"\n")
			_, err := Load(path, false)
			if err == nil || !strings.Contains(err.Error(), `alerts.views["bad"]`) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want view context and %q", err, tc.want)
			}
		})
	}
}

func TestExternalAlertViewErrors(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{"views: [", "yaml:"},
		{"views: {}\n---\nviews: {}", "single YAML document"},
		{"views: {}\ninclude: other.yaml", "field include not found"},
		{"views: []", "expected a mapping of view names"},
		{`views: {bad: {columns: [{source: state, extract: '('}]}}`, `views["bad"]: columns[0]: invalid extract regex`},
	} {
		t.Run(tc.body, func(t *testing.T) {
			dir := t.TempDir()
			file := writeConfig(t, makeAlertViewsDir(t, dir), "invalid.yaml", tc.body)
			_, err := LoadDefault(dir)
			if err == nil || !strings.Contains(err.Error(), file) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q and %q", err, file, tc.want)
			}
		})
	}
}

func TestAlertDefaultViewValidation(t *testing.T) {
	for _, tc := range []struct{ alerts, want string }{
		{"{default_view: absent}", `alerts.default_view "absent" is not defined`},
		{"{default_view: true}", "alerts: default_view must be a string"},
		{"{default_view: ''}", "alerts.default_view must not be empty"},
		{"{default_view: first, default_view: second}", "alerts.default_view: duplicate property"},
	} {
		t.Run(tc.alerts, func(t *testing.T) {
			path := writeConfig(t, t.TempDir(), "config.yaml", "connections:\n  test:\n    url: https://example.test\n    alerts: "+tc.alerts)
			_, err := Load(path, false)
			if err == nil || !strings.Contains(err.Error(), `connection "test": `+tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}
