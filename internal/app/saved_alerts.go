package app

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	"gopkg.in/yaml.v3"
	"promtui/internal/parameters"
	"promtui/internal/savedalerts"
)

type alertMatcherFilter struct {
	matchers     []savedalerts.Matcher // Runtime values, never written back to a template.
	match        func(map[string]string) bool
	name         string
	searchEdited bool // Keep the matchers active even after the name is hidden.
	params       string
}

func alertFilterParameters(defs []parameters.Parameter, values parameters.Values) string {
	fields := make([]string, 0, len(defs))
	for _, p := range defs {
		value := values[p.Name]
		if value == "" || strings.ContainsAny(value, " \n\t\r") {
			value = strconv.Quote(value)
		}
		fields = append(fields, p.Name+"="+value)
	}
	return strings.Join(fields, ", ")
}

func (m *Model) setAlertMatchers(f alertMatcherFilter) {
	a := &m.alerts
	a.structured = f
	a.rebuild(a.selection(), a.row, time.Now())
}

type alertSavedMode uint8

const (
	alertSavedClosed alertSavedMode = iota
	alertSavedBrowse
	alertSavedSearch
	alertSavedName
	alertSavedConfirm
	alertSavedManual
	alertSavedTemplate
)

type alertFiltersDialog struct {
	mode     alertSavedMode
	snapshot savedalerts.Snapshot
	items    []savedalerts.Filter
	views    []string
	err      error
	openErr  error

	action       savedAction
	filter       string
	searchOrigin alertPickerItem
	matches      []alertPickerItem
	row, target  int
	input        textinput.Model
	editors      []alertMatcherInputs
	field        int
	template     *savedYAML
	clipboard    *alertClipboardRequest
}

type alertClipboardRequest struct {
	mode       alertSavedMode
	row, field int
}

type alertClipboard struct {
	request *alertClipboardRequest
	value   string
	err     error
}

type alertPickerKind uint8

const (
	alertPickerNone alertPickerKind = iota
	alertPickerFilter
	alertPickerView
)

// The kind determines whether this points into the saved-filter snapshot
// or the named view registry. A picker row is never a snapshot index.
type alertPickerItem struct {
	kind        alertPickerKind
	filterIndex int
	viewName    string
}

func alertFilterItem(index int) alertPickerItem {
	return alertPickerItem{kind: alertPickerFilter, filterIndex: index}
}

func (d *alertFiltersDialog) selected() alertPickerItem {
	if len(d.matches) == 0 {
		return alertPickerItem{}
	}
	return d.matches[d.row]
}

func (d *alertFiltersDialog) rebuild(filter string, selected alertPickerItem, row int) {
	d.matches = d.matches[:0]
	search := newFuzzyFilter(filter)
	for i, f := range d.items {
		if search.matches(f.Name) {
			d.matches = append(d.matches, alertFilterItem(i))
		}
	}
	for _, name := range d.views {
		if search.matches(name) {
			d.matches = append(d.matches, alertPickerItem{kind: alertPickerView, viewName: name})
		}
	}
	d.row = slices.Index(d.matches, selected)
	if d.row < 0 {
		d.row = max(0, min(row, len(d.matches)-1))
	}
}

func (m *Model) openAlertFilters(save bool) tea.Cmd {
	snapshot, err := savedalerts.Open(m.opt.SavedAlertsPath)
	m.alerts.saved = alertFiltersDialog{
		mode: alertSavedBrowse, snapshot: snapshot, items: snapshot.Filters(),
		err: err, openErr: err, target: -1,
	}
	d := &m.alerts.saved
	if save {
		d.mode, d.action = alertSavedName, addSaved
		if d.err != nil {
			return nil
		}
		if len(m.alerts.structured.matchers) == 0 {
			d.err = errors.New("apply label matchers with m before saving")
			return nil
		}
		return m.startAlertSavedInput("Name: ", "")
	}
	d.views = slices.Sorted(maps.Keys(m.opt.AlertViews))
	d.rebuild("", alertPickerItem{}, 0)
	return nil
}

func (m *Model) startAlertSavedInput(prompt, value string) tea.Cmd {
	d := &m.alerts.saved
	d.input = textinput.New()
	d.input.Prompt = prompt
	if d.mode == alertSavedSearch {
		d.input.Placeholder = "filters and views…"
	}
	d.input.SetVirtualCursor(false)
	d.input.SetValue(value)
	m.sizeAlertSavedInputs()
	return d.input.Focus()
}

