package app

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"promtui/internal/api"
	"promtui/internal/editor"
	"promtui/internal/query"
)

type alertNavigation struct {
	serial uint64
	cancel context.CancelFunc
	row    alertRow
	cache  struct {
		row alertRow
		at  time.Time
		api.VMAlert
	}
}

type alertRuleMatch struct {
	group    *api.RuleGroup
	rule     *api.Rule
	activeAt time.Time
}

type alertNavigationDone struct {
	serial            uint64
	action            string
	row               alertRow
	source            int
	meta              api.VMAlert
	rules             *api.Rules
	exact, candidates []alertRuleMatch
	graph             query.AlertGraph
	fromCache         bool
	err               error
}

func alertGeneratorExpression(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", errors.New("No generator URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("Invalid generator URL")
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" ||
		!strings.HasSuffix(u.Path, "/graph") {
		return "", errors.New("Not a Prometheus graph URL")
	}
	params, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", errors.New("Invalid generator URL query")
	}
	key := "g0.expr"
	if !params.Has(key) {
		key = "expr"
	}
	if !params.Has(key) {
		return "", errors.New("No PromQL in generator URL")
	}
	expression := params.Get(key)
	if strings.TrimSpace(expression) == "" {
		return "", errors.New("Generator query is empty")
	}
	return expression, nil
}

func (m *Model) navigateAlert(action string) tea.Cmd {
	m.cancelAlertNavigation()
	a := &m.alerts
	row := a.detail
	expression, err := alertGeneratorExpression(row.GeneratorURL)
	var ref api.VMAlertRef
	source := -1
	if err != nil {
		u, parseErr := url.Parse(row.GeneratorURL)
		if parseErr == nil && strings.HasSuffix(u.Path, "/alert") && (len(m.opt.Connection.VMAlert) != 0 || u.Query().Has("group_id") || u.Query().Has("alert_id")) {
			bases := make([]string, len(m.opt.Connection.VMAlert))
			for i, s := range m.opt.Connection.VMAlert {
				bases[i] = s.URL
			}
			ref, source, err = api.ResolveVMAlert(row.GeneratorURL, bases)
		}
		if err != nil {
			a.detailStatus = err.Error()
			return nil
		}
	} else if len(m.opt.Connection.VMAlert) == 0 {
		source = 0 // Prometheus rules are served by the current TSDB.
	}
	if action == "P" && expression != "" {
		return m.showAlertQuery(expression, "", time.Time{})
	}
	if action == "r" && source < 0 {
		a.detailStatus = "No Prometheus Rules source configured for this connection"
		return nil
	}

	n := &m.alertNavigation
	timeout := m.opt.Timeout
	if timeout <= 0 {
		timeout = alertRefreshInterval
	}
	ctx, cancel := context.WithCancel(context.Background())
	n.cancel, n.row = cancel, row
	msg := alertNavigationDone{serial: n.serial, action: action, row: row, source: source,
		meta: api.VMAlert{Expression: expression}}
	a.detailStatus = "Resolving alert query…"
	if action == "r" {
		a.detailStatus = "Resolving generating rule…"
	}
	httpClient := m.connectionHTTP()
	tsdb := api.Client{Base: m.opt.Connection.URL, HTTP: httpClient}
	var ruleAPI api.Client
	var cachedRules *api.Rules
	if source >= 0 {
		s := &m.rules.sources[source]
		ruleAPI = api.Client{Base: s.URL, HTTP: httpClient}
		cachedRules = s.data
	}
	if ref.AlertID != "" && sameAlertNavigation(n.cache.row, row) && time.Since(n.cache.at) < rulesRefreshInterval {
		msg.meta, msg.fromCache = n.cache.VMAlert, true
	}
	return func() tea.Msg {
		if ref.AlertID != "" && !msg.fromCache {
			request, done := context.WithTimeout(ctx, timeout)
			msg.meta, msg.err = ruleAPI.VMAlert(request, ref)
			done()
		}
		data := cachedRules
		if action == "r" && data == nil {
			request, done := context.WithTimeout(ctx, timeout)
			fresh, fetchErr := ruleAPI.Rules(request)
			done()
			if fetchErr != nil {
				msg.err = fetchErr
				return msg
			}
			msg.rules, data = &fresh, &fresh
		}
		msg.exact, msg.candidates = matchAlertRules(data, row, msg.meta, ref)
		if action == "r" && data != nil && msg.rules == nil && len(msg.exact) != 1 {
			// The cached snapshot may predate this alert's rule or instance.
			request, done := context.WithTimeout(ctx, timeout)
			fresh, fetchErr := ruleAPI.Rules(request)
			done()
			if fetchErr == nil {
				msg.rules = &fresh
				msg.exact, msg.candidates = matchAlertRules(msg.rules, row, msg.meta, ref)
			}
		}
		if action == "r" {
			if data != nil {
				msg.err = nil // Rules can still show search results if exact resolution failed.
			}
			return msg
		}
		if strings.TrimSpace(msg.meta.Expression) == "" && len(msg.exact) == 1 {
			fallback := msg.exact[0]
			msg.meta.Expression = fallback.rule.Expression
			if msg.meta.ActiveAt.IsZero() {
				msg.meta.ActiveAt = fallback.activeAt
			}
			if strings.TrimSpace(msg.meta.Expression) != "" {
				msg.err = nil
			}
		}
		if strings.TrimSpace(msg.meta.Expression) == "" && msg.err == nil {
			msg.err = errors.New("No alert expression available")
		}
		if msg.err == nil && action == "p" {
			now := time.Now()
			at := alertActivation(msg.meta, row)
			if at.IsZero() || at.After(now) || at.Before(now.Add(-presets[len(presets)-1])) {
				at = now
			}
			request, done := context.WithTimeout(ctx, timeout)
			msg.graph = query.BuildAlertGraph(request, tsdb, msg.meta.Expression, alertScopeLabels(row, msg.meta.Labels, msg.exact), at, ref.AlertID != "")
			done()
		}
		return msg
	}
}

