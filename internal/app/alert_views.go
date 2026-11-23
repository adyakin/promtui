package app

import (
	"slices"

	"promtui/internal/config"
)

// Kept on Model because changing connections replaces alertsState.
type alertViewSelection struct {
	active     string
	remembered map[string]string
}

func (m *Model) alertViewName() string { return m.alertViews.active }

func (m *Model) restoreAlertView() {
	connection := m.opt.Connection.Name
	if m.selectAlertView(m.alertViews.remembered[connection]) {
		return
	}
	m.resetAlertView()
}

func (m *Model) resetAlertView() {
	c := m.opt.Connection
	delete(m.alertViews.remembered, c.Name)
	if view, ok := m.opt.AlertViews[c.AlertDefaultView]; ok && c.AlertDefaultView != "" {
		m.alertViews.active = c.AlertDefaultView
		m.alerts.setViewColumns(view.Columns)
		return
	}
	m.alertViews.active = ""
	m.alerts.setViewColumns(c.Alerts.Columns)
}

func (m *Model) selectAlertView(name string) bool {
	view, ok := m.opt.AlertViews[name]
	if !ok || name == "" {
		return false
	}
	if m.alertViews.remembered == nil {
		m.alertViews.remembered = make(map[string]string)
	}
	m.alertViews.remembered[m.opt.Connection.Name] = name
	m.alertViews.active = name
	m.alerts.setViewColumns(view.Columns)
	return true
}

func (a *alertsState) setViewColumns(columns []config.Column) {
	// The named view only replaces columns, not search or the existing
	// filtered/sorted rows and selection.
	sortTitle := viewSortTitle(a.sorts, a.sort)
	a.view.Columns = columns
	a.sorts = viewSorts(config.AlertsView, defaultAlertSorts, columns)
	if a.sort != "" && !slices.ContainsFunc(a.sorts, func(s viewSort) bool { return s.field == a.sort }) {
		// Keep the current sort usable even if its column isn't displayed.
		if sortTitle == "" {
			sortTitle = columnTitle(config.AlertsView, config.Column{Field: a.sort})
		}
		if sortTitle == "" {
			sortTitle = a.sort
		}
		a.sorts = append(slices.Clone(a.sorts), viewSort{a.sort, sortTitle})
	}
}
