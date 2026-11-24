package app

import (
	"maps"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"promtui/internal/api"
)

// matchAlertRules only selects an exact vmalert rule by source and IDs, or
// by the generator's instance ID when the single-alert API is unavailable.
// Prometheus has no rule IDs: prefer an active instance over name/expression.
func matchAlertRules(data *api.Rules, row alertRow, meta api.VMAlert, ref api.VMAlertRef) (exact, candidates []alertRuleMatch) {
	if data == nil {
		return nil, nil
	}
	var instances []alertRuleMatch
	for gi := range data.Groups {
		g := &data.Groups[gi]
		if ref.GroupID != "" && g.ID != ref.GroupID {
			continue
		}
		for ri := range g.Rules {
			r := &g.Rules[ri]
			if r.Type != api.AlertingRule {
				continue
			}
			match := alertRuleMatch{group: g, rule: r}
			if meta.RuleID != "" {
				if r.ID == meta.RuleID {
					exact = append(exact, match)
				}
				continue
			}
			if ref.AlertID == "" && (r.Name != row.Labels["alertname"] || r.Expression != meta.Expression) {
				continue
			}
			if r.Name == row.Labels["alertname"] {
				candidates = append(candidates, match)
			}
			for _, a := range r.Alerts {
				if ref.AlertID != "" && a.ID == ref.AlertID ||
					ref.AlertID == "" && compatibleAlertInstance(a.Labels, row.Labels) {
					match.activeAt = a.ActiveAt
					instances = append(instances, match)
					break
				}
			}
		}
	}
	if meta.RuleID != "" {
		return exact, exact
	}
	if len(instances) != 0 {
		return instances, instances
	}
	return nil, candidates
}

// Alertmanager may add external labels; all labels reported for the rule
// instance must still agree with the alert.
func compatibleAlertInstance(instance, alert map[string]string) bool {
	if len(instance) == 0 {
		return false
	}
	for k, v := range instance {
		if got, ok := alert[k]; !ok || got != v {
			return false
		}
	}
	return true
}

func (m *Model) openAlertRule(msg alertNavigationDone) tea.Cmd {
	s := &m.rules
	if msg.rules != nil {
		s.sources[msg.source].data, s.sources[msg.source].err = msg.rules, nil
		s.indexSource(msg.source)
	}
	s.search.Blur()
	s.search.Reset()
	s.ruleType, s.state, s.details, s.filter = 0, 0, false, ""
	s.overlay, s.overlayTitle = "", ""
	now := time.Now()
	if msg.rules != nil && len(s.sources) == 1 {
		s.lastDone = now
	}
	m.alerts.closeAlertDetails()
	cmd := m.switchTab(rulesTab)
	if len(msg.exact) == 1 {
		match := msg.exact[0]
		root := s.roots[msg.source]
		for _, group := range root.children {
			if group.group != match.group {
				continue
			}
			for _, node := range group.children {
				if node.rule == match.rule {
					s.expanded[root.id], s.expanded[group.id] = true, true
					s.selected, s.detailTop = node.id, 0
					s.rebuild()
					s.ensureVisible(m.explorerLayout().listRows - 1)
					s.notice = "Generating alert rule · " + root.title
					return cmd
				}
			}
		}
	}
	s.filter = msg.row.Labels["alertname"]
	s.search.SetValue(s.filter)
	s.rebuild()
	s.ensureVisible(m.explorerLayout().listRows - 1)
	s.notice = "Exact generating rule unavailable · Showing rule search"
	if len(msg.candidates) > 1 {
		s.notice = "Ambiguous generating rule · Showing rule search"
	}
	return cmd
}

func alertScopeLabels(row alertRow, vmLabels map[string]string, matches []alertRuleMatch) map[string]string {
	labels := maps.Clone(row.Labels)
	for key, value := range labels {
		vmValue, fromVM := vmLabels[key]
		if key == "alertname" || key == "alertgroup" || key == "severity" || strings.HasPrefix(key, "__") ||
			len(vmLabels) != 0 && (!fromVM || vmValue != value) {
			delete(labels, key)
		}
	}
	// Remove labels supplied by the rule or group, even if a metric happens
	// to have a label with the same value.
	for _, match := range matches {
		for key := range match.group.Labels {
			delete(labels, key)
		}
		for key := range match.rule.Labels {
			delete(labels, key)
		}
	}
	return labels
}