func sameAlertNavigation(a, b alertRow) bool {
	return a.id == b.id && a.GeneratorURL == b.GeneratorURL && a.StartsAt.Equal(b.StartsAt)
}

func (m *Model) alertNavigationVisible() bool {
	a := &m.alerts
	return m.activeTab == alertsTab && a.dialog == alertDetails && !a.help && !m.pickingConnection &&
		!m.silences.modal() && !m.quitting && sameAlertNavigation(a.detail, m.alertNavigation.row)
}

func (m *Model) cancelAlertNavigation() {
	n := &m.alertNavigation
	n.serial++
	if n.cancel != nil {
		n.cancel()
		n.cancel = nil
		if m.alerts.detailStatus == "Resolving alert query…" || m.alerts.detailStatus == "Resolving generating rule…" {
			m.alerts.detailStatus = ""
		}
	}
}

func (m *Model) onAlertNavigation(msg alertNavigationDone) tea.Cmd {
	n := &m.alertNavigation
	if msg.serial != n.serial || n.cancel == nil || !m.alertNavigationVisible() {
		return nil
	}
	m.cancelAlertNavigation()
	if msg.meta.ID != "" && !msg.fromCache {
		n.cache.row, n.cache.at, n.cache.VMAlert = msg.row, time.Now(), msg.meta
	}
	if msg.err != nil {
		m.alerts.detailStatus = msg.err.Error()
		return nil
	}
	switch msg.action {
	case "r":
		return m.openAlertRule(msg)
	case "P":
		return m.showAlertQuery(msg.meta.Expression, "", time.Time{})
	default:
		return m.showAlertQuery(msg.graph.Expression, msg.graph.Notice, alertActivation(msg.meta, msg.row))
	}
}

func alertActivation(meta api.VMAlert, row alertRow) time.Time {
	if !meta.ActiveAt.IsZero() {
		return meta.ActiveAt
	}
	return row.StartsAt
}

func alertGraphWindow(now, activeAt time.Time) time.Duration {
	needed := time.Hour
	if !activeAt.IsZero() && activeAt.Before(now) {
		needed += min(presets[len(presets)-1], now.Sub(activeAt).Truncate(time.Minute))
	}
	for _, window := range presets {
		if window >= needed {
			return window
		}
	}
	return presets[len(presets)-1]
}

func (m *Model) showAlertQuery(expression, notice string, at time.Time) tea.Cmd {
	cmd, err := m.loadPromQLExpression(expression)
	if err != nil {
		m.alerts.detailStatus = err.Error()
		return nil
	}
	m.alerts.closeAlertDetails()
	m.alerts.stop()
	if notice == "" {
		return cmd
	}
	m.setQueryMode(true)
	m.opt.Window, m.opt.End = alertGraphWindow(time.Now(), at), time.Time{}
	run := m.run()
	m.notice, m.alertQueryNotice = notice, notice
	m.editor.Blur()
	return run
}

func (m *Model) loadPromQLExpression(expression string) (tea.Cmd, error) {
	if strings.TrimSpace(expression) == "" {
		return nil, errors.New("No rule expression available")
	}
	next := m.editor
	next.SetExactValue(expression)
	// The textarea can normalize characters or truncate input; don't replace
	// the draft unless it can hold the exact decoded expression.
	if next.Value() != expression {
		return nil, errors.New("Query exceeds editor limits or contains unsupported characters")
	}

	m.activeTab = promQLTab
	m.dismiss()
	m.panel = 0
	if m.resultFilter.input.Focused() {
		// Keep the current result filter applied while moving focus to the editor.
		m.act("finishFilter", "", filterKeys)
	}
	m.editor = next
	m.cursor = -1
	m.diagnostic = editor.Diagnostic(expression)
	return m.editor.Focus(), nil
}
