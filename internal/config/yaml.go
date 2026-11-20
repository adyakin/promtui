package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

func decodeYAML(data []byte, s settings) (settings, bool, []rawConnection, error) {
	raw := struct {
		settings           `yaml:",inline"`
		InsecureSkipVerify yaml.Node       `yaml:"insecure_skip_verify"`
		Connections        yamlConnections `yaml:"connections"`
	}{settings: s}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&raw); err != nil && !errors.Is(err, io.EOF) {
		return s, false, nil, err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("expected a single YAML document")
		}
		return s, false, nil, err
	}
	global, err := yamlTLS(raw.InsecureSkipVerify)
	return raw.settings, global != nil && *global, raw.Connections, err
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
		}
		for j := 0; j < len(value.Content); j += 2 {
			field := value.Content[j]
			switch field.Value {
			case "url", "alertmanager_url":
				if value.Content[j+1].ShortTag() != "!!str" {
					return fmt.Errorf("connection %q: line %d: %s must be a string", key.Value, field.Line, field.Value)
				}
			case "insecure_skip_verify":
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
		*c = append(*c, rawConnection{
			Name: key.Value, URL: fields.URL, AlertmanagerURL: fields.AlertmanagerURL, InsecureSkipVerify: override,
		})
	}
	return nil
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
