package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type alertViewRegistry struct {
	views   map[string]TableView
	origins map[string]string
}

func (r *alertViewRegistry) loadMain(path string, node yaml.Node) error {
	if node.Kind == 0 {
		return nil
	}
	n := unalias(&node)
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("alerts: expected a mapping")
	}
	seenViews := false
	for i := 0; i < len(n.Content); i += 2 {
		key := n.Content[i].Value
		if key != "views" {
			return fmt.Errorf("alerts: unknown property %q", key)
		}
		if seenViews {
			return fmt.Errorf("alerts.views: duplicate property")
		}
		seenViews = true
		if err := r.add(path, "alerts.views", n.Content[i+1]); err != nil {
			return err
		}
	}
	return nil
}

func (r *alertViewRegistry) loadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read alert views directory: %w", err)
	}
	// ReadDir sorts entries by file name.
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if err := r.loadFile(path); err != nil {
			return fmt.Errorf("alert views file %s: %w", path, err)
		}
	}
	return nil
}

func (r *alertViewRegistry) loadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var file struct {
		Views yaml.Node `yaml:"views"`
	}
	if err := decodeYAMLDocument(data, &file); err != nil {
		return err
	}
	return r.add(path, "views", &file.Views)
}

func (r *alertViewRegistry) add(file, path string, node *yaml.Node) error {
	n := unalias(node)
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("%s: expected a mapping of view names to definitions", path)
	}
	for i := 0; i < len(n.Content); i += 2 {
		key := n.Content[i]
		name, err := yamlViewString(key, "view name")
		if err != nil {
			return fmt.Errorf("%s line %d: %w", path, key.Line, err)
		}
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("%s line %d: view name must not be empty", path, key.Line)
		}
		origin := fmt.Sprintf("%s:%d (%s)", file, key.Line, path)
		if previous, ok := r.origins[name]; ok {
			return fmt.Errorf("duplicate alert view %q: defined in %s and %s", name, previous, origin)
		}
		view, err := yamlNamedAlertView(unalias(n.Content[i+1]))
		if err != nil {
			return fmt.Errorf("%s[%q]: %w", path, name, err)
		}
		if r.views == nil {
			r.views = make(map[string]TableView)
			r.origins = make(map[string]string)
		}
		r.views[name], r.origins[name] = view, origin
	}
	return nil
}

func yamlNamedAlertView(node *yaml.Node) (TableView, error) {
	var view TableView
	if node.Kind != yaml.MappingNode {
		return view, fmt.Errorf("expected a mapping")
	}
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if key != "columns" {
			return view, fmt.Errorf("unknown property %q", key)
		}
		if view.Columns != nil {
			return view, fmt.Errorf("columns: duplicate property")
		}
		list := unalias(node.Content[i+1])
		if list.Kind != yaml.SequenceNode || len(list.Content) == 0 {
			return view, fmt.Errorf("columns: expected a nonempty list")
		}
		for j, item := range list.Content {
			col, err := yamlColumn(unalias(item), "source")
			if err == nil {
				err = validateAlertSource(col.Field)
			}
			if err != nil {
				return view, fmt.Errorf("columns[%d]: %w", j, err)
			}
			view.Columns = append(view.Columns, col)
		}
	}
	if view.Columns == nil {
		return view, fmt.Errorf("missing columns")
	}
	return view, nil
}

func validateAlertSource(source string) error {
	if strings.TrimSpace(source) == "" {
		return fmt.Errorf("source must not be empty")
	}
	for _, prefix := range []string{"labels.", "annotations."} {
		if name, ok := strings.CutPrefix(source, prefix); ok {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("source %q must specify a name after %s", source, prefix)
			}
			return nil
		}
	}
	if fieldTitles[AlertsView][source] == "" {
		return fmt.Errorf("unknown source %q (use a built-in field, labels.<name>, or annotations.<name>)", source)
	}
	return nil
}
