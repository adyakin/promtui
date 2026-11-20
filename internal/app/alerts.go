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

const alertRefreshInterval = 30 * time.Second

var errAlertsCancelled = errors.New("Alertmanager refresh cancelled")

type alertDialog uint8

const (
	alertNoDialog alertDialog = iota
	alertDetails
	alertStatusDialog
)

type alertsCompleted struct {
	id   uint64
	rows []alertmanager.Alert
	err  error
}
type alertsTick time.Time
type alertLabelsCopied struct{ err error }

type alertsState struct {
	rows, visible []alertRow
	counts        alertCounts
	row           int
	newestFirst   bool
	filter        string
	input         textinput.Model
	filterAnchor  string
	filterRow     int

	dialog             alertDialog
	detail             alertRow // Snapshot so refresh cannot move or replace the open details.
	offset, helpOffset int
	help               bool
	copyStatus         string

	requestID   uint64
	cancel      context.CancelFunc
	tickQueued  bool
	lastOK      time.Time
	lastDone    time.Time
	err         error
	errorOffset int
}

func newAlertsState() alertsState {
	input := textinput.New()
	input.Prompt = "  Filter: "
	input.Placeholder = "type a label value"
	input.CharLimit = 4096
	input.SetVirtualCursor(false)
	return alertsState{input: input}
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

func (a *alertsState) rebuild(id string, row int) {
	lower := strings.ToLower(a.currentFilter())
	a.visible = a.visible[:0]
	for _, item := range a.rows {
		if item.matches(lower) {
			a.visible = append(a.visible, item)
		}
	}
	slices.SortFunc(a.visible, func(x, y alertRow) int { return compareAlertRows(x, y, a.newestFirst) })
	a.moveTo(row)
	for i, item := range a.visible {
		if item.id == id {
			a.row = i
			break
		}
	}
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
	a := &m.alerts
	base := m.opt.Connection.AlertmanagerURL
	if m.activeTab != alertsTab || m.quitting || base == "" || a.cancel != nil {
		return nil
	}
	timeout := m.opt.Timeout
	if timeout <= 0 {
		timeout = alertRefreshInterval
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	a.cancel = cancel
	a.requestID++
	id := a.requestID
	client := alertmanager.Client{Base: base, HTTP: m.connectionHTTP()}
	return func() tea.Msg {
		defer cancel()
		rows, err := client.Alerts(ctx)
		return alertsCompleted{id, rows, err}
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
		return m.refreshAlerts()
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
	a.setError(v.err)
	if v.err == nil {
		id, row := a.selection(), a.row
		a.lastOK = now
		a.rows = make([]alertRow, 0, len(v.rows))
		for _, alert := range v.rows {
			a.rows = append(a.rows, newAlertRow(m.opt.Connection.AlertmanagerURL, alert))
		}
		a.counts = countAlerts(a.rows)
		a.rebuild(id, row)
	}
	return m.scheduleAlertsTick(now)
}

func (a *alertsState) edit(msg tea.Msg) tea.Cmd {
	before := a.input.Value()
	input, cmd := a.input.Update(msg)
	a.input = input
	if a.input.Value() != before {
		a.rebuild(a.selection(), a.row)
	}
	return cmd
}

func (a *alertsState) setError(err error) {
	a.err, a.errorOffset = err, 0
	if err == nil {
		return
	}
	if a.dialog == alertDetails {
		a.dialog = alertNoDialog
	}
	if a.input.Focused() {
		a.input.Blur()
		a.rebuild(a.filterAnchor, a.filterRow)
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
			a.dialog, a.offset = alertStatusDialog, 0
		}
		return nil
	}
	if a.input.Focused() && !a.help && a.dialog == alertNoDialog {
		switch key {
		case "enter":
			a.filter = a.input.Value()
			a.input.Blur()
		case "esc":
			a.input.Blur()
			a.rebuild(a.filterAnchor, a.filterRow)
		default:
			return a.edit(k)
		}
		return nil
	}
	if a.help || a.dialog != alertNoDialog {
		switch key {
		case "esc":
			if a.help {
				a.help = false
			} else {
				a.dialog = alertNoDialog
			}
		case "?":
			a.help, a.helpOffset = !a.help, 0
		case "up", "down", "pgup", "pgdown":
			m.scrollAlertDialog(key)
		case "y":
			if !a.help && a.dialog == alertDetails {
				text := alertLabelText(a.detail.Labels)
				return func() tea.Msg { return alertLabelsCopied{clipboard.WriteAll(text)} }
			}
		}
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
			a.dialog, a.detail, a.offset, a.copyStatus = alertDetails, a.visible[a.row], 0, ""
		}
	case "/":
		if !a.lastOK.IsZero() {
			a.filterAnchor, a.filterRow = a.selection(), a.row
			a.input.SetValue(a.filter)
			a.input.CursorEnd()
			return a.input.Focus()
		}
	case "s":
		if len(a.rows) > 0 {
			a.newestFirst = !a.newestFirst
			a.rebuild(a.selection(), a.row)
		}
	case "r":
		return m.refreshAlerts()
	case "?":
		a.help, a.helpOffset = true, 0
	case "q":
		return m.act("quit", "", 0)
	}
	return nil
}
