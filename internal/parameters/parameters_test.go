package parameters

import (
	"maps"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestResolve(t *testing.T) {
	for _, tc := range []struct {
		template string
		want     string
	}{
		{"up", "up"},
		{"${a}", "a.b"},
		{".*${a}.*${b}/${a}", ".*a.b.*雪/a.b"},
		{"${empty}", ""},
		{"$ $0 $$ $${other} ${nested}", "$ $0 $$ ${other} ${other}"},
	} {
		got, err := Resolve(tc.template, Values{"a": "a.b", "b": "雪", "empty": "", "nested": "${other}"})
		if err != nil || got != tc.want {
			t.Errorf("Resolve(%q) = %q, %v; want %q", tc.template, got, err, tc.want)
		}
	}
	if _, err := Resolve("${missing}", nil); err == nil {
		t.Fatal("missing value accepted")
	}
	for _, literal := range []string{"${name}", "$${", "$${${inner}", "$${name}${}"} {
		if got, err := Resolve(EscapeLiteral(literal), nil); err != nil || got != literal {
			t.Errorf("literal round trip %q = %q, %v", literal, got, err)
		}
	}
}

func TestDefinitionsAndReferences(t *testing.T) {
	defs := []Parameter{{Name: "a"}, {Name: "_b2"}}
	if err := Validate(defs, "${a}", "x${_b2}/${a}"); err != nil {
		t.Fatal(err)
	}
	if refs, err := References("${a}$${notARef}${_b2}${a}"); err != nil || !reflect.DeepEqual(refs, []string{"a", "_b2"}) {
		t.Fatalf("references = %v, %v", refs, err)
	}
	for _, text := range []string{"${a", "${}", "${a-b}", "${${a}}", "${a b}", "${1}", "${unknown}"} {
		if err := Validate(defs, text); err == nil {
			t.Errorf("accepted %q", text)
		}
	}
	for _, bad := range [][]Parameter{
		{{Name: "a"}, {Name: "a"}}, {{Name: "bad.name"}}, {{Name: ""}}, {{Name: "unused"}},
	} {
		if err := Validate(bad, "${a}"); err == nil {
			t.Errorf("accepted %v", bad)
		}
	}
}

func TestDefaults(t *testing.T) {
	var defs []Parameter
	if err := yaml.Unmarshal([]byte("- name: suggestion\n  default: abc\n- name: empty\n  default: ''\n- name: required\n- name: supplied\n"), &defs); err != nil {
		t.Fatal(err)
	}
	values, missing := WithDefaults(defs, Values{"suggestion": "override", "supplied": ""})
	if !maps.Equal(values, Values{"suggestion": "override", "empty": "", "supplied": ""}) ||
		!reflect.DeepEqual(missing, []string{"required"}) {
		t.Fatalf("values = %#v, missing = %v", values, missing)
	}
	for _, raw := range []string{"name: a\nname: b", "name: a\ndefault: null", "name: a\ndefault: false", "name: a\nextra: yes"} {
		if err := yaml.Unmarshal([]byte(raw), new(Parameter)); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}

func TestDefaultYAMLRoundTrip(t *testing.T) {
	for _, value := range []string{"", "true", "  a\nb", "a\r\nb", "\u0085\u2028\u2029", "\t雪 ${other}"} {
		raw, err := yaml.Marshal(Parameter{Name: "a", Default: &value})
		if err != nil {
			t.Fatal(err)
		}
		var p Parameter
		if err := yaml.Unmarshal(raw, &p); err != nil || p.Default == nil || *p.Default != value {
			t.Fatalf("default %q did not round trip: %q -> %+v (%v)", value, raw, p, err)
		}
	}
}

func FuzzEscapeLiteral(f *testing.F) {
	for _, seed := range []string{"", "${parameter}", "$${x} $$$", "${unclosed", "${outer${inner}雪", "\x00\r\n"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		template := EscapeLiteral(text)
		if err := Validate(nil, template); err != nil {
			t.Fatalf("escaped %q is invalid: %v", template, err)
		}
		if got, err := Resolve(template, nil); err != nil || got != text {
			t.Fatalf("literal %q became %q, %v", text, got, err)
		}
	})
}
