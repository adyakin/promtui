package app

import (
	"context"
	"errors"
	"slices"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	"promtui/internal/alertmanager"
	"promtui/internal/config"
)

const alertRefreshInterval = 30 * time.Second

var errAlertsCancelled = errors.New("Alertmanager refresh cancelled")

const (
	sortAlertDefault = "state/severity"
	sortAlertNewest  = "newest"
)

var defaultAlertSorts = []viewSort{{sortAlertDefault, sortAlertDefault}, {sortAlertNewest, sortAlertNewest}}

type alertDialog uint8

const (
	alertNoDialog alertDialog = iota
	alertDetails
	alertRelated
	alertStatusDialog
)

type alertsCompleted struct {
	id            uint64
	rows          []alertmanager.Alert
	err           error
	inhibition    *alertmanager.Inhibition
	inhibitionErr error
}
type alertsTick time.Time
type alertLabelsCopied struct{ err error }

type alertsState struct {
	rows, visible []alertRow
	counts        alertCounts
	row           int
	view          config.TableView
	sort          string
	sorts         []viewSort
	filter        string
	input         textinput.Model
	suggestions   alertSearchSuggestions
	filterAnchor  string
	filterRow     int
	structured    alertMatcherFilter
	saved         alertFiltersDialog

	dialog             alertDialog
	detail             alertRow // Snapshot so refresh cannot move or replace the open details.
	offset, helpOffset int
	help               bool
	detailStatus       string
	silenceNotice      string
	inhibition         alertInhibitionSnapshot
	detailInhibition   alertInhibitionSnapshot
	detailRelations    alertRelations
	detailTrail        []alertDetailVisit
	relatedRow         int

	requestID   uint64
	cancel      context.CancelFunc
	tickQueued  bool
	background  bool
	lastOK      time.Time
	lastDone    time.Time
	err         error
	refreshErr  error
	errorOffset int
}

func newAlertsState(view config.TableView) alertsState {
	input := textinput.New()
	input.Prompt = "  Filter: "
	input.Placeholder = "state, severity, alert, instance, duration"
	if view.Search != nil {
		input.Placeholder = "configured fields"
	}
	input.CharLimit = 4096
	input.SetVirtualCursor(false)
	return alertsState{input: input, view: view, sort: sortAlertDefault, sorts: viewSorts(config.AlertsView, defaultAlertSorts, view.Columns)}
}

func (a *alertsState) currentFilter() string {
	if a.input.Focused() {
		return a.input.Value()
	}
	return a.filter
}

func (a *alertsState) selection() string {
	if len(a.visible) > 0 {
		return a.visible[a.row].id
	}
	return ""
}

func (a *alertsState) rebuild(id string, row int, now time.Time) {
	filter := newFuzzyFilter(a.currentFilter())
	a.visible = a.visible[:0]
	for _, item := range a.rows {
		if (a.structured.match != nil && !a.structured.match(item.Labels)) || !a.matches(item, filter, now) {
			continue
		}
		a.visible = append(a.visible, item)
	}
	slices.SortFunc(a.visible, func(x, y alertRow) int {
		if a.sort != sortAlertDefault && a.sort != sortAlertNewest {
			if n := compareAlertField(x, y, a.sort, now); n != 0 {
				return n
			}
		}
		return compareAlertRows(x, y, a.sort == sortAlertNewest)
	})
	a.moveTo(row)
	for i, item := range a.visible {
		if item.id == id {
			a.row = i
			break
		}
	}
}

func (a *alertsState) matches(row alertRow, filter fuzzyFilter, now time.Time) bool {
	fields := a.view.Search
	if fields != nil {
		return matchesFields(filter, fields, func(field string) string { return row.field(field, now) })
	}
	return matchesFields(filter, defaultAlertFields[:], func(field string) string { return alertValue(row.field(field, now)) })
}

func (a *alertsState) moveTo(row int) { a.row = max(0, min(row, len(a.visible)-1)) }

func (a *alertsState) stop() {
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
		a.requestID++
	}
}

func (m *Model) refreshAlerts() tea.Cmd {
	if m.activeTab != alertsTab {
		return nil
	}
	return m.fetchAlerts()
}

