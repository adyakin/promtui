// Package parameters validates saved templates and substitutes raw string values.
package parameters

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Parameter struct {
	Name    string  `yaml:"name"`
	Default *string `yaml:"default,omitempty"`
}

type Values map[string]string

func (p Parameter) MarshalYAML() (any, error) {
	var value *yaml.Node
	if p.Default != nil {
		value = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: *p.Default, Style: yaml.DoubleQuotedStyle}
	}
	return struct {
		Name    string     `yaml:"name"`
		Default *yaml.Node `yaml:"default,omitempty"`
	}{p.Name, value}, nil
}

func (p *Parameter) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: expected a parameter mapping", node.Line)
	}
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		switch key.Value {
		case "name", "default":
			if value.Kind != yaml.ScalarNode || value.ShortTag() != "!!str" {
				return fmt.Errorf("line %d: parameter %s must be a string", value.Line, key.Value)
			}
		default:
			return fmt.Errorf("line %d: unexpected parameter field %q", key.Line, key.Value)
		}
	}
	type plain Parameter
	return node.Decode((*plain)(p))
}

var namePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// scan recognizes ${name} and $${ (a literal ${). Other dollars are literal.
// The writer is called on the input only, never on substituted values.
func scan(text string, literal func(string), ref func(string) error) error {
	for {
		i := strings.Index(text, "${")
		if i < 0 {
			literal(text)
			return nil
		}
		if i > 0 && text[i-1] == '$' {
			literal(text[:i-1])
			literal("${")
			text = text[i+2:]
			continue
		}
		literal(text[:i])
		text = text[i+2:]
		end := strings.IndexByte(text, '}')
		if end < 0 {
			return fmt.Errorf("unclosed parameter reference after ${")
		}
		name := text[:end]
		if !namePattern.MatchString(name) {
			return fmt.Errorf("invalid parameter reference ${%s}", name)
		}
		if err := ref(name); err != nil {
			return err
		}
		text = text[end+1:]
	}
}

// References returns each reference once, in order of first occurrence.
func References(text string) ([]string, error) {
	var refs []string
	seen := map[string]bool{}
	err := scan(text, func(string) {}, func(name string) error {
		if !seen[name] {
			refs = append(refs, name)
			seen[name] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return refs, nil
}

// Validate checks declarations against all pieces of a template (e.g. matcher values).
func Validate(definitions []Parameter, texts ...string) error {
	used := make(map[string]bool, len(definitions))
	for _, p := range definitions {
		if !namePattern.MatchString(p.Name) {
			return fmt.Errorf("invalid parameter name %q", p.Name)
		}
		if _, ok := used[p.Name]; ok {
			return fmt.Errorf("duplicate parameter %q", p.Name)
		}
		used[p.Name] = false
	}
	for _, text := range texts {
		refs, err := References(text)
		if err != nil {
			return err
		}
		for _, name := range refs {
			if _, ok := used[name]; !ok {
				return fmt.Errorf("undefined parameter %q", name)
			}
			used[name] = true
		}
	}
	for _, p := range definitions {
		if !used[p.Name] {
			return fmt.Errorf("unused parameter %q", p.Name)
		}
	}
	return nil
}

// WithDefaults builds a new map and returns required values still missing.
func WithDefaults(definitions []Parameter, supplied Values) (Values, []string) {
	values := make(Values, len(definitions))
	var missing []string
	for _, p := range definitions {
		if value, ok := supplied[p.Name]; ok {
			values[p.Name] = value
		} else if p.Default != nil {
			values[p.Name] = *p.Default
		} else {
			missing = append(missing, p.Name)
		}
	}
	return values, missing
}

// Resolve substitutes supplied values, once and without escaping or rewriting.
func Resolve(text string, values Values) (string, error) {
	var out strings.Builder
	err := scan(text, func(s string) { out.WriteString(s) }, func(name string) error {
		value, ok := values[name]
		if !ok {
			return fmt.Errorf("missing value for parameter %q", name)
		}
		out.WriteString(value)
		return nil
	})
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

// EscapeLiteral makes text safe to persist as a template without changing its resolved value.
func EscapeLiteral(text string) string {
	return strings.ReplaceAll(text, "${", "$${")
}
