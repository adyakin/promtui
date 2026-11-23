package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestHighlightYAML(t *testing.T) {
	s := YAMLStyles{
		Key: lipgloss.NewStyle().Bold(true), String: lipgloss.NewStyle().Italic(true),
		Number: lipgloss.NewStyle().Underline(true), Boolean: lipgloss.NewStyle().Reverse(true),
		Null: lipgloss.NewStyle().Faint(true), Comment: lipgloss.NewStyle().Strikethrough(true),
		List: lipgloss.NewStyle().Foreground(lipgloss.Color("5")), Quoted: lipgloss.NewStyle().Foreground(lipgloss.Color("6")),
	}
	const input = "# config\n- name: some string \n  number: 1.5e-2\n  on: true\n  unset: null\n" +
		"  url: https://example.com:9090/path#frag # details\n  \"key: #\": \"a:\\\" b # still quoted\"\n" +
		"  quote: 'it''s # still quoted: yes'\n  flow: [42, false, {nested: ~}]\n" +
		"  block: |\n    literal: # not a comment\t \n  next: ok\n"
	out := HighlightYAML(input, s)
	if got := ansi.Strip(out); got != input {
		t.Fatalf("highlighting changed YAML text:\n%q\n%q", got, input)
	}
	for _, expected := range []string{
		s.Key.Render("name"), s.List.Render("-"), s.String.Render("some string"),
		s.Number.Render("1.5e-2"), s.Boolean.Render("true"), s.Null.Render("null"),
		s.String.Render("https://example.com:9090/path#frag"), s.Comment.Render("# details"),
		s.Key.Render(`"key: #"`), s.Quoted.Render(`"a:\" b # still quoted"`),
		s.Quoted.Render("'it''s # still quoted: yes'"), s.Boolean.Render("false"),
		s.Key.Render("nested"), s.Null.Render("~"), s.Comment.Render("# config"),
		s.String.TabWidth(lipgloss.NoTabConversion).Render("    literal: # not a comment\t "),
	} {
		if !strings.Contains(out, expected) {
			t.Errorf("missing span %q:\n%q", expected, out)
		}
	}
}
