package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/config"
)

const routingFixture = `
route:
  receiver: first
  routes:
    - matchers: ['team="app"']
      receiver: second
      routes:
        - matchers: ['severity="critical"']
receivers:
  - name: first
    webhook_configs: [{url: <secret>}]
  - name: second
    slack_configs: [{channel: "#oncall", api_url: <secret>}]
  - name: unused
`

func routingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.URL.Path != "/api/v2/status" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"config": map[string]string{"original": routingFixture}})
}

func loadRouting(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("missing routing command")
	}
	msg := cmd().(routingLoaded)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	m.Update(msg)
}

func TestRoutingNavigationSearchAndLayout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(routingHandler))
	defer server.Close()
	m := New(Options{Connection: config.Connection{AlertmanagerURL: server.URL}}, nil)
	loadRouting(t, m, pressAlt(m, '4'))
	s := &m.routing
	if len(s.routes.visible) != 2 {
		t.Fatalf("expected root and its immediate child: %v", s.routes.visible)
	}
	press(m, '/', 'c', 'r', 'i', 't', tea.KeyEnter)
	if s.routes.selected != "0/0/0" || !s.expanded["0/0"] {
		t.Fatal("search did not reveal the route in the collapsed branch")
	}
	press(m, tea.KeyEnter)
	details := ansi.Strip(m.screen())
	if s.dialog != routingReceiverDialog || !strings.Contains(details, "#oncall") ||
		!strings.Contains(details, "Configuration references: 2") || !strings.Contains(details, "<secret>") {
		t.Fatal(details)
	}
	if got := s.selectedReceiverYAML(); !strings.Contains(got, "- name: second") || !strings.Contains(got, "#oncall") {
		t.Fatalf("wrong YAML for receiver overlay: %q", got)
	}
	if cmd := m.routingModalKey("y"); cmd == nil {
		t.Fatal("receiver overlay did not offer to copy YAML")
	}
	press(m, '0', 'v')
	if m.activeTab != routingTab || s.showReceivers {
		t.Fatal("overlay leaked key events")
	}
	press(m, tea.KeyEsc, '0', '4', 'v', 'j')
	if s.routes.selected != "0/0/0" || s.routes.filter != "crit" || s.receivers.selected != "second" {
		t.Fatal("switching lost navigation state")
	}
	if text := ansi.Strip(m.screen()); !strings.Contains(text, "#oncall") || !strings.Contains(text, "inherited") {
		t.Fatal(text)
	}
	press(m, tea.KeyEnter, tea.KeyEnter)
	if s.showReceivers || s.routes.selected != "0/0" || s.routes.filter != "crit" {
		t.Fatal("did not jump to visible parent reference")
	}
	press(m, 'v', tea.KeyEnter, 'j', tea.KeyEnter)
	if s.showReceivers || s.routes.selected != "0/0/0" || s.receivers.selected != "second" {
		t.Fatal("did not jump to inherited reference")
	}
	press(m, 'v', '/', 'n', 'o', 't', 'h', 'e', 'r', 'e')
	if s.receivers.selected != "" {
		t.Fatal("expected no matches")
	}
	press(m, tea.KeyEsc)
	if s.receivers.selected != "second" {
		t.Fatal("cancelling search did not restore selection")
	}
	press(m, 'k', '/', 's', 'e', 'c', 'r', 'e', 't')
	if s.receivers.selected != "" {
		t.Fatal("secret was searchable")
	}
	press(m, tea.KeyEsc)
	if got := s.selectedReceiverYAML(); !strings.Contains(got, "webhook_configs") || strings.Contains(got, "slack_configs") {
		t.Fatalf("wrong YAML for selection: %q", got)
	}
	press(m, '/', 's', 'l', 'a', 'c', 'k')
	if s.receivers.selected != "second" {
		t.Fatal("receiver search did not match the integration type")
	}
	press(m, tea.KeyEsc)
	for _, size := range [][2]int{{24, 10}, {67, 18}, {68, 18}, {110, 28}} {
		m.resize(size[0], size[1])
		for _, view := range []bool{false, true} {
			s.showReceivers = view
			check := func() {
				t.Helper()
				lines := strings.Split(ansi.Strip(m.screen()), "\n")
				if len(lines) != size[1] || strings.TrimRight(lines[len(lines)-1], " ") != strings.TrimRight(ansi.Strip(m.footer()), " ") {
					t.Fatalf("bad footer or height at %v:\n%s", size, strings.Join(lines, "\n"))
				}
				for _, line := range lines {
					if ansi.StringWidth(line) > size[0] {
						t.Fatalf("row too wide at %v: %q", size, line)
					}
				}
			}
			check()
			press(m, tea.KeyEnter)
			check()
			press(m, tea.KeyEsc)
		}
	}
}

