package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

func decodeYAML(path string, data []byte, s settings, views *alertViewRegistry) (settings, bool, []rawConnection, error) {
	raw := struct {
		settings           `yaml:",inline"`
		InsecureSkipVerify yaml.Node       `yaml:"insecure_skip_verify"`
		Connections        yamlConnections `yaml:"connections"`
		Alerts             yaml.Node       `yaml:"alerts"`
	}{settings: s}
	if err := decodeYAMLDocument(data, &raw); err != nil {
		return s, false, nil, err
	}
	if err := views.loadMain(path, raw.Alerts); err != nil {
		return s, false, nil, err
	}
	global, err := yamlTLS(raw.InsecureSkipVerify)
	return raw.settings, global != nil && *global, raw.Connections, err
}

func decodeYAMLDocument(data []byte, dst any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("expected a single YAML document")
		}
		return err
	}
	return nil
}

type yamlConnections []rawConnection

// Read the mapping in file order so the first connection remains the fallback.
func (c *yamlConnections) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: connections must be a mapping of names to settings", node.Line)
	}
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Kind != yaml.ScalarNode || key.ShortTag() != "!!str" || value.Kind != yaml.MappingNode {
			return fmt.Errorf("line %d: expected a connection name and mapping", key.Line)
		}
		var fields struct {
			URL                string    `yaml:"url"`
			AlertmanagerURL    string    `yaml:"alertmanager_url"`
			InsecureSkipVerify yaml.Node `yaml:"insecure_skip_verify"`
			VMAgent            yaml.Node `yaml:"vmagent"`
			LegacyTargets      yaml.Node `yaml:"targets"`
			VMAlert            yaml.Node `yaml:"vmalert"`
			Alerts             yaml.Node `yaml:"alerts"`
			TargetsView        yaml.Node `yaml:"targets_view"`
		}
		for j := 0; j < len(value.Content); j += 2 {
			field := value.Content[j]
			switch field.Value {
			case "url", "alertmanager_url":
				if value.Content[j+1].ShortTag() != "!!str" {
					return fmt.Errorf("connection %q: line %d: %s must be a string", key.Value, field.Line, field.Value)
				}
			case "insecure_skip_verify", "vmagent", "targets", "vmalert", "alerts", "targets_view":
			default:
				return fmt.Errorf("connection %q: line %d: unknown field %q", key.Value, field.Line, field.Value)
			}
		}
		if err := value.Decode(&fields); err != nil {
			return fmt.Errorf("connection %q: %w", key.Value, err)
		}
		override, err := yamlTLS(fields.InsecureSkipVerify)
		if err != nil {
			return fmt.Errorf("connection %q: %w", key.Value, err)
		}
		sourceNode, sourceKey := fields.VMAgent, "vmagent"
		if fields.LegacyTargets.Kind != 0 {
			if sourceNode.Kind != 0 {
				return fmt.Errorf("connection %q: use vmagent instead of targets; do not set both", key.Value)
			}
			sourceNode, sourceKey = fields.LegacyTargets, "targets"
		}
		targets, err := yamlSources(sourceNode, sourceKey, "target")
		if err != nil {
			return fmt.Errorf("connection %q: %w", key.Value, err)
		}
		rules, err := yamlSources(fields.VMAlert, "vmalert", "vmalert")
		if err != nil {
			return fmt.Errorf("connection %q: %w", key.Value, err)
		}
		var defaultView string
		alerts, err := yamlTableView(fields.Alerts, AlertsView, "alerts", &defaultView)
		if err != nil {
			return fmt.Errorf("connection %q: %w", key.Value, err)
		}
		targetsView, err := yamlTableView(fields.TargetsView, TargetsView, "targets_view", nil)
		if err != nil {
			return fmt.Errorf("connection %q: %w", key.Value, err)
		}
		*c = append(*c, rawConnection{
			Name: key.Value, URL: fields.URL, AlertmanagerURL: fields.AlertmanagerURL, InsecureSkipVerify: override, Targets: targets,
			Alerts: alerts, AlertDefaultView: defaultView, TargetsView: targetsView, VMAlert: rules,
		})
	}
	return nil
}

func yamlSources(node yaml.Node, keyName, role string) ([]EndpointSource, error) {
	if node.Kind == 0 {
		return nil, nil
	}
	if node.Kind == yaml.AliasNode {
		node = *node.Alias
	}
	if node.Kind != yaml.SequenceNode || len(node.Content) == 0 {
		return nil, fmt.Errorf("line %d: %s must be a nonempty list of named sources", node.Line, keyName)
	}
	for _, source := range node.Content {
		if source.Kind == yaml.AliasNode {
			source = source.Alias
		}
		if source.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("line %d: %s source must be a mapping", source.Line, role)
		}
		for i := 0; i < len(source.Content); i += 2 {
			key, value := source.Content[i], source.Content[i+1]
			if key.Value != "name" && key.Value != "url" {
				return nil, fmt.Errorf("line %d: unknown %s source field %q", key.Line, role, key.Value)
			}
			if value.Kind == yaml.AliasNode {
				value = value.Alias
			}
			if value.ShortTag() != "!!str" {
				return nil, fmt.Errorf("line %d: %s source %s must be a string", value.Line, role, key.Value)
			}
		}
	}
	var sources []TargetSource
	err := node.Decode(&sources)
	return sources, err
}

func yamlTLS(node yaml.Node) (*bool, error) {
	if node.Kind == 0 {
		return nil, nil
	}
	if node.Kind == yaml.AliasNode {
		node = *node.Alias
	}
	if node.Kind != yaml.ScalarNode || node.ShortTag() != "!!bool" {
		return nil, fmt.Errorf("line %d: insecure_skip_verify must be a boolean (true or false)", node.Line)
	}
	var value bool
	if err := node.Decode(&value); err != nil {
		return nil, err
	}
	return &value, nil
}
