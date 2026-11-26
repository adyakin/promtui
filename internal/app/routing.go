package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	"promtui/internal/alertmanager"
)

type routingLoaded struct {
	id   uint64
	data *alertmanager.Routing
	err  error
}

type receiverYAMLCopied struct {
	requestID uint64
	err       error
}

type routingView struct {
	selected, filter string
	visible          []string
	detailOffset     int
	details          bool
}

type routingDialog uint8

const (
	routingNoDialog routingDialog = iota
	routingReceiverDialog
	routingReferencesDialog
	routingHelpDialog
)

type routingState struct {
	data      *alertmanager.Routing
	err       error
	cancel    context.CancelFunc
	requestID uint64
	lastDone  time.Time

	routes, receivers routingView
	showReceivers     bool
	sortReceivers     bool
	expanded          map[string]bool
	searchTree        map[string]bool
	receiverIndex     map[string]int
	search            textinput.Model
	searchAnchor      string
	searchOffset      int
	dialog            routingDialog
	dialogRow         int
	errorOffset       int
	copyStatus        string
}

func newRoutingState() routingState {
	input := textinput.New()
	input.Prompt = " / "
	input.Placeholder = "Filter"
	input.CharLimit = 256
	input.SetVirtualCursor(false)
	return routingState{search: input, expanded: map[string]bool{alertmanager.RootRoute: true}}
}

func (s *routingState) current() *routingView {
	if s.showReceivers {
		return &s.receivers
	}
	return &s.routes
}

func (s *routingState) stop() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
		s.requestID++
	}
}

func (m *Model) refreshRouting() tea.Cmd {
	s := &m.routing
	if m.quitting || s.cancel != nil || m.opt.Connection.AlertmanagerURL == "" {
		return nil
	}
	client, ctx, cancel := m.alertmanagerRequest()
	s.requestID++
	id := s.requestID
	s.cancel = cancel
	s.copyStatus = ""
	return func() tea.Msg {
		defer cancel()
		data, err := client.Routing(ctx)
		return routingLoaded{id, data, err}
	}
}

func (m *Model) routingOnEntry(now time.Time) tea.Cmd {
	if m.routing.data == nil {
		return m.refreshRouting()
	}
	return m.onRoutingTick(now)
}

func (m *Model) routingRefreshDeadline() time.Time {
	s := &m.routing
	if s.cancel != nil || errors.Is(s.err, alertmanager.ErrConfig) {
		return time.Time{}
	}
	return refreshDeadline(s.lastDone, m.opt.AutoRefresh.Routing)
}

func (m *Model) scheduleRoutingTick(now time.Time) tea.Cmd {
	return m.schedulePaneTick(routingTab, m.routingRefreshDeadline(), now)
}

func (m *Model) onRoutingTick(now time.Time) tea.Cmd {
	if m.activeTab == routingTab && refreshDue(m.routingRefreshDeadline(), now) {
		return m.refreshRouting()
	}
	return m.scheduleRoutingTick(now)
}

func (m *Model) onRoutingLoaded(msg routingLoaded, now time.Time) tea.Cmd {
	s := &m.routing
	if msg.id != s.requestID || s.cancel == nil {
		return nil
	}
	s.cancel()
	s.lastDone = now
	s.cancel, s.err, s.errorOffset = nil, msg.err, 0
	if msg.err == nil {
		s.setData(msg.data)
	}
	return m.scheduleRoutingTick(now)
}

func (s *routingState) setData(data *alertmanager.Routing) {
	route, receiver := s.routes.selected, s.receivers.selected
	var reference string
	if s.dialog == routingReferencesDialog {
		reference = s.data.References[receiver][s.dialogRow]
	}
	s.data = data
	s.receiverIndex = make(map[string]int, len(s.data.Receivers))
	for i, receiver := range s.data.Receivers {
		s.receiverIndex[receiver.Name] = i
	}
	for id := range s.expanded {
		if s.data.Routes[id] == nil {
			delete(s.expanded, id)
		}
	}
	s.rebuild()
	switch s.dialog {
	case routingReceiverDialog:
		if node := s.data.Routes[route]; s.routes.selected != route || node == nil || node.Receiver.Value == "" {
			s.dialog = routingNoDialog
		}
	case routingReferencesDialog:
		if row := slices.Index(s.data.References[receiver], reference); s.receivers.selected == receiver && row >= 0 {
			s.dialogRow = row
		} else {
			s.dialog = routingNoDialog
		}
	}
}

