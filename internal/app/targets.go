package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"promtui/internal/api"
	"promtui/internal/config"
)

const (
	targetRefreshInterval = 30 * time.Second
	targetConcurrency     = 4
)

var errTargetsCancelled = errors.New("target refresh cancelled")

type targetSourceState struct {
	config.TargetSource
	fallback, loading bool
	lastOK            time.Time
	err               error
	rows              []targetRow
	counts            [targetAllActive]int
}

func (s targetSourceState) status() string {
	switch {
	case s.loading:
		return "Loading"
	case s.err != nil && !s.lastOK.IsZero():
		return "Stale"
	case s.err != nil:
		return "Error"
	case !s.lastOK.IsZero():
		return "OK"
	default:
		return "Pending"
	}
}

type targetsCompleted struct {
	id, source int
	data       api.Targets
	err        error
}

type targetsTick time.Time

type targetDialog int

const (
	noTargetDialog targetDialog = iota
	targetSources
	targetDetails
	targetHelp
	targetStatus
)

type targetsState struct {
	sources         []targetSourceState
	visible         []*targetRow
	source, row     int // source 0 means all; otherwise it is the source index plus one.
	health          int // Index into targetHealthOrder.
	activeHealth    int
	sort            targetSort
	filter, anchor  string
	anchorRow       int
	input           textinput.Model
	dialog          targetDialog
	dialogRow       int
	errorOffset     int
	dialogID        int
	detail          targetRow
	copyStatus      string
	requestID       int
	ctx             context.Context
	cancel          context.CancelFunc
	next, remaining int
	lastDone        time.Time
	tickQueued      bool
}

func newTargetsState(conn config.Connection) targetsState {
	input := textinput.New()
	input.Prompt = "  Filter: "
	input.Placeholder = "source, job, instance, URL, label"
	input.CharLimit = 4096
	input.SetVirtualCursor(false)
	a := targetsState{input: input}
	for _, src := range conn.Targets {
		a.sources = append(a.sources, targetSourceState{TargetSource: src})
	}
	if len(a.sources) == 0 && conn.URL != "" {
		a.sources = []targetSourceState{{TargetSource: config.TargetSource{Name: "default", URL: conn.URL}, fallback: true}}
	}
	return a
}

func (a *targetsState) currentFilter() string {
	if a.input.Focused() {
		return a.input.Value()
	}
	return a.filter
}

func (a *targetsState) matchesSource(i int) bool { return a.source == 0 || a.source == i+1 }

func (a *targetsState) hasError() bool {
	if a.source != 0 {
		return a.sources[a.source-1].err != nil
	}
	return slices.ContainsFunc(a.sources, func(src targetSourceState) bool { return src.err != nil })
}

func (a *targetsState) updateErrorView() {
	a.errorOffset = 0
	if a.dialog == targetDetails && a.sources[a.detail.sourceIndex].err != nil {
		a.dialog = noTargetDialog
	}
	if a.hasError() && a.input.Focused() {
		a.input.Blur()
		a.rebuild(a.anchor, a.anchorRow)
	}
}

func (a *targetsState) selection() string {
	if len(a.visible) != 0 {
		return a.visible[a.row].id
	}
	return ""
}

func (a *targetsState) moveTo(row int) { a.row = max(0, min(row, len(a.visible)-1)) }

func (a *targetsState) rebuild(id string, row int) {
	filter := strings.ToLower(a.currentFilter())
	health := targetHealthOrder[a.health]
	clear(a.visible)
	a.visible = a.visible[:0]
	for i, source := range a.sources {
		if !a.matchesSource(i) {
			continue
		}
		for j := range source.rows {
			item := &source.rows[j]
			if (item.health == health || health == targetAllActive && item.health != targetDropped) &&
				strings.Contains(item.search, filter) {
				a.visible = append(a.visible, item)
			}
		}
	}
	slices.SortFunc(a.visible, func(x, y *targetRow) int { return compareTargetRows(x, y, a.sort) })
	a.moveTo(row)
	for i, item := range a.visible {
		if item.id == id {
			a.row = i
			break
		}
	}
}

