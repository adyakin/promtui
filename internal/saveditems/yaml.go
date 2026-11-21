package saveditems

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// StringNode keeps multiline text exact despite YAML block indentation and
// line-break normalization.
func StringNode(s string) yaml.Node {
	n := yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
	if strings.ContainsAny(s, "\n\r\u0085\u2028\u2029") {
		n.Style = yaml.DoubleQuotedStyle
	}
	return n
}
