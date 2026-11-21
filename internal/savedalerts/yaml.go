package savedalerts

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type collection struct {
	Alerts []Filter `yaml:"alerts"`
}

func (c *collection) UnmarshalYAML(node *yaml.Node) error {
	if err := checkFields(node, "saved alerts", map[string]yaml.Kind{"alerts": yaml.SequenceNode}, "alerts"); err != nil {
		return err
	}
	type plain collection
	return node.Decode((*plain)(c))
}

func (f *Filter) UnmarshalYAML(node *yaml.Node) error {
	if err := checkFields(node, "filter", map[string]yaml.Kind{
		"name": yaml.ScalarNode, "matchers": yaml.SequenceNode, "parameters": yaml.SequenceNode,
	}, "name", "matchers"); err != nil {
		return err
	}
	type plain Filter
	return node.Decode((*plain)(f))
}

func (m *Matcher) UnmarshalYAML(node *yaml.Node) error {
	if err := checkFields(node, "matcher", map[string]yaml.Kind{
		"label": yaml.ScalarNode, "operator": yaml.ScalarNode, "value": yaml.ScalarNode,
	}, "label", "operator", "value"); err != nil {
		return err
	}
	type plain Matcher
	return node.Decode((*plain)(m))
}

// Check types before decoding: yaml.v3 can coerce scalars to strings and
// silently discard null entries in sequences of structs.
func checkFields(node *yaml.Node, kind string, fields map[string]yaml.Kind, required ...string) error {
	if node.Kind != yaml.MappingNode || node.ShortTag() != "!!map" {
		return fmt.Errorf("line %d: expected a %s mapping", node.Line, kind)
	}
	seen := make(map[string]bool, len(fields))
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		want, ok := fields[key.Value]
		if !ok || key.Kind != yaml.ScalarNode || key.ShortTag() != "!!str" {
			return fmt.Errorf("line %d: unexpected %s field %q", key.Line, kind, key.Value)
		}
		if seen[key.Value] {
			return fmt.Errorf("line %d: duplicate %s field %q", key.Line, kind, key.Value)
		}
		seen[key.Value] = true
		switch want {
		case yaml.ScalarNode:
			if value.Kind != want || value.ShortTag() != "!!str" {
				return fmt.Errorf("line %d: %s must be a string", value.Line, key.Value)
			}
		case yaml.SequenceNode:
			if value.Kind != want || value.ShortTag() != "!!seq" {
				return fmt.Errorf("line %d: %s must be a list", value.Line, key.Value)
			}
			for _, entry := range value.Content {
				if entry.Kind != yaml.MappingNode || entry.ShortTag() != "!!map" {
					return fmt.Errorf("line %d: expected a mapping in %s", entry.Line, key.Value)
				}
			}
		}
	}
	for _, key := range required {
		if !seen[key] {
			return fmt.Errorf("line %d: missing %s field %q", node.Line, kind, key)
		}
	}
	return nil
}

func (f Filter) MarshalYAML() (any, error) {
	type parameter struct {
		Name    string     `yaml:"name"`
		Default *yaml.Node `yaml:"default,omitempty"`
	}
	items := make([]parameter, len(f.Parameters))
	for i, p := range f.Parameters {
		items[i].Name = p.Name
		if p.Default != nil {
			node := textNode(*p.Default)
			items[i].Default = &node
		}
	}
	return struct {
		Name       yaml.Node   `yaml:"name"`
		Matchers   []Matcher   `yaml:"matchers"`
		Parameters []parameter `yaml:"parameters,omitempty"`
	}{textNode(f.Name), f.Matchers, items}, nil
}

func (m Matcher) MarshalYAML() (any, error) {
	return struct {
		Label    yaml.Node `yaml:"label"`
		Operator yaml.Node `yaml:"operator"`
		Value    yaml.Node `yaml:"value"`
	}{textNode(m.Label), textNode(m.Operator), textNode(m.Value)}, nil
}

func textNode(s string) yaml.Node {
	node := yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
	// Implicitly indented blocks and YAML line-break normalization can
	// otherwise change the original bytes when the file is reloaded.
	if strings.ContainsAny(s, "\n\r\u0085\u2028\u2029") {
		node.Style = yaml.DoubleQuotedStyle
	}
	return node
}
