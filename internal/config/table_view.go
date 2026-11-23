package config

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type TableView struct {
	// A nil list uses that view's defaults, independently of the other list.
	Columns []Column
	Search  []string
}

type Column struct {
	Field   string
	Title   string
	Width   int
	Weight  int
	Extract *regexp.Regexp
}

type ViewKind int

const (
	AlertsView ViewKind = iota
	TargetsView
)

var fieldTitles = [...]map[string]string{
	AlertsView: {
		"state": "State", "severity": "Severity", "alertname": "Alert", "instance": "Instance", "duration": "Duration",
	},
	TargetsView: {
		"health": "Health", "job": "Job", "instance": "Instance", "source": "Source",
		"last_scrape": "Last scrape", "duration": "Duration", "scrape_url": "Scrape URL", "scrape_pool": "Scrape pool",
		"global_url": "Global URL", "last_error": "Last error", "scrape_interval": "Scrape interval", "scrape_timeout": "Scrape timeout",
	},
}

func (v ViewKind) FieldTitle(field string) string {
	if label, ok := LabelName(field); ok {
		return strings.ToUpper(label)
	}
	if v == AlertsView {
		if annotation, ok := strings.CutPrefix(field, "annotations."); ok {
			return strings.ToUpper(annotation)
		}
	}
	return fieldTitles[v][field]
}

func LabelName(field string) (string, bool) {
	return strings.CutPrefix(field, "labels.")
}

func (v ViewKind) validateField(field string) error {
	if strings.TrimSpace(field) == "" {
		return fmt.Errorf("field must not be empty")
	}
	if name, ok := LabelName(field); ok {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("label reference %q must name a label after labels.", field)
		}
		return nil
	}
	if fieldTitles[v][field] == "" {
		return fmt.Errorf("unknown field %q (use labels.<name> for a Prometheus label)", field)
	}
	return nil
}

func yamlTableView(node yaml.Node, view ViewKind, path string, defaultView *string) (TableView, error) {
	var result TableView
	if node.Kind == 0 {
		return result, nil
	}
	n := unalias(&node)
	if n.Kind != yaml.MappingNode {
		return result, fmt.Errorf("%s: expected a mapping", path)
	}
	keys := map[string]bool{}
	for i := 0; i < len(n.Content); i += 2 {
		key, list := n.Content[i].Value, unalias(n.Content[i+1])
		if key != "columns" && key != "search" && !(key == "default_view" && defaultView != nil) {
			return result, fmt.Errorf("%s: unknown property %q", path, key)
		}
		listPath := path + "." + key
		if keys[key] {
			return result, fmt.Errorf("%s: duplicate property", listPath)
		}
		keys[key] = true
		if key == "default_view" {
			name, err := yamlViewString(list, key)
			if err != nil {
				return result, fmt.Errorf("%s: %w", path, err)
			}
			if strings.TrimSpace(name) == "" {
				return result, fmt.Errorf("%s must not be empty", listPath)
			}
			*defaultView = name
			continue
		}
		if list.Kind != yaml.SequenceNode || len(list.Content) == 0 {
			return result, fmt.Errorf("%s: expected a nonempty list", listPath)
		}
		seen := map[string]bool{}
		for j, item := range list.Content {
			item = unalias(item)
			var col Column
			var err error
			if key == "columns" {
				col, err = yamlColumn(item, "field")
			} else {
				col.Field, err = yamlViewString(item, "field")
			}
			if err == nil {
				err = view.validateField(col.Field)
			}
			if err != nil {
				return result, fmt.Errorf("%s[%d]: %w", listPath, j, err)
			}
			if key == "search" {
				result.Search = append(result.Search, col.Field)
				continue
			}
			if seen[col.Field] {
				return result, fmt.Errorf("%s[%d]: duplicate column %q", listPath, j, col.Field)
			}
			seen[col.Field] = true
			result.Columns = append(result.Columns, col)
		}
	}
	return result, nil
}

func yamlColumn(n *yaml.Node, sourceKey string) (Column, error) {
	if n.Kind != yaml.MappingNode {
		if sourceKey == "source" {
			return Column{}, fmt.Errorf("expected a mapping with a source")
		}
		field, err := yamlViewString(n, "field")
		return Column{Field: field}, err
	}
	var col Column
	seen := map[string]bool{}
	for i := 0; i < len(n.Content); i += 2 {
		key := n.Content[i].Value
		if seen[key] {
			return col, fmt.Errorf("duplicate column property %q", key)
		}
		seen[key] = true
		valueNode := unalias(n.Content[i+1])
		switch {
		case key == sourceKey:
			value, err := yamlViewString(valueNode, key)
			if err != nil {
				return col, err
			}
			col.Field = value
		case key == "title":
			value, err := yamlViewString(valueNode, key)
			if err != nil {
				return col, err
			}
			if strings.TrimSpace(value) == "" {
				return col, fmt.Errorf("title must not be empty")
			}
			col.Title = value
		case sourceKey == "source" && (key == "width" || key == "weight"):
			var value int
			if valueNode.Kind != yaml.ScalarNode || valueNode.ShortTag() != "!!int" || valueNode.Decode(&value) != nil || value <= 0 {
				return col, fmt.Errorf("%s must be a positive integer", key)
			}
			if key == "width" {
				col.Width = value
			} else {
				col.Weight = value
			}
		case sourceKey == "source" && key == "extract":
			value, err := yamlViewString(valueNode, key)
			if err != nil {
				return col, err
			}
			col.Extract, err = regexp.Compile(value)
			if err != nil {
				return col, fmt.Errorf("invalid extract regex: %w", err)
			}
			if col.Extract.NumSubexp() != 1 {
				return col, fmt.Errorf("extract regex must contain exactly one capture group")
			}
		default:
			return col, fmt.Errorf("unknown column property %q", key)
		}
	}
	if !seen[sourceKey] {
		return col, fmt.Errorf("missing %s", sourceKey)
	}
	if seen["width"] && seen["weight"] {
		return col, fmt.Errorf("width and weight are mutually exclusive")
	}
	return col, nil
}

func yamlViewString(n *yaml.Node, property string) (string, error) {
	if n.Kind != yaml.ScalarNode || n.ShortTag() != "!!str" {
		return "", fmt.Errorf("%s must be a string", property)
	}
	return n.Value, nil
}

func unalias(n *yaml.Node) *yaml.Node {
	if n.Kind == yaml.AliasNode {
		return n.Alias
	}
	return n
}
