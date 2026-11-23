package app

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"promtui/internal/alertmanager"
)

type alertInhibitionSnapshot struct {
	rows  []alertRow
	index *alertmanager.Inhibition
	err   error
}

type alertRelation struct {
	index int
	kind  string
}

type alertRelations struct {
	lines []string
	links []alertRelation
}

type alertDetailVisit struct {
	row                alertRow
	relations          alertRelations
	offset, relatedRow int
	status             string
}

func (a *alertsState) openAlertDetails(row alertRow) {
	a.dialog, a.detail, a.offset, a.detailStatus = alertDetails, row, 0, ""
	a.relatedRow = 0
	a.detailRelations = a.detailInhibition.relations(row)
	a.detailWrap = alertDetailWrap{}
}

func (a *alertsState) closeAlertDetails() {
	a.dialog = alertNoDialog
	a.detailTrail = nil
	a.detailInhibition = alertInhibitionSnapshot{}
	a.detailRelations = alertRelations{}
	a.detailWrap = alertDetailWrap{}
}

func (a *alertsState) followRelatedAlert() {
	link := a.detailRelations.links[a.relatedRow]
	a.detailTrail = append(a.detailTrail, alertDetailVisit{a.detail, a.detailRelations, a.offset, a.relatedRow, a.detailStatus})
	a.openAlertDetails(a.detailInhibition.rows[link.index])
}

func (a *alertsState) backToAlert() bool {
	if len(a.detailTrail) == 0 {
		return false
	}
	i := len(a.detailTrail) - 1
	v := a.detailTrail[i]
	a.detailTrail[i] = alertDetailVisit{}
	a.detailTrail = a.detailTrail[:i]
	a.detail, a.detailRelations, a.offset, a.relatedRow, a.detailStatus = v.row, v.relations, v.offset, v.relatedRow, v.status
	a.detailWrap = alertDetailWrap{}
	return true
}

func (s alertInhibitionSnapshot) relations(row alertRow) alertRelations {
	var out alertRelations
	if s.index == nil {
		return out
	}
	sources, targets := s.index.Sources(row.snapshotIndex), s.index.Targets(row.snapshotIndex)
	if row.IsInhibited() || len(sources) > 0 || s.err != nil {
		out.lines = append(out.lines, "", "Inhibition")
		if row.IsInhibited() {
			out.lines = append(out.lines, "State: inhibited (reported by Alertmanager)")
			if len(row.Status.InhibitedBy) == 0 {
				out.lines = append(out.lines, "No runtime source reference reported.")
			} else {
				out.lines = append(out.lines, "Alertmanager may report only one of several matching sources.")
			}
		} else if len(sources) > 0 {
			out.lines = append(out.lines, "No runtime inhibition reported; sources below are local matches only.")
		}
		if s.err != nil {
			out.lines = append(out.lines, "Rule explanations unavailable: "+s.err.Error())
		}
		for _, source := range sources {
			if source.Source < 0 {
				reason := "not uniquely resolvable in this alert snapshot"
				switch {
				case source.Fingerprint == "":
					reason = "empty fingerprint"
				case source.Fingerprint == row.Fingerprint:
					reason = "self-reference"
				}
				out.lines = append(out.lines, "", "Unresolved runtime source: "+alertValue(source.Fingerprint), "  Reason: "+reason+".")
				continue
			}
			kind := "Potential source"
			switch {
			case source.Confirmed:
				kind = "Confirmed source"
			case row.IsInhibited():
				kind = "Matching source (not runtime-confirmed)"
			}
			out.links = append(out.links, alertRelation{source.Source, kind})
			src := s.rows[source.Source]
			out.lines = appendRelatedLabels(out.lines, kind, src)
			out.lines = s.appendMatchingRules(out.lines, source.Rules, src, row)
		}
	}
	if len(targets) > 0 {
		out.lines = append(out.lines, "", fmt.Sprintf("Inhibits (%d confirmed)", len(targets)))
		for _, targetIndex := range targets {
			target := s.rows[targetIndex]
			out.links = append(out.links, alertRelation{targetIndex, "Confirmed target"})
			out.lines = appendRelatedLabels(out.lines, "Confirmed target", target)
			out.lines = s.appendMatchingRules(out.lines, s.index.MatchingRules(row.snapshotIndex, targetIndex), row, target)
		}
	}
	return out
}

