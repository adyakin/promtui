// Package savedalerts stores named Alertmanager label matcher templates.
package savedalerts

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"promtui/internal/alertmanager"
	"promtui/internal/parameters"
)

type Filter struct {
	Name       string                 `yaml:"name"`
	Matchers   []Matcher              `yaml:"matchers"`
	Parameters []parameters.Parameter `yaml:"parameters,omitempty"`
}

type Matcher struct {
	Label    string `yaml:"label"`
	Operator string `yaml:"operator"`
	Value    string `yaml:"value"`
}

// Compile evaluates literal (not templated) matchers. An empty set matches all
// alerts, allowing the manual matcher editor to clear the structured filter.
func Compile(matchers []Matcher) (func(map[string]string) bool, error) {
	if len(matchers) == 0 {
		return func(map[string]string) bool { return true }, nil
	}
	compiled := make([]alertmanager.Matcher, len(matchers))
	for i, matcher := range matchers {
		m := alertmanager.Matcher{Name: matcher.Label, Value: matcher.Value}
		switch matcher.Operator {
		case "=":
			m.IsEqual = true
		case "!=":
		case "=~":
			m.IsRegex, m.IsEqual = true, true
		case "!~":
			m.IsRegex = true
		default:
			return nil, fmt.Errorf("matcher %d: unsupported operator %q", i+1, matcher.Operator)
		}
		compiled[i] = m
	}
	match, _, err := alertmanager.CompileMatchers(compiled)
	return match, err
}

// Resolve applies defaults and substitutes raw values without modifying the template.
func (f Filter) Resolve(supplied parameters.Values) ([]Matcher, func(map[string]string) bool, error) {
	if err := f.validateTemplate(); err != nil {
		return nil, nil, err
	}
	values, missing := parameters.WithDefaults(f.Parameters, supplied)
	if len(missing) != 0 {
		return nil, nil, fmt.Errorf("missing values for parameters: %s", strings.Join(missing, ", "))
	}
	matchers := slices.Clone(f.Matchers)
	for i, matcher := range matchers {
		value, err := parameters.Resolve(matcher.Value, values)
		if err != nil {
			return nil, nil, fmt.Errorf("matcher %d: %w", i+1, err)
		}
		matchers[i].Value = value
	}
	match, err := Compile(matchers)
	if err != nil {
		return nil, nil, err
	}
	return matchers, match, nil
}

func (f Filter) validateTemplate() error {
	if len(f.Matchers) == 0 {
		return errors.New("add at least one matcher")
	}
	texts := make([]string, len(f.Matchers))
	for i, m := range f.Matchers {
		texts[i] = m.Value
	}
	return parameters.Validate(f.Parameters, texts...)
}

func (f Filter) validateStored() error {
	if err := f.validateTemplate(); err != nil {
		return err
	}
	matchers := slices.Clone(f.Matchers)
	for i, matcher := range matchers {
		refs, _ := parameters.References(matcher.Value) // already validated
		if len(refs) == 0 {
			matchers[i].Value, _ = parameters.Resolve(matcher.Value, nil)
		} else {
			// Check the label and operator now, but leave the regex itself
			// for Resolve, since supplied values can change its validity.
			matchers[i].Value = ""
		}
	}
	_, err := Compile(matchers)
	return err
}

func (f Filter) clone() Filter {
	f.Matchers = slices.Clone(f.Matchers)
	f.Parameters = parameters.CloneDefinitions(f.Parameters)
	return f
}
