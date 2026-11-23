package app

import (
	"strings"
	"unicode"
)

type searchToken struct {
	field, value        string
	start, valueAt, end int // Rune offsets, as used by textinput's cursor.
}

func scanSearch(query []rune, visit func(searchToken) bool) {
	for i := 0; i < len(query); {
		if unicode.IsSpace(query[i]) {
			i++
			continue
		}
		token := searchToken{start: i, valueAt: i}
		// Only an initial field-like name qualifies a term. Leave literal
		// URLs, numeric addresses, and colons inside quoted text alone.
		j := i
		for j < len(query) && (unicode.IsLetter(query[j]) || query[j] == '_' ||
			j > i && (unicode.IsDigit(query[j]) || query[j] == '.' || query[j] == '-')) {
			j++
		}
		if j > i && j < len(query) && query[j] == ':' &&
			!(j+2 < len(query) && query[j+1] == '/' && query[j+2] == '/') {
			token.field = string(query[i:j])
			i, token.valueAt = j+1, j+1
		}
		var value strings.Builder
		quoted := false
		for i < len(query) && (quoted || !unicode.IsSpace(query[i])) {
			r := query[i]
			switch {
			case r == '"':
				quoted = !quoted
			case quoted && r == '\\' && i+1 < len(query) && (query[i+1] == '"' || query[i+1] == '\\'):
				i++
				value.WriteRune(query[i])
			default:
				value.WriteRune(r)
			}
			i++
		}
		token.value, token.end = value.String(), i
		if !visit(token) {
			return
		}
	}
}

func activeSearchToken(query []rune, pos int) (searchToken, bool) {
	var token searchToken
	var found bool
	scanSearch(query, func(t searchToken) bool {
		if pos >= t.valueAt && pos <= t.end {
			token, found = t, true
		}
		return !found && pos >= t.start
	})
	return token, found
}

func quoteSearchValue(value string) string {
	if strings.ContainsAny(value, `":`) || strings.ContainsFunc(value, unicode.IsSpace) {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
	}
	return value
}

type searchTerm struct {
	field string
	value fuzzyFilter
}

// parsedSearch caches the last expression across refreshes and sort changes.
type parsedSearch struct {
	query string
	terms []searchTerm
}

func (s *parsedSearch) update(query string) {
	if s.query == query {
		return
	}
	s.query = query
	clear(s.terms)
	s.terms = s.terms[:0]
	scanSearch([]rune(query), func(t searchToken) bool {
		if value := newFuzzyFilter(t.value); len(value) != 0 {
			s.terms = append(s.terms, searchTerm{t.field, value})
		}
		return true
	})
}

func (s *parsedSearch) matches(all func(fuzzyFilter) bool, field func(string) string) bool {
	for _, term := range s.terms {
		if term.field == "" {
			if !all(term.value) {
				return false
			}
		} else if !term.value.matches(field(term.field)) {
			return false
		}
	}
	return true
}
