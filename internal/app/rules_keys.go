package app

import (
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"
	"promtui/internal/api"
)

func (m *Model) cancelRulesRefresh(now time.Time) tea.Cmd {
	s := &m.rules
	for i := range s.sources {
		if s.sources[i].loading {
			s.sources[i].err = errors.New("Rules request cancelled")
		}
	}
	s.stop()
	s.lastDone = now
	s.rebuild()
	return m.scheduleRulesTick(now)
}

func (m *Model) rulesKey(msg tea.KeyPressMsg) tea.Cmd {
	s := &m.rules
	key := msg.String()
	if s.search.Focused() {
		switch key {
		case "enter":
			s.filter = s.search.Value()
			s.search.Blur()
			s.rebuild()
		case "esc":
			m.cancelRulesSearch()
		default:
			return m.editRulesSearch(msg)
		}
		return nil
	}
	n := s.selection()
	layout := m.explorerLayout()
	defer s.ensureVisible(layout.listRows - 1)
	switch key {
	case "q":
		return m.act("quit", "", 0)
	case "r":
		return m.refreshRules()
	case "/":
		s.searchAnchor, s.searchTop, s.searchDetailTop = s.selected, s.listTop, s.detailTop
		s.search.SetValue(s.filter)
		s.search.CursorEnd()
		return s.search.Focus()
	case "esc":
		if len(s.fuzzy()) != 0 {
			// Reveal the current match in the unsearched tree before clearing.
			for p := n; p != nil; p = p.parent {
				s.expanded[p.id] = true
			}
			s.filter = ""
			s.rebuild()
		}
	case "f":
		s.ruleType = (s.ruleType + 1) % len(ruleTypeFilters)
		if ruleTypeFilters[s.ruleType] == api.RecordingRule {
			s.state = 0
		}
		s.rebuild()
	case "s":
		s.state = (s.state + 1) % len(ruleStateFilters)
		if s.state != 0 && ruleTypeFilters[s.ruleType] == api.RecordingRule {
			s.ruleType = 0
		}
		s.rebuild()
	case "tab", "shift+tab":
		s.details = !s.details
	case "p":
		if n != nil && n.rule != nil {
			cmd, err := m.loadPromQLExpression(n.rule.Expression)
			if err != nil {
				s.notice = err.Error()
				return nil
			}
			s.stop()
			return cmd
		}
	case "e":
		if text := n.errorText(); text != "" {
			s.overlayTitle, s.overlay, s.overlayTop = "Rule error", text, 0
		}
	case "?":
		s.overlayTitle, s.overlay, s.overlayTop = "Rules help", rulesHelp, 0
	case "h", "left", "l", "right", "enter":
		if n == nil {
			break
		}
		if s.details && key == "enter" && n.errorText() != "" {
			s.overlayTitle, s.overlay, s.overlayTop = "Rule error", n.errorText(), 0
		} else if !s.details {
			s.treeKey(n, key)
		}
	default:
		key = routingNavKey(key)
		if s.details {
			s.detailTop = scrollPaneError(key, s.detailTop, len(rulesDetails(n, layout.detailWidth)), layout.detailRows)
		} else if n != nil {
			index := targetNavigate(key, s.selectedRow(), len(s.visible), layout.listRows-1)
			s.selectNode(s.visible[index])
		}
	}
	return nil
}

func (s *rulesState) treeKey(n *ruleNode, key string) {
	searching := len(s.fuzzy()) != 0
	if key == "left" || key == "h" {
		if len(n.children) > 0 && s.isOpen(n) && !searching {
			s.expanded[n.id] = false
			s.rebuild()
		} else if p := n.parent; p != nil && (p.parent != nil || s.showRoot(p)) {
			s.selectNode(p)
		}
		return
	}
	if len(n.children) == 0 {
		if key == "enter" {
			s.details = true
		}
		return
	}
	if !searching && (!s.isOpen(n) || key == "enter") {
		s.expanded[n.id] = !s.isOpen(n)
		s.rebuild()
		return
	}
	if i := s.selectedRow() + 1; i < len(s.visible) && s.visible[i].parent == n {
		s.selectNode(s.visible[i])
	}
}

func (m *Model) editRulesSearch(msg tea.Msg) tea.Cmd {
	s := &m.rules
	before := s.search.Value()
	var cmd tea.Cmd
	s.search, cmd = s.search.Update(msg)
	if s.search.Value() != before {
		s.rebuild()
		s.ensureVisible(m.explorerLayout().listRows - 1)
	}
	return cmd
}

func (m *Model) cancelRulesSearch() {
	s := &m.rules
	if s.search.Focused() {
		s.search.Blur()
		s.selected, s.listTop, s.detailTop = s.searchAnchor, s.searchTop, s.searchDetailTop
		s.rebuild()
	}
}

func (m *Model) rulesModalKey(key string) tea.Cmd {
	s := &m.rules
	switch key {
	case "ctrl+q":
		return m.act("quit", "", 0)
	case "esc":
		s.overlayTitle = ""
	case "?":
		if s.overlayTitle == "Rules help" {
			s.overlayTitle = ""
		}
	default:
		rows, _, room := m.rulesOverlayContent()
		s.overlayTop = scrollPaneError(routingNavKey(key), s.overlayTop, len(rows), room)
	}
	return nil
}
