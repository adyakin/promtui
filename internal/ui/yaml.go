package ui

import (
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
)

type YAMLStyles struct {
	Key, String, Number, Boolean, Null, Comment, List, Quoted lipgloss.Style
}

var yamlNumber = regexp.MustCompile(`(?i)^[+-]?(?:0[xob][0-9a-f_]+|[0-9][0-9_]*(?:\.[0-9_]*)?(?:e[+-]?[0-9_]+)?|\.[0-9_]+(?:e[+-]?[0-9_]+)?|\.(?:inf|nan))$`)

// HighlightYAML highlights display text, not a parsed/re-serialized document.
// It accepts incomplete input and leaves spacing, indentation and newlines intact.
func HighlightYAML(s string, styles YAMLStyles) string {
	render := func(style lipgloss.Style, text string) string {
		return style.TabWidth(lipgloss.NoTabConversion).Render(text)
	}
	var out strings.Builder
	flow := 0
	var quote byte
	block := -1
	for n, line := range strings.Split(s, "\n") {
		if n > 0 {
			out.WriteByte('\n')
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if block >= 0 {
			if indent > block || strings.TrimSpace(line) == "" {
				out.WriteString(render(styles.String, line))
				continue
			}
			block = -1
		}
		space := func(i int) bool { return i >= len(line) || line[i] == ' ' || line[i] == '\t' }
		colon := func(i int) bool {
			return line[i] == ':' && (space(i+1) || flow > 0 && strings.ContainsRune(",[]{}", rune(line[i+1])))
		}
		for i := 0; i < len(line); {
			start := i
			if quote != 0 || line[i] == '"' || line[i] == '\'' {
				if quote == 0 {
					quote = line[i]
					i++
				}
				for i < len(line) {
					c := line[i]
					i++
					if i < len(line) && (quote == '"' && c == '\\' || quote == '\'' && c == '\'' && line[i] == '\'') {
						i++
					} else if c == quote {
						quote = 0
						break
					}
				}
				style := styles.Quoted
				separator := -1
				if j := i + len(line[i:]) - len(strings.TrimLeft(line[i:], " \t")); j < len(line) && line[j] == ':' {
					style = styles.Key
					separator = j
				}
				out.WriteString(render(style, line[start:i]))
				if separator >= 0 {
					out.WriteString(line[i : separator+1])
					i = separator + 1
				}
			} else if space(i) {
				out.WriteByte(line[i])
				i++
			} else if line[i] == '#' && (i == 0 || space(i-1)) {
				out.WriteString(render(styles.Comment, line[i:]))
				break
			} else if line[i] == '-' && space(i+1) {
				out.WriteString(render(styles.List, "-"))
				i++
			} else if line[i] == '[' || line[i] == '{' {
				flow++
				out.WriteByte(line[i])
				i++
			} else if flow > 0 && strings.ContainsRune("]},", rune(line[i])) || colon(i) {
				if line[i] == ']' || line[i] == '}' {
					flow--
				}
				out.WriteByte(line[i])
				i++
			} else {
				i++
				for i < len(line) && !colon(i) && !(line[i] == '#' && space(i-1)) &&
					!(flow > 0 && strings.ContainsRune(",]}", rune(line[i]))) {
					i++
				}
				text := strings.TrimRight(line[start:i], " \t")
				style := styles.String
				switch {
				case i < len(line) && colon(i):
					style = styles.Key
				case strings.EqualFold(text, "true"), strings.EqualFold(text, "false"):
					style = styles.Boolean
				case strings.EqualFold(text, "null"), text == "~":
					style = styles.Null
				case yamlNumber.MatchString(text):
					style = styles.Number
				case flow == 0 && (strings.HasPrefix(text, "|") || strings.HasPrefix(text, ">")) &&
					strings.Trim(text[1:], "123456789+-") == "":
					block = indent
				}
				out.WriteString(render(style, text))
				out.WriteString(line[start+len(text) : i])
			}
		}
	}
	return out.String()
}