func (m *Model) fetchAlerts() tea.Cmd {
	a := &m.alerts
	base := m.opt.Connection.AlertmanagerURL
	if m.quitting || base == "" || a.cancel != nil {
		return nil
	}
	timeout := m.opt.Timeout
	if timeout <= 0 {
		timeout = alertRefreshInterval
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	a.cancel = cancel
	a.background = false
	a.requestID++
	id := a.requestID
	client := alertmanager.Client{Base: base, HTTP: m.connectionHTTP()}
	return func() tea.Msg {
		defer cancel()
		type rulesResult struct {
			rules []alertmanager.InhibitRule
			err   error
		}
		status := make(chan rulesResult, 1)
		go func() {
			rules, err := client.InhibitRules(ctx)
			status <- rulesResult{rules, err}
		}()
		rows, err := client.Alerts(ctx)
		if err != nil {
			cancel()
			<-status
			return alertsCompleted{id: id, err: err}
		}
		result := <-status
		return alertsCompleted{
			id: id, rows: rows, inhibitionErr: result.err,
			inhibition: alertmanager.IndexInhibition(rows, result.rules, time.Now()),
		}
	}
}

func (m *Model) alertsOnEntry(now time.Time) tea.Cmd {
	a := &m.alerts
	if a.lastOK.IsZero() || now.Sub(a.lastOK) >= alertRefreshInterval {
		return m.refreshAlerts()
	}
	return m.scheduleAlertsTick(now)
}

// A pending tick is reused across tab switches and manual refreshes. It only
// starts a request while visible, then completion schedules the next tick.
func (m *Model) scheduleAlertsTick(now time.Time) tea.Cmd {
	a := &m.alerts
	if m.activeTab != alertsTab || m.quitting || a.tickQueued || a.cancel != nil || a.lastDone.IsZero() ||
		errors.Is(a.err, alertmanager.ErrConfig) {
		return nil
	}
	a.tickQueued = true
	return tea.Tick(max(0, a.lastDone.Add(alertRefreshInterval).Sub(now)), func(t time.Time) tea.Msg { return alertsTick(t) })
}

func (m *Model) onAlertsTick(now time.Time) tea.Cmd {
	a := &m.alerts
	a.tickQueued = false
	if now.Sub(a.lastDone) >= alertRefreshInterval && !errors.Is(a.err, alertmanager.ErrConfig) {
		cmd := m.refreshAlerts()
		if cmd != nil {
			a.background = true
		}
		return cmd
	}
	return m.scheduleAlertsTick(now)
}

func (m *Model) onAlertsCompleted(v alertsCompleted, now time.Time) tea.Cmd {
	a := &m.alerts
	if v.id != a.requestID || a.cancel == nil {
		return nil
	}
	a.cancel = nil
	a.lastDone = now
	if a.background && v.err != nil && !a.lastOK.IsZero() && a.err == nil {
		// Keep the previous list, relations and open details usable.
		a.refreshErr = v.err
		return m.scheduleAlertsTick(now)
	}
	a.refreshErr = nil
	a.setError(v.err, m.silences.editor != nil && m.silences.editor.fromAlert)
	if v.err == nil {
		id, row := a.selection(), a.row
		a.lastOK = now
		a.rows = make([]alertRow, 0, len(v.rows))
		for i, alert := range v.rows {
			row := newAlertRow(m.opt.Connection.AlertmanagerURL, alert)
			row.snapshotIndex = i
			a.rows = append(a.rows, row)
		}
		a.inhibition = alertInhibitionSnapshot{a.rows, v.inhibition, v.inhibitionErr}
		a.refreshAlertNames()
		a.counts = countAlerts(a.rows)
		a.rebuild(id, row, now)
	}
	return m.scheduleAlertsTick(now)
}

func (a *alertsState) edit(msg tea.Msg) tea.Cmd {
	before := a.input.Value()
	input, cmd := a.input.Update(msg)
	a.input = input
	if a.input.Value() != before {
		a.rebuild(a.selection(), a.row, time.Now())
		a.refreshSearchSuggestions()
	}
	return cmd
}

func (a *alertsState) setError(err error, keepDetails bool) {
	a.err, a.errorOffset = err, 0
	if err == nil {
		return
	}
	if !keepDetails && (a.dialog == alertDetails || a.dialog == alertRelated) {
		a.closeAlertDetails()
	}
	if a.input.Focused() {
		a.input.Blur()
		a.rebuild(a.filterAnchor, a.filterRow, time.Now())
	}
}

func (m *Model) alertsKey(k tea.KeyPressMsg) tea.Cmd {
	a := &m.alerts
	key := k.String()
	if k.Keystroke() == "alt+s" {
		a.help = false
		if a.dialog == alertStatusDialog {
			a.dialog = alertNoDialog
		} else {
			a.closeAlertDetails()
			a.dialog, a.offset = alertStatusDialog, 0
		}
		return nil
	}
	if a.input.Focused() && !a.help && a.dialog == alertNoDialog {
		switch key {
		case "enter":
			if a.searchSuggestionKey(key) {
				a.rebuild(a.selection(), a.row, time.Now())
			}
			a.filter = a.input.Value()
			a.input.Blur()
		case "esc":
			a.input.Blur()
			a.rebuild(a.filterAnchor, a.filterRow, time.Now())
		default:
			if a.searchSuggestionKey(key) {
				return nil
			}
			return a.edit(k)
		}
		return nil
	}
	if a.help || a.dialog != alertNoDialog {
		switch key {
		case "esc":
			if a.help {
				a.help = false
			} else if a.dialog == alertRelated {
				a.dialog = alertDetails
			} else if a.dialog != alertDetails || !a.backToAlert() {
				a.closeAlertDetails()
			}
		case "?":
			a.help, a.helpOffset = !a.help, 0
		case "up", "down", "pgup", "pgdown":
			if !a.help && a.dialog == alertRelated {
				m.moveRelatedAlert(key)
			} else {
				m.scrollAlertDialog(key)
			}
		case "i":
			if !a.help && a.dialog == alertDetails && len(a.detailRelations.links) > 0 {
				a.dialog = alertRelated
			}
		case "enter":
			if !a.help && a.dialog == alertRelated {
				a.followRelatedAlert()
			}
		case "p":
			if !a.help && a.dialog == alertDetails {
				return m.loadAlertQuery()
			}
		case "s":
			if !a.help && a.dialog == alertDetails {
				return m.openSilenceEditor(nil, a.detail.Labels, true)
			}
		case "y":
			if !a.help && a.dialog == alertDetails {
				text := alertLabelText(a.detail.Labels)
				return func() tea.Msg { return alertLabelsCopied{clipboard.WriteAll(text)} }
			}
		}
		return nil
	}
	switch key {
	case "m":
		return m.openAlertMatcherEditor()
	case "ctrl+o", "ctrl+s":
		return m.openAlertFilters(key == "ctrl+s")
	case "D", "shift+d":
		m.resetAlertView()
		return nil
	case "x":
		m.setAlertMatchers(alertMatcherFilter{})
		return nil
	}
	if a.err != nil {
		switch key {
		case "up", "down", "pgup", "pgdown", "home", "end":
			a.errorOffset = scrollPaneError(key, a.errorOffset, len(m.alertErrorRows()), max(1, m.height-5))
		case "r":
			return m.refreshAlerts()
		case "?":
			a.help, a.helpOffset = true, 0
		case "q":
			return m.act("quit", "", 0)
		}
		return nil
	}
	switch key {
	case "up":
		a.moveTo(a.row - 1)
	case "down":
		a.moveTo(a.row + 1)
	case "pgup":
		a.moveTo(a.row - m.alertPage())
	case "pgdown":
		a.moveTo(a.row + m.alertPage())
	case "home":
		a.moveTo(0)
	case "end":
		a.moveTo(len(a.visible) - 1)
	case "enter":
		if len(a.visible) > 0 {
			a.detailTrail = nil
			a.detailInhibition = a.inhibition
			a.openAlertDetails(a.visible[a.row])
		}
	case "/":
		if !a.lastOK.IsZero() {
			a.filterAnchor, a.filterRow = a.selection(), a.row
			a.input.SetValue(a.filter)
			a.input.CursorEnd()
			cmd := a.input.Focus()
			a.refreshSearchSuggestions()
			return cmd
		}
	case "s":
		if len(a.rows) > 0 {
			a.sort = nextViewSort(a.sorts, a.sort)
			a.rebuild(a.selection(), a.row, time.Now())
		}
	case "r":
		a.silenceNotice = ""
		return m.refreshAlerts()
	case "?":
		a.help, a.helpOffset = true, 0
	case "q":
		return m.act("quit", "", 0)
	}
	return nil
}