func (s alertInhibitionSnapshot) appendMatchingRules(lines []string, rules []int, source, target alertRow) []string {
	if len(rules) == 0 && s.err == nil {
		return append(lines, "  No configured rule matches this pair in the local snapshot.")
	}
	for _, i := range rules {
		rule := s.index.Rules[i]
		lines = append(lines, fmt.Sprintf("  Matching rule %d (local evaluation):", i+1))
		lines = appendRuleValues(lines, "source_matchers", matcherStrings(rule.Source))
		lines = appendRuleValues(lines, "target_matchers", matcherStrings(rule.Target))
		var equal []string
		missing := false
		for _, key := range rule.Equal {
			src, srcOK := equalLabelValue(source.Labels, key)
			dst, dstOK := equalLabelValue(target.Labels, key)
			if !srcOK || !dstOK {
				missing = true
				equal = append(equal, fmt.Sprintf("%s: source %s = target %s", strconv.QuoteToGraphic(key), src, dst))
			} else {
				equal = append(equal, strconv.QuoteToGraphic(key)+" = "+src)
			}
		}
		lines = appendRuleValues(lines, "equal", equal)
		if missing {
			lines = append(lines, "      A missing label counts as empty.")
		}
	}
	return lines
}

func equalLabelValue(set map[string]string, key string) (string, bool) {
	if value, ok := set[key]; ok {
		return strconv.QuoteToGraphic(value), true
	}
	return "<missing>", false
}

func appendRelatedLabels(lines []string, kind string, row alertRow) []string {
	lines = append(lines, "", kind+": "+alertValue(row.Labels["alertname"]))
	for _, key := range slices.Sorted(maps.Keys(row.Labels)) {
		lines = append(lines, "  "+strconv.QuoteToGraphic(key)+" = "+strconv.QuoteToGraphic(row.Labels[key]))
	}
	if row.Fingerprint != "" {
		lines = append(lines, "  Fingerprint: "+row.Fingerprint)
	}
	return lines
}

func matcherStrings[T interface{ String() string }](matchers []T) []string {
	values := make([]string, 0, len(matchers))
	for _, matcher := range matchers {
		values = append(values, matcher.String())
	}
	return values
}

func appendRuleValues(lines []string, key string, values []string) []string {
	if len(values) == 0 {
		return append(lines, "    "+key+": []")
	}
	lines = append(lines, "    "+key+":")
	for _, value := range values {
		lines = append(lines, "      - "+value)
	}
	return lines
}

func (a *alertsState) relatedAlertName(link alertRelation) string {
	row := a.detailInhibition.rows[link.index]
	name := []string{alertValue(row.Labels["alertname"])}
	for _, key := range []string{"severity", "servicename", "instance"} {
		if value := row.Labels[key]; value != "" {
			name = append(name, value)
		}
	}
	if row.Fingerprint != "" {
		name = append(name, row.Fingerprint)
	}
	kind := link.kind
	if kind == "Matching source (not runtime-confirmed)" || kind == "Potential source" {
		kind = "Local match"
	}
	return kind + ": " + strings.Join(name, " · ")
}

func (m *Model) moveRelatedAlert(key string) {
	if m.width < minWidth || m.height < minHeight {
		return
	}
	a := &m.alerts
	step := 1
	if key == "pgup" || key == "pgdown" {
		step = max(1, m.alertRelatedRoom())
	}
	if key == "up" || key == "pgup" {
		step = -step
	}
	a.relatedRow = max(0, min(a.relatedRow+step, len(a.detailRelations.links)-1))
}

func (m *Model) alertRelatedRoom() int {
	return min(len(m.alerts.detailRelations.links), max(0, m.height-max(7, m.height/3)))
}
