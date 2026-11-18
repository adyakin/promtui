// Package editor provides syntax-aware presentation and completion on unfinished input.
package editor

import (
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/prometheus/prometheus/promql/parser"
)

// A tolerant tokenizer keeps unfinished strings/comments opaque. The official
// parser, not this tokenizer, is authoritative for syntax diagnostics.
var tokens = regexp.MustCompile("(?m)#[^\\n]*|\"(?:\\\\.|[^\"\\\\])*\"?|'(?:\\\\.|[^'\\\\])*'?|`[^`]*`?|[a-zA-Z_:][a-zA-Z_0-9:]*|[0-9]+(?:\\.[0-9]+)?[a-z]*|=~|!~|!=|==|>=|<=|[^\\s]")
var words = strings.Fields("and or unless by without on ignoring group_left group_right bool offset atan2 sum avg min max count group stddev stdvar topk bottomk count_values quantile limitk limit_ratio")

type Token struct {
	Start, End int
	Text       string
}

func scan(s string) []Token {
	var out []Token
	for _, p := range tokens.FindAllStringIndex(s, -1) {
		out = append(out, Token{p[0], p[1], s[p[0]:p[1]]})
	}
	return out
}
func quoted(s string) bool { return s != "" && strings.ContainsRune("\"'`", rune(s[0])) }
func closed(s string) bool {
	if len(s) < 2 || s[len(s)-1] != s[0] {
		return false
	}
	if s[0] == '`' {
		return true
	}
	n := 0
	for i := len(s) - 2; i > 0 && s[i] == '\\'; i-- {
		n++
	}
	return n%2 == 0
}
func LiteralAt(s string, pos int) bool {
	for _, t := range scan(s) {
		if t.Start < pos && pos <= t.End {
			return t.Text[0] == '#' || quoted(t.Text) && (pos < t.End || !closed(t.Text))
		}
	}
	return false
}
func ident(s string) bool {
	return s != "" && (s[0] == '_' || s[0] == ':' || s[0] >= 'A' && s[0] <= 'Z' || s[0] >= 'a' && s[0] <= 'z')
}
func Diagnostic(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	_, err := parser.ParseExpr(s)
	if err != nil {
		return err.Error()
	}
	return ""
}

type Context struct {
	Kind, Metric, Label, Prefix string
	Start, End                  int // byte offsets in source
	Quote                       byte
	Regex                       bool
}

func At(s string, pos int) Context {
	c := Context{Kind: "expr", Start: pos, End: pos}
	var stack []string
	var last, prev string
	ts := scan(s)
	current := -1
	for i, t := range ts {
		if t.Start >= pos {
			break
		}
		v := t.Text
		if t.End >= pos {
			if v[0] == '#' {
				c.Kind = ""
				return c
			}
			if quoted(v) {
				if pos == t.End && closed(v) {
					c.Kind = ""
					return c
				}
				if len(stack) == 0 || stack[len(stack)-1] != "{" || !(last == "=" || last == "!=" || last == "=~" || last == "!~") {
					c.Kind = ""
					return c
				}
				c.Kind, c.Label, c.Quote = "value", prev, v[0]
				c.Regex = last == "=~" || last == "!~"
				c.Start, c.End = t.Start+1, t.End
				if closed(v) {
					c.End--
				}
				c.Prefix = s[c.Start:pos]
				return c
			}
			if ident(v) || v[0] >= '0' && v[0] <= '9' {
				current = i
				c.Start, c.End, c.Prefix = t.Start, t.End, s[t.Start:pos]
				break
			}
		}
		switch v {
		case "{", "[", "(":
			tag := v
			if v == "{" {
				c.Metric = ""
				if ident(last) {
					c.Metric = last
				}
			}
			if v == "(" && (last == "by" || last == "without" || last == "on" || last == "ignoring" || last == "group_left" || last == "group_right") {
				tag = "group"
			}
			stack = append(stack, tag)
		case "}", "]", ")":
			if v == "}" {
				c.Metric = ""
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
		prev, last = last, v
	}
	if len(stack) > 0 {
		switch stack[len(stack)-1] {
		case "[":
			c.Kind = "duration"
		case "{", "group":
			c.Kind = "label"
		}
	}
	if last == "offset" {
		c.Kind = "duration"
	}
	// Adjacent identifiers aren't valid PromQL. Treat them as search terms
	// while editing, stopping at operators, keywords and line boundaries.
	if c.Kind == "expr" && current >= 0 && ident(c.Prefix) && !slices.Contains(words, c.Prefix) {
		for i := current - 1; i >= 0; i-- {
			t := ts[i]
			gap := s[t.End:c.Start]
			if !ident(t.Text) || slices.Contains(words, t.Text) || gap == "" || strings.Trim(gap, " \t") != "" {
				break
			}
			c.Start = t.Start
		}
		c.Prefix = s[c.Start:pos]
	}
	return c
}
func (c Context) Local() []string {
	var values []string
	switch c.Kind {
	case "expr":
		values = append(values, words...)
		for name, fn := range parser.Functions {
			if !fn.Experimental {
				values = append(values, name)
			}
		}
	case "duration":
		values = strings.Fields("15s 30s 1m 5m 15m 30m 1h 6h 24h 7d")
	}
	return c.Filter(values)
}
func (c Context) Filter(values []string) []string {
	seen := map[string]bool{}
	var out []string
	terms := strings.Fields(c.Prefix)
	multi := c.Kind == "expr" && len(terms) > 1
	spans := map[string]int{}
	for _, v := range values {
		match := strings.HasPrefix(v, c.Prefix)
		if multi && strings.HasPrefix(v, terms[0]) {
			match = true
			end := len(terms[0])
			for _, term := range terms[1:] {
				d := strings.Index(v[end:], term)
				if d < 0 {
					match = false
					break
				}
				end += d + len(term)
			}
			spans[v] = end
		}
		if match && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if multi {
			if spans[a] != spans[b] {
				return spans[a] < spans[b]
			}
			if len(a) != len(b) {
				return len(a) < len(b)
			}
		}
		return a < b
	})
	return out
}
func (c Context) Insert(v string) string {
	if c.Kind != "value" {
		return v
	}
	if c.Regex {
		v = regexp.QuoteMeta(v)
	}
	q := strconv.Quote(v)
	text := q[1 : len(q)-1]
	if c.Quote == '\'' {
		return strings.ReplaceAll(text, "'", "\\'")
	}
	if c.Quote == '`' {
		return v
	}
	return text
}