func (s *routingState) filter(receivers bool) fuzzyFilter {
	if s.showReceivers == receivers && s.search.Focused() {
		return newFuzzyFilter(s.search.Value())
	}
	if receivers {
		return newFuzzyFilter(s.receivers.filter)
	}
	return newFuzzyFilter(s.routes.filter)
}

func (s *routingState) rebuild() {
	if s.data == nil {
		return
	}
	s.routes.visible = s.routes.visible[:0]
	f := s.filter(false)
	var include map[string]bool
	var firstMatch string
	selectedMatches := false
	if len(f) > 0 {
		include = make(map[string]bool)
		for _, id := range s.data.Order {
			node := s.data.Routes[id]
			match := f.matches(node.Receiver.Value) || id == alertmanager.RootRoute && f.matches("root")
			for _, matcher := range node.Matchers {
				match = match || f.matches(matcher.Name, matcher.Value, matcher.String())
			}
			if match {
				if firstMatch == "" {
					firstMatch = id
				}
				selectedMatches = selectedMatches || id == s.routes.selected
				// Show matching routes in context, even when their parent is collapsed.
				for at := id; at != "" && !include[at]; at = s.data.Routes[at].Parent {
					include[at] = true
				}
			}
		}
	}
	if s.search.Focused() && !s.showReceivers && !selectedMatches && firstMatch != "" {
		s.routes.selected, s.routes.detailOffset = firstMatch, 0
	}
	s.searchTree = include
	var collect func(string)
	collect = func(id string) {
		if include != nil && !include[id] {
			return
		}
		s.routes.visible = append(s.routes.visible, id)
		if include != nil || s.expanded[id] {
			for _, child := range s.data.Routes[id].Children {
				collect(child)
			}
		}
	}
	collect(alertmanager.RootRoute)
	s.receivers.visible = s.receivers.visible[:0]
	f = s.filter(true)
	for _, r := range s.data.Receivers {
		match := f.matches(r.Name)
		for _, i := range r.Integrations {
			match = match || f.matches(i)
		}
		if match {
			s.receivers.visible = append(s.receivers.visible, r.Name)
		}
	}
	if s.sortReceivers {
		slices.Sort(s.receivers.visible)
	}
	for _, v := range []*routingView{&s.routes, &s.receivers} {
		if !slices.Contains(v.visible, v.selected) {
			v.selected, v.detailOffset = "", 0
			if len(v.visible) > 0 {
				v.selected = v.visible[0]
			} else if !s.search.Focused() {
				v.details = false
			}
		}
	}
}

func (s *routingState) receiver(name string) *alertmanager.Receiver {
	if i, ok := s.receiverIndex[name]; ok {
		return &s.data.Receivers[i]
	}
	return nil
}

func (s *routingState) selectedReceiverYAML() string {
	if s.data == nil {
		return ""
	}
	var name string
	switch {
	case s.dialog == routingReceiverDialog:
		if route := s.data.Routes[s.routes.selected]; route != nil {
			name = route.Receiver.Value
		}
	case s.showReceivers && s.dialog == routingNoDialog:
		name = s.receivers.selected
	}
	if receiver := s.receiver(name); receiver != nil {
		return receiver.YAML
	}
	return ""
}

func (s *routingState) copyReceiverYAML() tea.Cmd {
	text, id := s.selectedReceiverYAML(), s.requestID
	if text == "" {
		return nil
	}
	return func() tea.Msg { return receiverYAMLCopied{id, clipboard.WriteAll(text)} }
}

func (s *routingState) reveal(id string) {
	for node := s.data.Routes[id]; node != nil && node.Parent != ""; node = s.data.Routes[node.Parent] {
		s.expanded[node.Parent] = true
	}
}

func (s *routingState) jump(id string) {
	s.showReceivers, s.dialog = false, routingNoDialog
	s.reveal(id)
	s.routes.selected, s.routes.detailOffset, s.routes.details = id, 0, false
	s.rebuild()
	if s.routes.selected != id {
		s.routes.filter, s.routes.selected = "", id
		s.rebuild()
	}
}

func routingNavKey(key string) string {
	switch key {
	case "j":
		return "down"
	case "k":
		return "up"
	}
	return key
}

