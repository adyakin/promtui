package savedalerts

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"promtui/internal/parameters"
)

func TestCompileAlertmanagerSemantics(t *testing.T) {
	tests := []struct {
		name     string
		matchers []Matcher
		labels   map[string]string
		want     bool
	}{
		{"clear", nil, nil, true},
		{"exact", []Matcher{{"service", "=", "a.b"}}, map[string]string{"service": "a.b"}, true},
		{"literal not regex", []Matcher{{"service", "=", "a.b"}}, map[string]string{"service": "axb"}, false},
		{"inequality", []Matcher{{"service", "!=", "a"}}, map[string]string{"service": "a"}, false},
		{"regex", []Matcher{{"service", "=~", "okd-.*"}}, map[string]string{"service": "okd-prod"}, true},
		{"anchored", []Matcher{{"service", "=~", "okd-.*"}}, map[string]string{"service": "not-okd-prod"}, false},
		{"negative regex", []Matcher{{"service", "!~", "okd-.*"}}, map[string]string{"service": "okd-prod"}, false},
		{"missing is empty", []Matcher{{"service", "=", ""}}, nil, true},
		{"missing satisfies negative", []Matcher{{"service", "!~", ".+"}}, nil, true},
		{"and", []Matcher{{"service", "=", "a"}, {"cluster", "!=", ""}}, map[string]string{"service": "a"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, err := Compile(tt.matchers)
			if err != nil {
				t.Fatal(err)
			}
			if got := match(tt.labels); got != tt.want {
				t.Fatalf("match = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	defaultPattern := "okd.prod"
	filter := Filter{
		Name: "Production",
		Matchers: []Matcher{
			{"service", "=~", "${pattern}"},
			{"description", "=", "$${static} ${note}"},
		},
		Parameters: []parameters.Parameter{{Name: "pattern", Default: &defaultPattern}, {Name: "note"}},
	}
	if _, _, err := filter.Resolve(nil); err == nil || !strings.Contains(err.Error(), "note") {
		t.Fatalf("missing required value: %v", err)
	}
	if resolved, match, err := filter.Resolve(parameters.Values{"note": "", "pattern": "["}); err == nil || resolved != nil || match != nil {
		t.Fatalf("invalid resolved regex not rejected: %v", err)
	}
	resolved, match, err := filter.Resolve(parameters.Values{"note": "${other}"})
	if err != nil {
		t.Fatal(err)
	}
	if !match(map[string]string{"service": "okd-prod", "description": "${static} ${other}"}) {
		t.Fatalf("values were escaped or recursively substituted: %+v", resolved)
	}
	resolved[0].Value = "changed"
	if filter.Matchers[0].Value != "${pattern}" {
		t.Fatal("Resolve mutated the template")
	}
	_, match, err = filter.Resolve(parameters.Values{"note": "", "pattern": ""})
	if err != nil || !match(map[string]string{"description": "${static} "}) {
		t.Fatalf("supplied empty values should override defaults and satisfy required values: %v", err)
	}
}

func TestOpenRejectsInvalidYAML(t *testing.T) {
	const valid = `alerts:
  - name: Production
    matchers:
      - {label: service, operator: '=', value: ''}
`
	tests := []struct{ name, input, want string }{
		{"syntax", "alerts: [", "invalid alerts.yaml"},
		{"empty", "", "invalid alerts.yaml"},
		{"null root", "null", "expected an alerts list"},
		{"null list", "alerts: null", "must be a list"},
		{"null entry", "alerts: [null]", "expected a mapping"},
		{"second document", valid + "---\nalerts: []\n", "only one YAML document"},
		{"unknown root field", valid + "queries: []\n", "unexpected saved alerts field"},
		{"duplicate root field", valid + "alerts: []\n", "duplicate saved alerts field"},
		{"unknown filter field", valid + "    status: active\n", "unexpected filter field"},
		{"duplicate filter name", valid + strings.ReplaceAll(strings.TrimPrefix(valid, "alerts:\n"), "Production", "' production '"), "already exists"},
		{"blank name", strings.ReplaceAll(valid, "Production", "'  '"), "enter a name"},
		{"invalid label", strings.ReplaceAll(valid, "label: service", "label: ''"), "valid, non-empty label name"},
		{"unknown operator", strings.ReplaceAll(valid, "operator: '='", "operator: '=='"), "unsupported operator"},
		{"nonstring value", strings.ReplaceAll(valid, "value: ''", "value: false"), "value must be a string"},
		{"missing value", strings.ReplaceAll(valid, ", value: ''", ""), "missing matcher field"},
		{"duplicate value", strings.ReplaceAll(valid, "value: ''", "value: '', value: ''"), "duplicate matcher field"},
		{"unknown reference", strings.ReplaceAll(valid, "value: ''", "value: '${other}'"), "undefined parameter"},
		{"unused parameter", valid + "    parameters: [{name: other}]\n", "unused parameter"},
		{"duplicate parameter", valid + "    parameters: [{name: other}, {name: other}]\n", "duplicate parameter"},
		{"malformed parameter", strings.ReplaceAll(valid, "value: ''", "value: '${other'"), "unclosed parameter reference"},
		{"null parameter", valid + "    parameters: [null]\n", "expected a mapping"},
		{"invalid default", valid + "    parameters: [{name: other, default: false}]\n", "default must be a string"},
		{"empty matchers", "alerts: [{name: x, matchers: []}]", "at least one matcher"},
		{"static regex in mixed filter", valid + "      - {label: service, operator: '=~', value: '['}\n" +
			"      - {label: cluster, operator: '=~', value: '${pattern}'}\n" +
			"    parameters: [{name: pattern}]\n", "matcher 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "alerts.yaml")
			if err := os.WriteFile(path, []byte(tt.input), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Open(path); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Open error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestSnapshotLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", "alerts.yaml")
	s, err := Open(path)
	if err != nil || len(s.Filters()) != 0 {
		t.Fatalf("missing file should be empty: %v", err)
	}
	rawValue := "  leading \n\u0085middle\r\u2028line\u2029 \t\n${looks_like_a_ref}\n\n"
	rawDefault := "  default\n\u0085middle\r\u2028line\u2029 \t\n"
	emptyDefault := ""
	input := Filter{
		Name: "  original  ",
		Matchers: []Matcher{
			{"service", "=", parameters.EscapeLiteral(rawValue)},
			{"yes", "=", "${pattern}"},
			{"true", "=", "${empty}"},
			{"odd label", "=", "${required}"},
		},
		Parameters: []parameters.Parameter{
			{Name: "pattern", Default: &rawDefault},
			{Name: "empty", Default: &emptyDefault},
			{Name: "required"},
		},
	}
	added, err := s.Add(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Filters()) != 0 {
		t.Fatal("Add mutated the original snapshot")
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("file should be private: %v, %v", info, err)
	}
	input.Matchers[0].Value = "mutated input"
	*input.Parameters[0].Default = "mutated input default"
	copy := added.Filters()
	copy[0].Matchers[0].Value = "mutated result"
	*copy[0].Parameters[0].Default = "mutated result default"
	current, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(current.Filters(), added.Filters()) {
		t.Fatalf("snapshot did not round-trip or was mutated through an alias: %#v vs %#v", current.Filters(), added.Filters())
	}
	got := current.Filters()[0]
	resolved, _, err := got.Resolve(parameters.Values{"required": "false"})
	if err != nil || resolved[0].Value != rawValue || resolved[1].Value == "mutated input default" ||
		resolved[1].Value != "  default\n\u0085middle\r\u2028line\u2029 \t\n" ||
		resolved[2].Value != "" || resolved[3].Value != "false" || got.Name != "original" {
		t.Fatalf("raw values/defaults not preserved: %+v, %v", resolved, err)
	}

	// A standalone filter is the schema used by the template editor.
	editor, err := yaml.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Filter
	if err := yaml.Unmarshal(editor, &decoded); err != nil || !reflect.DeepEqual(decoded, got) {
		t.Fatalf("template editor YAML did not round-trip: %s (%v)", editor, err)
	}

	second := Filter{Name: "Second", Matchers: []Matcher{{"service", "=", "false"}}}
	current, err = current.Add(second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := current.Rename(0, " sECONd  "); err == nil {
		t.Fatal("rename should reject a duplicate name")
	}
	current, err = current.Rename(0, " Renamed ")
	if err != nil || current.Filters()[0].Name != "Renamed" {
		t.Fatalf("rename failed: %v", err)
	}
	current, err = current.Update(1, Filter{Name: "Second", Matchers: []Matcher{{"service", "!=", "false"}}})
	if err != nil {
		t.Fatal(err)
	}
	got.Name = "Renamed"
	if !reflect.DeepEqual(current.Filters()[0], got) {
		t.Fatal("updating another filter changed the renamed template")
	}
	for range 2 {
		current, err = current.Delete(0)
		if err != nil {
			t.Fatal(err)
		}
	}
	reopened, err := Open(path)
	if err != nil || len(reopened.Filters()) != 0 {
		t.Fatalf("deleted filters were persisted incorrectly: %v", err)
	}
}

func TestWriteValidationAndVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alerts.yaml")
	const external = `# managed externally
alerts:
  - name: Existing
    parameters: [{name: service, default: "["}]
    matchers:
      - label: service
        operator: '=~'
        value: '${service}'
`
	if err := os.WriteFile(path, []byte(external), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Filters()[0].Resolve(nil); err == nil {
		t.Fatal("invalid regex default should be rejected only on resolution")
	}
	for _, filter := range []Filter{
		{Name: "existing", Matchers: []Matcher{{"service", "=", ""}}},
		{Name: "new", Matchers: []Matcher{{"service", "=~", "["}}},
		{Name: "new", Matchers: []Matcher{{"service", "=", "${typo}"}}},
	} {
		if _, err := s.Add(filter); err == nil {
			t.Fatalf("invalid Add succeeded: %+v", filter)
		}
	}
	if _, err := s.Update(0, Filter{Name: "Existing", Matchers: []Matcher{{"service", "!~", "["}}}); err == nil {
		t.Fatal("invalid Update succeeded")
	}
	assertFile := func(want string) {
		t.Helper()
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("file changed: %q, %v", got, err)
		}
	}
	assertFile(external)
	const replacement = "alerts: []\n"
	if err := os.WriteFile(path, []byte(replacement), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Delete(0); !errors.Is(err, ErrChanged) {
		t.Fatalf("expected stale snapshot error, got %v", err)
	}
	assertFile(replacement)
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("failed update should leave no temporary file: %v, %v", entries, err)
	}
}
