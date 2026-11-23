package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestYAMLTargetSources(t *testing.T) {
	for _, key := range []string{"vmagent", "targets"} {
		t.Run(key, func(t *testing.T) {
			cfg, err := decode("config.yaml", []byte(`insecure_skip_verify: true
connections:
  prod:
    url: https://query.example.test
    `+key+`:
      - {name: first, url: "https://a.example.test/prefix%2Fsegment"}
      - {name: second, url: "http://b.example.test/proxy/"}
  staging:
    url: https://prom.example.test/proxy
    insecure_skip_verify: false
`))
			if err != nil {
				t.Fatal(err)
			}
			want := []TargetSource{{"first", "https://a.example.test/prefix%2Fsegment"}, {"second", "http://b.example.test/proxy/"}}
			if !reflect.DeepEqual(cfg.Connections[0].Targets, want) || !cfg.Connections[0].InsecureSkipVerify ||
				cfg.Connections[1].Targets != nil || cfg.Connections[1].InsecureSkipVerify {
				t.Fatalf("connections: %+v", cfg.Connections)
			}
		})
	}
}

func TestYAMLTargetSourceKeyConflict(t *testing.T) {
	_, err := decode("config.yaml", []byte(`connections:
  prod:
    url: https://query.test
    vmagent: [{name: a, url: https://a.test}]
    targets: [{name: b, url: https://b.test}]
`))
	if err == nil || !strings.Contains(err.Error(), "use vmagent instead of targets; do not set both") {
		t.Fatalf("error = %v, want conflicting source keys error", err)
	}
}

func TestYAMLTargetSourceValidation(t *testing.T) {
	for _, tc := range []struct{ value, want string }{
		{"[]", "vmagent must be a nonempty list"},
		{"null", "vmagent must be a nonempty list"},
		{"{}", "vmagent must be a nonempty list"},
		{"[null]", "must be a mapping"},
		{"[{name: '', url: 'https://a.test'}]", "source name"},
		{"[{name: a}, {name: a}]", "HTTP(S)"}, // The first invalid source URL is reported first.
		{"[{name: a, url: 'https://a.test'}, {name: ' a ', url: 'https://b.test'}]", "duplicate target source name"},
		{"[{name: a, url: ''}]", "HTTP(S)"},
		{"[{name: a, url: 'ftp://a.test'}]", "HTTP(S)"},
		{"[{name: a, url: 'https://a.test?q=1'}]", "HTTP(S)"},
		{"[{name: a, url: 'https://a.test', insecure_skip_verify: true}]", "unknown target source field"},
		{"[{name: true, url: 'https://a.test'}]", "must be a string"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			data := "connections:\n  prod:\n    url: https://query.test\n    vmagent: " + tc.value + "\n"
			_, err := decode("config.yaml", []byte(data))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}