func TestReceiverYAMLScrollAndCopy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(routingHandler))
	defer server.Close()
	m := New(Options{Connection: config.Connection{AlertmanagerURL: server.URL}}, nil)
	loadRouting(t, m, pressAlt(m, '4'))
	press(m, 'v', 'j', tea.KeyTab)
	s := &m.routing
	m.resize(32, 14)
	width := m.explorerLayout().detailWidth
	wrapped := m.routingDetails(width)
	yaml := strings.TrimSuffix(s.selectedReceiverYAML(), "\n")
	yamlRows := wrapped[2:]
	end := slices.Index(yamlRows, "")
	if end < 3 || strings.Join(yamlRows[:end], "") != strings.ReplaceAll(yaml, "\n", "") {
		t.Fatalf("wrap lost YAML contents:\n%q\n%q", yamlRows, yaml)
	}
	press(m, tea.KeyEnd)
	if s.receivers.detailOffset == 0 || !strings.Contains(ansi.Strip(m.screen()), "inherited") {
		t.Fatal("scroll did not reach the route references")
	}
	m.resize(112, 24)
	press(m, tea.KeyHome)
	if !strings.Contains(ansi.Strip(m.screen()), "#oncall") || !strings.Contains(ansi.Strip(m.footer()), "Copy YAML") {
		t.Fatal("receiver YAML or copy hint missing after resize")
	}
	_, copyCmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if copyCmd == nil {
		t.Fatal("receiver details did not offer to copy YAML")
	}
	// Do not modify the host clipboard during tests.
	m.Update(receiverYAMLCopied{requestID: s.requestID})
	if !strings.Contains(m.screen(), "Copied receiver YAML") {
		t.Fatal("copy status not visible")
	}
	press(m, '/')
	press(m, 'y')
	if s.search.Value() != "y" {
		t.Fatal("copy binding intercepted search input")
	}
}

func TestRoutingMissingReceiver(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"config": map[string]string{
			"original": "route: {receiver: missing}\nreceivers: []",
		}})
	}))
	defer server.Close()
	m := New(Options{Connection: config.Connection{AlertmanagerURL: server.URL}}, nil)
	loadRouting(t, m, pressAlt(m, '4'))
	press(m, tea.KeyEnter)
	if !strings.Contains(ansi.Strip(m.screen()), "Receiver not present in runtime configuration") ||
		m.routingModalKey("y") != nil {
		t.Fatal("missing receiver should be shown as unavailable and cannot be copied")
	}
	press(m, tea.KeyEsc, 'v')
	if !strings.Contains(ansi.Strip(m.screen()), "No receivers configured") {
		t.Fatal("empty receiver list should have an unavailable state")
	}
}

func TestRoutingRefreshAndConnectionSwitch(t *testing.T) {
	var original atomic.Value
	original.Store(routingFixture)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"config": map[string]string{"original": original.Load().(string)}})
	}))
	defer server.Close()
	strict := config.Connection{Name: "strict", AlertmanagerURL: server.URL}
	insecure := config.Connection{Name: "insecure", AlertmanagerURL: server.URL, InsecureSkipVerify: true}
	missing := config.Connection{Name: "missing"}
	other := config.Connection{Name: "other", AlertmanagerURL: server.URL + "/other", InsecureSkipVerify: true}
	m := New(Options{Connection: insecure, Connections: []config.Connection{insecure, strict, missing, other}}, nil)
	loadRouting(t, m, pressAlt(m, '4'))
	press(m, 'v', 'j')
	snapshot := m.routing.data
	original.Store(strings.ReplaceAll(routingFixture, "#oncall", "#platform"))
	loadRouting(t, m, m.routingKey(tea.KeyPressMsg{Code: 'r', Text: "r"}))
	if m.routing.receivers.selected != "second" || snapshot == m.routing.data ||
		!strings.Contains(m.routing.selectedReceiverYAML(), "#platform") {
		t.Fatal("refresh did not update snapshot or preserve selection")
	}
	snapshot = m.routing.data
	original.Store("route: [")
	m.Update(m.refreshRouting()())
	if m.routing.data != snapshot || !strings.Contains(m.screen(), "Stale data") ||
		!strings.Contains(m.routing.selectedReceiverYAML(), "#platform") {
		t.Fatal("refresh failure discarded valid configuration")
	}
	pending := m.refreshRouting()
	oldID := m.routing.requestID
	press(m, 'c', tea.KeyDown)
	strictCmd := m.connectionKey("enter")
	if strictCmd == nil || m.routing.data != nil || m.opt.Connection.Name != "strict" || m.routing.selectedReceiverYAML() != "" {
		t.Fatal("connection switch did not reset snapshot and fetch")
	}
	if old := pending().(routingLoaded); !errors.Is(old.err, context.Canceled) {
		t.Fatalf("obsolete request was not cancelled: %v", old.err)
	}
	m.Update(routingLoaded{id: oldID, data: snapshot})
	if m.routing.data != nil {
		t.Fatal("obsolete result was accepted")
	}
	failed := strictCmd().(routingLoaded)
	if failed.err == nil {
		t.Fatal("strict connection reused insecure TLS settings")
	}
	m.Update(failed)
	press(m, 'c', tea.KeyDown, tea.KeyEnter)
	if m.routing.data != nil || m.routing.cancel != nil || !strings.Contains(m.screen(), "Set alertmanager_url") {
		t.Fatal("missing Alertmanager URL was not handled")
	}
	original.Store(strings.ReplaceAll(routingFixture, "#oncall", "#other"))
	press(m, 'c', tea.KeyDown)
	loadRouting(t, m, m.connectionKey("enter"))
	press(m, 'j')
	if text := m.routing.selectedReceiverYAML(); !strings.Contains(text, "#other") || strings.Contains(text, "#platform") {
		t.Fatalf("wrong receiver YAML after switching connections: %q", text)
	}
}

func TestRoutingTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	m := New(Options{Connection: config.Connection{AlertmanagerURL: server.URL}, Timeout: 50 * time.Millisecond}, nil)
	msg := pressAlt(m, '4')().(routingLoaded)
	if !errors.Is(msg.err, context.DeadlineExceeded) {
		t.Fatalf("expected timeout, got %v", msg.err)
	}
}