func (m *Model) routingKey(msg tea.KeyPressMsg) tea.Cmd {
	s := &m.routing
	v := s.current()
	k := msg.String()
	if s.search.Focused() {
		switch k {
		case "esc":
			m.cancelRoutingSearch()
		case "enter":
			v.filter = s.search.Value()
			s.search.Blur()
			if !s.showReceivers && v.selected != "" {
				s.reveal(v.selected)
			}
			s.rebuild()
		default:
			return m.editRoutingSearch(msg)
		}
		return nil
	}
	switch k {
	case "v":
		s.showReceivers = !s.showReceivers
	case "r":
		return m.refreshRouting()
	case "q":
		return m.act("quit", "", 0)
	case "?":
		s.dialog, s.dialogRow = routingHelpDialog, 0
	default:
		if s.data == nil {
			s.errorOffset = scrollPaneError(routingNavKey(k), s.errorOffset, len(m.routingErrorRows()), m.height-5)
			return nil
		}
		switch k {
		case "y":
			return s.copyReceiverYAML()
		case "/":
			s.searchAnchor, s.searchOffset = v.selected, v.detailOffset
			s.search.SetValue(v.filter)
			s.search.CursorEnd()
			return s.search.Focus()
		case "tab", "shift+tab":
			if v.selected != "" {
				v.details = !v.details
			}
		case "s":
			if s.showReceivers {
				s.sortReceivers = !s.sortReceivers
				s.rebuild()
			}
		case "enter":
			if s.showReceivers {
				if len(s.data.References[v.selected]) > 0 {
					s.dialog, s.dialogRow = routingReferencesDialog, 0
				}
			} else if node := s.data.Routes[v.selected]; node != nil && node.Receiver.Value != "" {
				s.dialog, s.dialogRow = routingReceiverDialog, 0
			}
		case "h", "left", "l", "right":
			if !s.showReceivers && !v.details && len(s.filter(false)) == 0 {
				if node := s.data.Routes[v.selected]; node != nil {
					if k == "l" || k == "right" {
						if len(node.Children) != 0 {
							s.expanded[node.ID] = true
						}
					} else if s.expanded[node.ID] && len(node.Children) != 0 {
						delete(s.expanded, node.ID)
					} else if node.Parent != "" {
						v.selected, v.detailOffset = node.Parent, 0
					}
					s.rebuild()
				}
			}
		default:
			layout := m.explorerLayout()
			if v.details {
				v.detailOffset = scrollPaneError(routingNavKey(k), v.detailOffset, len(m.routingDetails(layout.detailWidth)), layout.detailRows)
			} else if len(v.visible) != 0 {
				row := slices.Index(v.visible, v.selected)
				if next := targetNavigate(routingNavKey(k), row, len(v.visible), layout.listRows-1); next != row {
					v.selected, v.detailOffset = v.visible[next], 0
					if !s.showReceivers && len(s.filter(false)) != 0 {
						s.reveal(v.selected)
					}
				}
			}
		}
	}
	return nil
}

func (m *Model) cancelRoutingSearch() {
	s := &m.routing
	if s.search.Focused() {
		v := s.current()
		s.search.Blur()
		v.selected, v.detailOffset = s.searchAnchor, s.searchOffset
		s.rebuild()
	}
}

func (m *Model) editRoutingSearch(msg tea.Msg) tea.Cmd {
	s := &m.routing
	before := s.search.Value()
	var cmd tea.Cmd
	s.search, cmd = s.search.Update(msg)
	if s.search.Value() != before {
		s.rebuild()
	}
	return cmd
}

func (s *routingState) routePath(id string) string {
	var path []string
	for node := s.data.Routes[id]; node != nil; node = s.data.Routes[node.Parent] {
		path = append(path, routeLabel(node))
	}
	slices.Reverse(path)
	return strings.Join(path, " → ")
}

func routeLabel(node *alertmanager.Route) string {
	if node.Parent == "" {
		return "root"
	}
	if len(node.Matchers) == 0 {
		return "(all alerts reaching this route)"
	}
	matchers := make([]string, 0, len(node.Matchers))
	for _, matcher := range node.Matchers {
		matchers = append(matchers, matcher.String())
	}
	return strings.Join(matchers, ", ")
}