func (m *Model) alertSavedInput(msg tea.Msg) tea.Cmd {
	d := &m.alerts.saved
	switch d.mode {
	case alertSavedSearch, alertSavedName:
		if !d.input.Focused() {
			return nil
		}
		previous := d.input.Value()
		var cmd tea.Cmd
		d.input, cmd = d.input.Update(msg)
		if previous != d.input.Value() {
			d.err = nil
			if d.mode == alertSavedSearch {
				d.rebuild(d.input.Value(), d.selected(), d.row)
			}
		}
		return cmd
	case alertSavedManual:
		return d.editMatcher(msg)
	case alertSavedTemplate:
		d.err = nil
		return d.template.edit(msg)
	}
	return nil
}

func (m *Model) closeAlertSaved() { m.alerts.saved = alertFiltersDialog{} }

func (m *Model) alertSavedBack() {
	d := &m.alerts.saved
	switch d.mode {
	case alertSavedSearch:
		d.rebuild(d.filter, d.searchOrigin, d.row)
	case alertSavedManual, alertSavedBrowse:
		m.closeAlertSaved()
		return
	case alertSavedName:
		if d.action == addSaved {
			m.closeAlertSaved()
			return
		}
	}
	d.input.Blur()
	d.mode, d.err = alertSavedBrowse, d.openErr
}

func (m *Model) alertSavedKey(k tea.KeyPressMsg) tea.Cmd {
	d := &m.alerts.saved
	key := k.Keystroke()
	if key == "esc" {
		m.alertSavedBack()
		return nil
	}
	if key == "ctrl+v" && (d.input.Focused() || d.mode == alertSavedManual && d.field != 1) {
		req := &alertClipboardRequest{d.mode, d.row, d.field}
		d.clipboard = req
		return func() tea.Msg {
			value, err := clipboard.ReadAll()
			return alertClipboard{req, value, err}
		}
	}
	switch d.mode {
	case alertSavedManual:
		return m.alertMatcherKey(k)
	case alertSavedTemplate:
		if key == "ctrl+enter" || key == "ctrl+g" {
			var fields yaml.Node
			if err := d.template.decode(&fields); err != nil {
				d.err = err
				return nil
			}
			item := savedalerts.Filter{Name: d.items[d.target].Name}
			if err := item.DecodeTemplate(&fields); err != nil {
				d.err = fmt.Errorf("invalid template YAML: %w", err)
				return nil
			}
			next, err := d.snapshot.Update(d.target, item)
			m.finishAlertSave(next, err)
			return nil
		}
		return m.alertSavedInput(k)
	case alertSavedName, alertSavedConfirm:
		if key == "enter" {
			m.applyAlertSave()
			return nil
		}
		return m.alertSavedInput(k)
	case alertSavedSearch, alertSavedBrowse:
		switch key {
		case "up", "down", "pgup", "pgdown":
			step := 1
			if strings.HasPrefix(key, "pg") {
				step = max(1, min(10, m.height-6))
			}
			if key == "up" || key == "pgup" {
				step = -step
			}
			d.row = max(0, min(d.row+step, len(d.matches)-1))
			return nil
		}
		if d.mode == alertSavedSearch {
			if key == "enter" {
				d.filter = d.input.Value()
				d.input.Blur()
				d.mode = alertSavedBrowse
				return nil
			}
			return m.alertSavedInput(k)
		}
		if key == "/" && (len(d.items) != 0 || len(d.views) != 0) {
			d.mode, d.searchOrigin, d.err = alertSavedSearch, d.selected(), nil
			return m.startAlertSavedInput("Search: ", d.filter)
		}
		selected := d.selected()
		if selected.kind == alertPickerView {
			if key == "enter" {
				if m.selectAlertView(selected.viewName) {
					m.closeAlertSaved()
				} else {
					d.err = fmt.Errorf("alert view %q is no longer configured", selected.viewName)
				}
			}
			return nil
		}
		if selected.kind != alertPickerFilter || d.openErr != nil {
			return nil
		}
		item := d.items[selected.filterIndex]
		switch key {
		case "enter":
			d.err = nil
			return m.loadAlertFilter(item)
		case "e":
			d.target, d.err = selected.filterIndex, nil
			d.mode, d.action = alertSavedName, renameSaved
			return m.startAlertSavedInput("Name: ", item.Name)
		case "u", "t":
			d.target, d.err = selected.filterIndex, nil
			d.action = updateSaved
			if key == "t" || len(item.Parameters) > 0 {
				return m.openAlertTemplate(item)
			}
			d.mode = alertSavedConfirm
		case "d":
			d.target, d.err = selected.filterIndex, nil
			d.mode, d.action = alertSavedConfirm, deleteSaved
		}
	}
	return nil
}

