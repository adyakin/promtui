// Package savedqueries stores the global, ordered collection of named PromQL expressions.
package savedqueries

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
	"promtui/internal/parameters"
	"promtui/internal/saveditems"
)

// A query without parameters is a literal expression. A parameterized query
// stores its expression as a template plus declarations for ${name}
// references. The parameters field stays absent (or empty) for plain items so
// legacy files are unchanged.
type Query struct {
	Name       string                 `yaml:"name"`
	Expression string                 `yaml:"expression"`
	Parameters []parameters.Parameter `yaml:"parameters,omitempty"`
}

func (q Query) MarshalYAML() (any, error) {
	// yaml.v3 uses an implicitly indented block for multiline strings.
	// Quote leading indentation so the first line cannot set the block's
	// indent ambiguously or strip whitespace from the expression.
	text := func(s string) yaml.Node {
		node := yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
		first := strings.TrimLeft(s, "\n")
		if strings.ContainsAny(s, "\r\u0085\u2028\u2029") ||
			strings.Contains(s, "\n") && (strings.HasPrefix(first, " ") || strings.HasPrefix(first, "\t")) {
			node.Style = yaml.DoubleQuotedStyle
		}
		return node
	}
	params := make([]parameters.Parameter, len(q.Parameters))
	copy(params, q.Parameters)
	return struct {
		Name       yaml.Node              `yaml:"name"`
		Expression yaml.Node              `yaml:"expression"`
		Parameters []parameters.Parameter `yaml:"parameters,omitempty"`
	}{text(q.Name), text(q.Expression), params}, nil
}

// Do not let yaml.v3 coerce booleans or numbers to strings and then rewrite them.
func (q *Query) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return errors.New("expected a query with a name and expression")
	}
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		switch key.Value {
		case "name", "expression", "parameters":
			if value.Kind != yaml.ScalarNode || value.ShortTag() != "!!str" {
				if key.Value == "parameters" {
					continue // The parameters node is decoded as a list below.
				}
				return fmt.Errorf("line %d: %s must be a string", value.Line, key.Value)
			}
		default:
			return fmt.Errorf("line %d: unexpected query field %q", key.Line, key.Value)
		}
	}
	type plain Query
	return node.Decode((*plain)(q))
}

func queriesEqual(a, b []Query) bool {
	return slices.EqualFunc(a, b, func(x, y Query) bool {
		if x.Name != y.Name || x.Expression != y.Expression || len(x.Parameters) != len(y.Parameters) {
			return false
		}
		for i := range x.Parameters {
			p, q := x.Parameters[i], y.Parameters[i]
			if p.Name != q.Name || (p.Default == nil) != (q.Default == nil) || (p.Default != nil && *p.Default != *q.Default) {
				return false
			}
		}
		return true
	})
}

// cloneParameters copies declarations so saved state never aliases caller input.
func cloneParameters(defs []parameters.Parameter) []parameters.Parameter {
	if len(defs) == 0 {
		return nil
	}
	out := make([]parameters.Parameter, len(defs))
	for i, p := range defs {
		out[i] = p
		if p.Default != nil {
			def := *p.Default
			out[i].Default = &def
		}
	}
	return out
}

type Snapshot struct {
	path    string
	version []byte // nil only when the file did not exist
	queries []Query
}

type collection struct {
	Queries []Query `yaml:"queries"`
}

var ErrChanged = saveditems.ErrChanged

func Open(path string) (Snapshot, error) {
	s := Snapshot{path: path}
	if path == "" {
		return s, errors.New("saved queries path is not configured")
	}
	raw, err := saveditems.Read(path)
	if err != nil {
		return s, fmt.Errorf("read saved queries: %w", err)
	}
	if raw == nil {
		return s, nil
	}
	var data collection
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&data); err != nil {
		return s, fmt.Errorf("invalid queries.yaml: %w", err)
	}
	if err := dec.Decode(new(yaml.Node)); err != io.EOF {
		if err == nil {
			err = errors.New("expected only one YAML document")
		}
		return s, fmt.Errorf("invalid queries.yaml: %w", err)
	}
	if data.Queries == nil {
		return s, errors.New("invalid queries.yaml: expected a queries list")
	}
	for i, q := range data.Queries {
		name, err := checkedName(data.Queries[:i], q.Name, -1)
		if err == nil {
			err = CheckExpression(q.Expression)
		}
		if err == nil && len(q.Parameters) != 0 {
			err = parameters.Validate(q.Parameters, q.Expression)
		}
		if err != nil {
			return s, fmt.Errorf("invalid queries.yaml entry %d: %w", i+1, err)
		}
		data.Queries[i].Name, data.Queries[i].Parameters = name, cloneParameters(q.Parameters)
	}
	s.version, s.queries = raw, data.Queries
	return s, nil
}

func (s Snapshot) Queries() []Query { return slices.Clone(s.queries) }

func CheckExpression(expression string) error {
	if strings.TrimSpace(expression) == "" {
		return errors.New("enter a PromQL expression first")
	}
	return nil
}

func checkedName(queries []Query, name string, except int) (string, error) {
	return saveditems.CheckName(name, len(queries), except, func(i int) string { return queries[i].Name }, "query")
}

func (s Snapshot) Add(name, expression string, definitions []parameters.Parameter) (Snapshot, error) {
	name, err := checkedName(s.queries, name, -1)
	if err != nil {
		return s, err
	}
	if err := checkTemplate(definitions, expression); err != nil {
		return s, err
	}
	next := append(s.Queries(), Query{Name: name, Expression: expression, Parameters: cloneParameters(definitions)})
	return s.commit(next)
}

// checkTemplate applies template rules only when parameters were declared;
// plain items keep their legacy literal-dollar behavior.
func checkTemplate(definitions []parameters.Parameter, expression string) error {
	if err := CheckExpression(expression); err != nil {
		return err
	}
	if len(definitions) == 0 {
		return nil
	}
	return parameters.Validate(definitions, expression)
}

func (s Snapshot) Rename(index int, name string) (Snapshot, error) {
	if index < 0 || index >= len(s.queries) {
		return s, errors.New("select a saved query")
	}
	name, err := checkedName(s.queries, name, index)
	if err != nil {
		return s, err
	}
	next := s.Queries()
	next[index].Name = name
	return s.commit(next)
}

func (s Snapshot) Update(index int, expression string, definitions []parameters.Parameter) (Snapshot, error) {
	if index < 0 || index >= len(s.queries) {
		return s, errors.New("select a saved query")
	}
	if err := checkTemplate(definitions, expression); err != nil {
		return s, err
	}
	next := s.Queries()
	next[index].Expression = expression
	next[index].Parameters = cloneParameters(definitions)
	return s.commit(next)
}

func (s Snapshot) Delete(index int) (Snapshot, error) {
	if index < 0 || index >= len(s.queries) {
		return s, errors.New("select a saved query")
	}
	return s.commit(slices.Delete(s.Queries(), index, index+1))
}

func (s Snapshot) commit(next []Query) (Snapshot, error) {
	fail := func(err error) (Snapshot, error) {
		return s, fmt.Errorf("write saved queries %s: %w", s.path, err)
	}
	if queriesEqual(s.queries, next) {
		if err := saveditems.CheckVersion(s.path, s.version); err != nil {
			return fail(err)
		}
		return s, nil
	}
	raw, err := yaml.Marshal(collection{next})
	if err != nil {
		return fail(err)
	}
	if err := saveditems.Write(s.path, s.version, raw); err != nil {
		return fail(err)
	}
	s.version, s.queries = raw, next
	return s, nil
}
