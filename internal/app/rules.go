package app

import (
	"context"
	"fmt"
	"slices"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"promtui/internal/api"
	"promtui/internal/config"
)

const rulesRefreshInterval = 30 * time.Second

type rulesLoaded struct {
	id     uint64
	source int
	data   api.Rules
	err    error
}

type rulesTick time.Time

type ruleSourceState struct {
	config.RuleSource
	data    *api.Rules
	err     error
	loading bool
}

type ruleNode struct {
	id, title string
	parent    *ruleNode
	children  []*ruleNode
	source    *ruleSourceState
	group     *api.RuleGroup
	rule      *api.Rule
	alert     *api.RuleAlert
}

type rulesState struct {
	sources    []ruleSourceState
	cancel     context.CancelFunc
	requestID  uint64
	pending    int
	lastDone   time.Time
	tickQueued bool

	roots, visible                []*ruleNode
	expanded                      map[string]bool
	selected                      string
	listTop, detailTop            int
	details                       bool
	ruleType, state               int
	search                        textinput.Model
	filter                        string
	searchAnchor                  string
	searchTop, searchDetailTop    int
	overlay, overlayTitle, notice string
	overlayTop                    int
}

var ruleTypeFilters = [...]string{"", api.AlertingRule, api.RecordingRule}
var ruleStateFilters = [...]string{"", "firing", "pending", "inactive"}

func newRulesState(c config.Connection) rulesState {
	input := textinput.New()
	input.Prompt = " / "
	input.Placeholder = "Filter rules"
	input.CharLimit = 256
	input.SetVirtualCursor(false)
	s := rulesState{search: input, expanded: make(map[string]bool)}
	for _, source := range c.RuleSources() {
		s.sources = append(s.sources, ruleSourceState{RuleSource: source})
	}
	s.roots = make([]*ruleNode, len(s.sources))
	for i := range s.sources {
		s.indexSource(i)
	}
	s.rebuild()
	return s
}

func (s *rulesState) stop() {
	if s.cancel == nil {
		return
	}
	s.cancel()
	s.cancel = nil
	s.pending = 0
	s.requestID++
	for i := range s.sources {
		s.sources[i].loading = false
	}
}

