package alertmanager

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/alertmanager/pkg/labels"
	"gopkg.in/yaml.v3"
)

// IsInhibited describes runtime state, not the outcome of local rule matching.
// "suppressed" by itself does not say whether an alert was silenced or inhibited.
func (a Alert) IsInhibited() bool {
	return len(a.Status.InhibitedBy) > 0 || a.Status.State == "inhibited"
}

type InhibitRule struct {
	Source, Target labels.Matchers
	Equal          []string
}

func (c Client) InhibitRules(ctx context.Context) ([]InhibitRule, error) {
	original, err := c.originalConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch inhibition rules: %w", err)
	}
	return parseInhibitRules(original)
}

func parseInhibitRules(original string) ([]InhibitRule, error) {
	type ruleYAML struct {
		SourceMatchers []string          `yaml:"source_matchers"`
		TargetMatchers []string          `yaml:"target_matchers"`
		SourceMatch    map[string]string `yaml:"source_match"`
		TargetMatch    map[string]string `yaml:"target_match"`
		SourceRE       map[string]string `yaml:"source_match_re"`
		TargetRE       map[string]string `yaml:"target_match_re"`
		Equal          []string
	}
	var config *struct {
		Rules []*ruleYAML `yaml:"inhibit_rules"`
	}
	if err := yaml.Unmarshal([]byte(original), &config); err != nil || config == nil {
		// Avoid showing parser errors, which can quote unrelated config values.
		return nil, errors.New("could not parse Alertmanager configuration for inhibition rules")
	}
	rules := make([]InhibitRule, 0, len(config.Rules))
	for i, raw := range config.Rules {
		if raw == nil {
			return nil, fmt.Errorf("inhibit rule %d is empty", i+1)
		}
		source, err := configMatchers(raw.SourceMatchers, raw.SourceMatch, raw.SourceRE)
		if err != nil {
			return nil, fmt.Errorf("invalid source matcher in inhibit rule %d", i+1)
		}
		target, err := configMatchers(raw.TargetMatchers, raw.TargetMatch, raw.TargetRE)
		if err != nil {
			return nil, fmt.Errorf("invalid target matcher in inhibit rule %d", i+1)
		}
		rules = append(rules, InhibitRule{source, target, raw.Equal})
	}
	return rules, nil
}

// Inhibition indexes one snapshot. Positions are indices into the original alerts
// slice. Local matches share source buckets rather than storing every possible pair.
type Inhibition struct {
	alerts     []Alert
	byID       map[string]int
	targets    map[int][]int
	candidates map[int][]candidateGroup
	Rules      []InhibitRule
}

type candidateGroup struct {
	rule    int
	sources []int
}

type Inhibitor struct {
	Source      int // -1 when the runtime reference is unavailable
	Fingerprint string
	Confirmed   bool
	Rules       []int // matching rule indices, not confirmed runtime rule IDs
}

func IndexInhibition(alerts []Alert, rules []InhibitRule, now time.Time) *Inhibition {
	x := &Inhibition{
		alerts: alerts, Rules: rules, byID: make(map[string]int, len(alerts)),
		targets: make(map[int][]int), candidates: make(map[int][]candidateGroup),
	}
	for i, a := range alerts {
		if a.Fingerprint != "" {
			x.byID[a.Fingerprint] = i
		}
	}
	for i, a := range alerts {
		seen := make(map[int]bool)
		for _, id := range a.Status.InhibitedBy {
			if source := x.source(i, id); source >= 0 && !seen[source] {
				x.targets[source] = append(x.targets[source], i)
				seen[source] = true
			}
		}
	}
	for r, rule := range rules {
		type bucket struct{ all, sourceOnly []int }
		buckets := make(map[string]*bucket)
		type target struct {
			index int
			key   string
			both  bool
		}
		var targets []target
		for i, a := range alerts {
			isSource := matchAlert(rule.Source, a.Labels)
			isTarget := matchAlert(rule.Target, a.Labels)
			if !isSource && !isTarget {
				continue
			}
			key := equalKey(rule.Equal, a.Labels)
			if isTarget {
				targets = append(targets, target{i, key, isSource})
			}
			if isSource && (a.EndsAt.IsZero() || a.EndsAt.After(now)) {
				b := buckets[key]
				if b == nil {
					b = &bucket{}
					buckets[key] = b
				}
				b.all = append(b.all, i)
				if !isTarget {
					b.sourceOnly = append(b.sourceOnly, i)
				}
			}
		}
		for _, t := range targets {
			if b := buckets[t.key]; b != nil {
				sources := b.all
				// For a two-sided target, Alertmanager excludes all
				// two-sided sources, including the target itself.
				if t.both {
					sources = b.sourceOnly
				}
				if len(sources) > 0 {
					x.candidates[t.index] = append(x.candidates[t.index], candidateGroup{r, sources})
				}
			}
		}
	}
	return x
}

func matchAlert(ms labels.Matchers, set map[string]string) bool {
	for _, m := range ms {
		if !m.Matches(set[m.Name]) {
			return false
		}
	}
	return true
}

func equalKey(equal []string, set map[string]string) string {
	var key strings.Builder
	for _, label := range equal {
		value := set[label] // Alertmanager treats missing and empty as equal.
		key.WriteString(strconv.Itoa(len(value)))
		key.WriteByte(':')
		key.WriteString(value)
	}
	return key.String()
}

func (x *Inhibition) source(target int, id string) int {
	if i, ok := x.byID[id]; ok && i != target {
		return i
	}
	return -1
}

// Sources combines authoritative runtime references with separately marked
// local matches. A matching rule is never proof that Alertmanager used it.
func (x *Inhibition) Sources(target int) []Inhibitor {
	var result []Inhibitor
	positions := make(map[int]int)
	seen := make(map[string]bool)
	for _, id := range x.alerts[target].Status.InhibitedBy {
		if seen[id] {
			continue
		}
		seen[id] = true
		source := x.source(target, id)
		if source >= 0 {
			positions[source] = len(result)
		}
		result = append(result, Inhibitor{Source: source, Fingerprint: id, Confirmed: true})
	}
	for _, group := range x.candidates[target] {
		for _, source := range group.sources {
			id := x.alerts[source].Fingerprint
			if source == target || id != "" && id == x.alerts[target].Fingerprint {
				continue
			}
			pos, ok := positions[source]
			if !ok {
				pos = len(result)
				positions[source] = pos
				result = append(result, Inhibitor{Source: source, Fingerprint: id})
			}
			result[pos].Rules = append(result[pos].Rules, group.rule)
		}
	}
	return result
}

// Targets returns only targets that refer to source in the runtime snapshot.
func (x *Inhibition) Targets(source int) []int { return x.targets[source] }
