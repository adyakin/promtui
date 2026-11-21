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
