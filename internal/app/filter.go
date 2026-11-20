package app

import (
	"slices"
	"strings"
	"unicode"
)

type fuzzyFilter []rune

func newFuzzyFilter(text string) fuzzyFilter {
	return []rune(strings.ToLower(strings.TrimSpace(text)))
}

// Match an ordered subsequence within a single field, without ranking rows.
func (f fuzzyFilter) matches(fields ...string) bool {
	if len(f) == 0 {
		return true
	}
	for _, field := range fields {
		i := 0
		for _, r := range field {
			if unicode.ToLower(r) == f[i] {
				i++
				if i == len(f) {
					return true
				}
			}
		}
	}
	return false
}

func matchLabels[K, V ~string](f fuzzyFilter, labels map[K]V) bool {
	pair := slices.Contains(f, '=')
	for k, v := range labels {
		if f.matches(string(k), string(v)) ||
			pair && f.matches(string(k)+"="+string(v)) {
			return true
		}
	}
	return false
}