func (m *Model) refreshRules() tea.Cmd {
	if m.quitting || m.activeTab != rulesTab {
		return nil
	}
	s := &m.rules
	s.stop()
	s.notice = ""
	timeout := m.opt.Timeout
	if timeout <= 0 {
		timeout = rulesRefreshInterval
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	s.requestID++
	id := s.requestID
	s.cancel, s.pending = cancel, len(s.sources)
	client := m.connectionHTTP()
	cmds := make([]tea.Cmd, 0, s.pending)
	for i := range s.sources {
		source := &s.sources[i]
		source.loading = true
		endpoint := api.Client{Base: source.URL, HTTP: client}
		cmds = append(cmds, func() tea.Msg {
			data, err := endpoint.Rules(ctx)
			return rulesLoaded{id, i, data, err}
		})
	}
	s.rebuild()
	return tea.Batch(cmds...)
}

func (m *Model) onRulesLoaded(msg rulesLoaded, now time.Time) tea.Cmd {
	s := &m.rules
	if msg.id != s.requestID || s.cancel == nil || msg.source < 0 || msg.source >= len(s.sources) || !s.sources[msg.source].loading {
		return nil
	}
	source := &s.sources[msg.source]
	source.loading, source.err = false, msg.err
	if msg.err == nil {
		source.data = &msg.data
		s.indexSource(msg.source)
	}
	s.rebuild()
	s.pending--
	if s.pending == 0 {
		s.cancel()
		s.cancel = nil
		s.lastDone = now
		return m.scheduleRulesTick(now)
	}
	return nil
}

func (m *Model) rulesOnEntry(now time.Time) tea.Cmd {
	if now.Sub(m.rules.lastDone) >= rulesRefreshInterval {
		return m.refreshRules()
	}
	return m.scheduleRulesTick(now)
}

func (m *Model) scheduleRulesTick(now time.Time) tea.Cmd {
	s := &m.rules
	if m.activeTab != rulesTab || m.quitting || s.tickQueued || s.cancel != nil {
		return nil
	}
	s.tickQueued = true
	return tea.Tick(max(0, s.lastDone.Add(rulesRefreshInterval).Sub(now)), func(t time.Time) tea.Msg {
		return rulesTick(t)
	})
}

func (m *Model) onRulesTick(now time.Time) tea.Cmd {
	s := &m.rules
	s.tickQueued = false
	if s.cancel != nil || m.activeTab != rulesTab {
		return nil
	}
	return m.rulesOnEntry(now)
}

func (s *rulesState) indexSource(i int) {
	src := &s.sources[i]
	root := &ruleNode{id: fmt.Sprintf("%q", src.Name), title: src.Name, source: src}
	s.roots[i] = root
	if src.data == nil {
		return
	}
	add := func(parent, n *ruleNode, key string, seen map[string]int) *ruleNode {
		n.parent = parent
		n.id = fmt.Sprintf("%s/%q:%d", parent.id, key, seen[key])
		seen[key]++
		parent.children = append(parent.children, n)
		return n
	}
	groups := make(map[string]int)
	for gi := range src.data.Groups {
		g := &src.data.Groups[gi]
		key := fmt.Sprintf("%q:%q", g.File, g.Name)
		if g.ID != "" {
			key = fmt.Sprintf("id:%q", g.ID)
		}
		group := add(root, &ruleNode{title: g.Name, source: src, group: g}, key, groups)
		rules := make(map[string]int)
		for ri := range g.Rules {
			r := &g.Rules[ri]
			key := fmt.Sprintf("pos:%d", r.Position)
			if r.ID != "" {
				key = "id:" + r.ID
			}
			rule := add(group, &ruleNode{title: r.Name, source: src, group: g, rule: r}, key, rules)
			instances := make(map[string]int)
			for ai := range r.Alerts {
				a := &r.Alerts[ai]
				key := a.ID
				if key == "" {
					key = alertLabelText(a.Labels)
				}
				title := a.Labels["instance"]
				if title == "" {
					title = a.Labels["pod"]
				}
				if title == "" {
					title = fmt.Sprintf("Instance %d", ai+1)
				}
				add(rule, &ruleNode{title: title, source: src, group: g, rule: r, alert: a}, key, instances)
			}
		}
	}
}

func (s *rulesState) isOpen(n *ruleNode) bool {
	if open, exists := s.expanded[n.id]; exists {
		return open
	}
	return n.rule == nil
}

func (s *rulesState) showRoot(n *ruleNode) bool {
	return len(s.roots) > 1 || len(n.children) == 0 || n.source.err != nil
}

func (s *rulesState) fuzzy() fuzzyFilter {
	if s.search.Focused() {
		return newFuzzyFilter(s.search.Value())
	}
	return newFuzzyFilter(s.filter)
}

func (s *rulesState) rebuild() {
	f := s.fuzzy()
	clear(s.visible[:cap(s.visible)])
	s.visible = s.visible[:0]
	filterType, filterState := ruleTypeFilters[s.ruleType], ruleStateFilters[s.state]
	for _, root := range s.roots {
		rootAt := len(s.visible)
		showRoot := s.showRoot(root)
		rootMatches := f.matches(root.title)
		if showRoot {
			s.visible = append(s.visible, root)
		}
		for _, group := range root.children {
			groupAt := len(s.visible)
			s.visible = append(s.visible, group)
			groupMatches := rootMatches || f.matches(group.title)
			for _, node := range group.children {
				r := node.rule
				if filterType != "" && r.Type != filterType ||
					filterState != "" && (r.Type != api.AlertingRule || r.State != filterState) {
					continue
				}
				match := groupMatches || node.matches(f)
				at := len(s.visible)
				s.visible = append(s.visible, node)
				if len(f) != 0 || s.isOpen(node) {
					for _, a := range node.children {
						if match || a.matches(f) {
							s.visible = append(s.visible, a)
						}
					}
				}
				if !match && len(s.visible) == at+1 {
					s.visible = s.visible[:at]
				}
			}
			if len(s.visible) == groupAt+1 && (!groupMatches || len(group.children) != 0 || filterType != "" || filterState != "") {
				s.visible = s.visible[:groupAt]
			} else if len(f) == 0 && !s.isOpen(group) {
				s.visible = s.visible[:groupAt+1]
			}
		}
		if showRoot {
			switch {
			case len(s.visible) == rootAt+1 && root.source.err == nil && !root.source.loading &&
				(len(root.children) > 0 || root.source.data != nil && (!rootMatches || filterType != "" || filterState != "")):
				s.visible = s.visible[:rootAt]
			case len(f) == 0 && !s.isOpen(root):
				s.visible = s.visible[:rootAt+1]
			}
		}
	}
	row := s.selectedRow()
	if row < 0 || s.search.Focused() && !s.visible[row].matches(f) {
		s.selectNode(nil)
		if len(s.visible) > 0 {
			i := 0
			if len(f) != 0 {
				if match := slices.IndexFunc(s.visible, func(n *ruleNode) bool { return n.matches(f) }); match >= 0 {
					i = match
				}
			}
			s.selectNode(s.visible[i])
		}
	}
}

func (n *ruleNode) matches(f fuzzyFilter) bool {
	if f.matches(n.title) {
		return true
	}
	if n.alert != nil {
		return matchLabels(f, n.alert.Labels)
	}
	return n.rule != nil && (f.matches(n.rule.Expression) || matchLabels(f, n.rule.Labels) || matchLabels(f, n.rule.Annotations))
}

func (s *rulesState) selectNode(n *ruleNode) {
	id := ""
	if n != nil {
		id = n.id
	}
	if s.selected != id {
		s.selected, s.detailTop, s.notice = id, 0, ""
	}
}

func (s *rulesState) selectedRow() int {
	return slices.IndexFunc(s.visible, func(n *ruleNode) bool { return n.id == s.selected })
}

func (s *rulesState) selection() *ruleNode {
	if i := s.selectedRow(); i >= 0 {
		return s.visible[i]
	}
	return nil
}

func (s *rulesState) ensureVisible(room int) {
	row := s.selectedRow()
	s.listTop = min(s.listTop, max(0, len(s.visible)-room))
	if row >= 0 {
		s.listTop = min(row, max(s.listTop, row-room+1))
	}
}