func (a *targetsState) stop(err error) {
	if a.cancel == nil {
		return
	}
	a.cancel()
	a.ctx, a.cancel = nil, nil
	a.requestID++
	for i := range a.sources {
		src := &a.sources[i]
		if src.loading {
			src.loading = false
			if err != nil {
				src.err = err
			}
		}
	}
	if err != nil {
		a.updateErrorView()
	}
	a.lastDone = time.Time{}
}

func (m *Model) refreshTargets() tea.Cmd {
	a := &m.targets
	if m.quitting || m.activeTab != targetsTab || a.cancel != nil || len(a.sources) == 0 {
		return nil
	}
	a.ctx, a.cancel = context.WithCancel(context.Background())
	a.requestID++
	a.next, a.remaining = 0, len(a.sources)
	for i := range a.sources {
		a.sources[i].loading = true
	}
	var cmds []tea.Cmd
	for range min(targetConcurrency, len(a.sources)) {
		cmds = append(cmds, m.nextTargetsRequest())
	}
	return tea.Batch(cmds...)
}

// Only the update loop advances the queue. Each completion frees one slot,
// so no source waits behind a slow request while capacity is available.
func (m *Model) nextTargetsRequest() tea.Cmd {
	a := &m.targets
	if a.next == len(a.sources) {
		return nil
	}
	i, id, ctx := a.next, a.requestID, a.ctx
	a.next++
	timeout := m.opt.Timeout
	if timeout <= 0 {
		timeout = targetRefreshInterval
	}
	client := api.Client{Base: a.sources[i].URL, HTTP: m.connectionHTTP()}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		data, err := client.Targets(ctx)
		return targetsCompleted{id, i, data, err}
	}
}

func (m *Model) onTargetsCompleted(v targetsCompleted, now time.Time) tea.Cmd {
	a := &m.targets
	if v.id != a.requestID || a.cancel == nil || !a.sources[v.source].loading {
		return nil
	}
	src := &a.sources[v.source]
	hadError := src.err != nil
	src.loading, src.err = false, v.err
	if (hadError || v.err != nil) && a.matchesSource(v.source) {
		a.updateErrorView()
	}
	if v.err == nil {
		id, row := a.selection(), a.row
		src.lastOK, src.rows = now, makeTargetRows(src.Name, v.data)
		src.counts = [targetAllActive]int{}
		for i := range src.rows {
			src.rows[i].sourceIndex = v.source
			src.counts[src.rows[i].health]++
		}
		a.rebuild(id, row)
	}
	a.remaining--
	if a.remaining > 0 {
		return m.nextTargetsRequest()
	}
	a.cancel()
	a.ctx, a.cancel, a.lastDone = nil, nil, now
	return m.scheduleTargetsTick(now)
}

func (m *Model) targetsOnEntry(now time.Time) tea.Cmd {
	a := &m.targets
	if a.lastDone.IsZero() || now.Sub(a.lastDone) >= targetRefreshInterval ||
		slices.ContainsFunc(a.sources, func(s targetSourceState) bool { return s.err != nil }) {
		return m.refreshTargets()
	}
	return m.scheduleTargetsTick(now)
}

func (m *Model) scheduleTargetsTick(now time.Time) tea.Cmd {
	a := &m.targets
	if m.activeTab != targetsTab || m.quitting || a.tickQueued || a.cancel != nil || a.lastDone.IsZero() {
		return nil
	}
	a.tickQueued = true
	return tea.Tick(max(0, a.lastDone.Add(targetRefreshInterval).Sub(now)), func(t time.Time) tea.Msg {
		return targetsTick(t)
	})
}

func (m *Model) onTargetsTick(now time.Time) tea.Cmd {
	a := &m.targets
	a.tickQueued = false
	if now.Sub(a.lastDone) >= targetRefreshInterval {
		return m.refreshTargets()
	}
	return m.scheduleTargetsTick(now)
}

func (a *targetsState) edit(msg tea.Msg) tea.Cmd {
	before := a.input.Value()
	var cmd tea.Cmd
	a.input, cmd = a.input.Update(msg)
	if a.input.Value() != before {
		a.rebuild(a.selection(), a.row)
	}
	return cmd
}