func (m *Model) applyAlertClipboard(p alertClipboard) tea.Cmd {
	d := &m.alerts.saved
	if m.quitting || m.activeTab != alertsTab || m.parameterDialog != nil || p.request == nil ||
		d.clipboard != p.request || *p.request != (alertClipboardRequest{d.mode, d.row, d.field}) {
		return nil
	}
	d.clipboard = nil
	if p.err != nil {
		d.err = fmt.Errorf("clipboard: %w", p.err)
		return nil
	}
	return m.alertSavedInput(tea.PasteMsg{Content: p.value})
}

func (m *Model) loadAlertFilter(item savedalerts.Filter) tea.Cmd {
	apply := func(values parameters.Values) (tea.Cmd, error) {
		matchers, match, err := item.Resolve(values)
		if err != nil {
			return nil, err
		}
		m.setAlertMatchers(alertMatcherFilter{matchers: matchers, match: match, name: item.Name,
			params: alertFilterParameters(item.Parameters, values)})
		m.closeAlertSaved()
		return nil, nil
	}
	if len(item.Parameters) != 0 {
		return m.openParameters(item.Name, item.Parameters, apply)
	}
	cmd, err := apply(nil)
	m.alerts.saved.err = err
	return cmd
}

func literalAlertFilter(name string, current []savedalerts.Matcher) savedalerts.Filter {
	item := savedalerts.Filter{Name: name, Matchers: slices.Clone(current)}
	for i := range item.Matchers {
		item.Matchers[i].Value = parameters.EscapeLiteral(item.Matchers[i].Value)
	}
	return item
}

func (m *Model) applyAlertSave() {
	d := &m.alerts.saved
	if d.openErr != nil {
		return
	}
	current := m.alerts.structured.matchers
	var next savedalerts.Snapshot
	var err error
	switch d.action {
	case addSaved, updateSaved:
		if len(current) == 0 {
			d.err = errors.New("apply label matchers with m before saving")
			return
		}
		if d.action == addSaved {
			next, err = d.snapshot.Add(literalAlertFilter(d.input.Value(), current))
		} else {
			next, err = d.snapshot.Update(d.target, literalAlertFilter(d.items[d.target].Name, current))
		}
	case renameSaved:
		next, err = d.snapshot.Rename(d.target, d.input.Value())
	case deleteSaved:
		next, err = d.snapshot.Delete(d.target)
	}
	m.finishAlertSave(next, err)
}

func (m *Model) finishAlertSave(next savedalerts.Snapshot, err error) {
	d := &m.alerts.saved
	d.err = err
	if err != nil {
		return
	}
	if d.action == addSaved {
		active := &m.alerts.structured
		active.name = next.Filters()[len(d.items)].Name
		active.searchEdited = false
		active.params = ""
		m.closeAlertSaved()
		return
	}
	active := &m.alerts.structured
	if active.name == d.items[d.target].Name {
		switch d.action {
		case renameSaved:
			active.name = next.Filters()[d.target].Name
		case deleteSaved:
			active.name = ""
		}
	}
	selected := alertFilterItem(d.target)
	if d.action == deleteSaved {
		selected = alertPickerItem{}
	}
	d.snapshot, d.items = next, next.Filters()
	d.rebuild(d.filter, selected, d.row)
	if d.action == renameSaved && d.selected() != selected {
		d.filter = ""
		d.rebuild("", selected, d.row)
	}
	d.input.Blur()
	d.mode = alertSavedBrowse
}

func alertMatcherHint(row int) string { return fmt.Sprintf("Matcher %d", row+1) }
