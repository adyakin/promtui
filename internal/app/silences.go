package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"promtui/internal/alertmanager"
)

const (
	silenceStatusSort = "status"
	silenceEndSort    = "end time"
	silenceAuthorSort = "creator"
)

var silenceSorts = []viewSort{{silenceStatusSort, silenceStatusSort}, {silenceEndSort, silenceEndSort}, {silenceAuthorSort, silenceAuthorSort}}

type silenceMatches struct {
	id     uint64
	cancel context.CancelFunc
	valid  bool
	rows   []alertmanager.Alert
	err    error
}

func cancelSilenceRequest(cancel *context.CancelFunc) {
	if *cancel != nil {
		(*cancel)()
		*cancel = nil
	}
}

func (p *silenceMatches) reset() {
	cancelSilenceRequest(&p.cancel)
	*p = silenceMatches{}
}

type silenceConfirm uint8

const (
	noSilenceConfirm silenceConfirm = iota
	confirmSilenceExpire
	confirmSilenceBroad
)

type silencesState struct {
	rows, visible    []alertmanager.Silence
	counts           [3]int
	row              int
	includeExpired   bool
	sort, filter     string
	search           textinput.Model
	anchor           string
	anchorRow        int
	lastOK, lastDone time.Time
	listErr          error
	notice           string
	wantedID         string
	serial, listID   uint64
	listCancel       context.CancelFunc

	detail        *alertmanager.Silence
	detailID      uint64
	detailCancel  context.CancelFunc
	detailErr     error
	detailMatches silenceMatches
	detailOffset  int
	editor        *silenceDraft
	showMatches   bool
	matchesOffset int
	confirm       silenceConfirm
	confirmOffset int
	help          bool
	helpOffset    int
	writeID       uint64
	writeCancel   context.CancelFunc
	confirmErr    error
}

type silencesLoaded struct {
	id   uint64
	rows []alertmanager.Silence
	err  error
}
type silenceDetailLoaded struct {
	id  uint64
	row alertmanager.Silence
	err error
}
type silenceMatchesLoaded struct {
	id     uint64
	editor bool
	rows   []alertmanager.Alert
	err    error
}
type silenceWritten struct {
	id        uint64
	silenceID string
	expire    bool
	err       error
}

func newSilencesState() silencesState {
	search := silenceInput("", "matchers, creator, comment, status")
	search.Prompt = " Search: "
	return silencesState{sort: silenceStatusSort, search: search}
}

func (s *silencesState) modal() bool { return s.editor != nil || s.detail != nil || s.help }

func (s *silencesState) nextID() uint64 {
	s.serial++
	return s.serial
}

func (s *silencesState) selection() string {
	if len(s.visible) > 0 {
		return s.visible[s.row].ID
	}
	return ""
}

func (s *silencesState) closeDetail() {
	cancelSilenceRequest(&s.detailCancel)
	s.detailMatches.reset()
	s.detail, s.detailErr, s.detailOffset = nil, nil, 0
}

func (s *silencesState) closeEditor() {
	if s.editor != nil {
		s.editor.preview.reset()
		s.editor = nil
	}
}

func (s *silencesState) stop() {
	cancelSilenceRequest(&s.listCancel)
	cancelSilenceRequest(&s.writeCancel)
	s.closeDetail()
	s.closeEditor()
}

func silenceRank(s alertmanager.Silence, now time.Time) int {
	switch s.State(now) {
	case "active":
		return 0
	case "pending":
		return 1
	default:
		return 2
	}
}

func (s *silencesState) rebuild(id string, row int, now time.Time) {
	query := s.filter
	if s.search.Focused() {
		query = s.search.Value()
	}
	f := newFuzzyFilter(query)
	s.counts = [3]int{}
	s.visible = s.visible[:0]
	for _, row := range s.rows {
		rank := silenceRank(row, now)
		s.counts[rank]++
		if rank == 2 && !s.includeExpired {
			continue
		}
		if !f.matches(row.State(now), row.CreatedBy, row.Comment) && !slices.ContainsFunc(row.Matchers, func(m alertmanager.Matcher) bool {
			return f.matches(m.Name, m.Value, m.String())
		}) {
			continue
		}
		s.visible = append(s.visible, row)
	}
	slices.SortFunc(s.visible, func(a, b alertmanager.Silence) int {
		x, y := silenceRank(a, now), silenceRank(b, now)
		if (x == 2) != (y == 2) {
			return cmp.Compare(x, y)
		}
		switch s.sort {
		case silenceStatusSort:
			if n := cmp.Compare(x, y); n != 0 {
				return n
			}
		case silenceAuthorSort:
			if n := cmp.Compare(strings.ToLower(a.CreatedBy), strings.ToLower(b.CreatedBy)); n != 0 {
				return n
			}
		}
		if n := a.EndsAt.Compare(b.EndsAt); n != 0 {
			return n
		}
		return cmp.Compare(a.ID, b.ID)
	})
	s.row = max(0, min(row, len(s.visible)-1))
	if i := slices.IndexFunc(s.visible, func(s alertmanager.Silence) bool { return s.ID == id }); i >= 0 {
		s.row = i
	}
}

func (m *Model) refreshSilences() tea.Cmd {
	s := &m.silences
	if m.quitting || m.opt.Connection.AlertmanagerURL == "" || s.listCancel != nil {
		return nil
	}
	client, ctx, cancel := m.alertmanagerRequest()
	id := s.nextID()
	s.listID, s.listCancel = id, cancel
	return func() tea.Msg {
		defer cancel()
		rows, err := client.Silences(ctx)
		return silencesLoaded{id, rows, err}
	}
}

func (m *Model) onSilencesEntry(now time.Time) tea.Cmd {
	if m.silences.lastOK.IsZero() {
		return m.refreshSilences()
	}
	return m.onSilencesTick(now)
}

func (m *Model) silencesRefreshDeadline() time.Time {
	s := &m.silences
	if s.listCancel != nil || errors.Is(s.listErr, alertmanager.ErrConfig) {
		return time.Time{}
	}
	return refreshDeadline(s.lastDone, m.opt.AutoRefresh.Silences)
}

func (m *Model) scheduleSilencesTick(now time.Time) tea.Cmd {
	// Local status transitions do not depend on API polling.
	next := m.silencesRefreshDeadline()
	for _, row := range m.silences.rows {
		if row.State(now) != "expired" {
			for _, t := range []time.Time{row.StartsAt, row.EndsAt} {
				if t.After(now) && (next.IsZero() || t.Before(next)) {
					next = t
				}
			}
		}
	}
	return m.schedulePaneTick(silencesTab, next, now)
}

func (m *Model) onSilencesTick(now time.Time) tea.Cmd {
	s := &m.silences
	if m.activeTab != silencesTab {
		return nil
	}
	s.rebuild(s.selection(), s.row, now)
	if refreshDue(m.silencesRefreshDeadline(), now) {
		if cmd := m.refreshSilences(); cmd != nil {
			return tea.Batch(cmd, m.scheduleSilencesTick(now))
		}
	}
	return m.scheduleSilencesTick(now)
}

func (m *Model) onSilencesLoaded(v silencesLoaded, now time.Time) tea.Cmd {
	s := &m.silences
	if s.listCancel == nil || v.id != s.listID {
		return nil
	}
	s.listCancel, s.lastDone, s.listErr = nil, now, v.err
	id := s.selection()
	if v.err == nil {
		if s.wantedID != "" {
			id, s.wantedID = s.wantedID, ""
		}
		s.lastOK, s.rows = now, v.rows
	}
	s.rebuild(id, s.row, now)
	return m.scheduleSilencesTick(now)
}

func (m *Model) fetchSilenceDetail() tea.Cmd {
	s := &m.silences
	cancelSilenceRequest(&s.detailCancel)
	s.detailMatches.reset()
	s.detailErr = nil
	client, ctx, cancel := m.alertmanagerRequest()
	id, target := s.nextID(), s.detail.ID
	s.detailCancel, s.detailID = cancel, id
	return func() tea.Msg {
		defer cancel()
		row, err := client.Silence(ctx, target)
		return silenceDetailLoaded{id, row, err}
	}
}

func (m *Model) onSilenceDetailLoaded(v silenceDetailLoaded) tea.Cmd {
	s := &m.silences
	if s.detailCancel == nil || s.detailID != v.id {
		return nil
	}
	s.detailCancel, s.detailErr = nil, v.err
	if v.err == nil {
		s.detail = &v.row
	}
	return m.fetchSilenceMatches(false, s.detail.Matchers)
}

func (m *Model) fetchSilenceMatches(editor bool, matchers []alertmanager.Matcher) tea.Cmd {
	s := &m.silences
	p := &s.detailMatches
	if editor {
		p = &s.editor.preview
	}
	p.reset()
	matches, _, err := alertmanager.CompileMatchers(matchers)
	if err != nil {
		p.err = err
		return nil
	}
	client, ctx, cancel := m.alertmanagerRequest()
	id := s.nextID()
	p.id, p.cancel = id, cancel
	return func() tea.Msg {
		defer cancel()
		rows, err := client.Alerts(ctx)
		if err == nil {
			if slices.ContainsFunc(rows, func(a alertmanager.Alert) bool { return a.Labels == nil }) {
				return silenceMatchesLoaded{id: id, editor: editor, err: errors.New("Alertmanager returned an incomplete alert without labels")}
			}
			rows = slices.DeleteFunc(rows, func(a alertmanager.Alert) bool { return !matches(a.Labels) })
			slices.SortStableFunc(rows, func(a, b alertmanager.Alert) int {
				if n := cmp.Compare(a.Labels["alertname"], b.Labels["alertname"]); n != 0 {
					return n
				}
				return cmp.Compare(a.Labels["instance"], b.Labels["instance"])
			})
		}
		return silenceMatchesLoaded{id, editor, rows, err}
	}
}

func (m *Model) onSilenceMatchesLoaded(v silenceMatchesLoaded) {
	s := &m.silences
	p := &s.detailMatches
	if v.editor {
		if s.editor == nil {
			return
		}
		p = &s.editor.preview
	}
	if p.cancel != nil && p.id == v.id {
		p.cancel, p.valid, p.rows, p.err = nil, v.err == nil, v.rows, v.err
	}
}

func (m *Model) writeSilence(spec alertmanager.SilenceSpec, previous *alertmanager.Silence, expire bool) tea.Cmd {
	s := &m.silences
	if s.writeCancel != nil || m.opt.Connection.AlertmanagerURL == "" {
		return nil
	}
	client, ctx, cancel := m.alertmanagerRequest()
	id := s.nextID()
	s.writeID, s.writeCancel = id, cancel
	return func() tea.Msg {
		defer cancel()
		v := silenceWritten{id: id, expire: expire}
		if previous != nil {
			// API v2 has no compare-and-swap. Check just before writing so
			// an already expired/changed silence isn't silently replaced.
			current, err := client.Silence(ctx, previous.ID)
			switch {
			case err != nil:
				v.err = fmt.Errorf("check current silence: %w", err)
			case current.State(time.Now()) == "expired":
				v.err = errors.New("silence has already expired; reopen its details")
			case !current.UpdatedAt.Equal(previous.UpdatedAt) || !current.StartsAt.Equal(previous.StartsAt) ||
				!current.EndsAt.Equal(previous.EndsAt) || current.Comment != previous.Comment ||
				current.CreatedBy != previous.CreatedBy || !slices.Equal(current.Matchers, previous.Matchers) ||
				!maps.Equal(current.Annotations, previous.Annotations):
				v.err = errors.New("silence changed on Alertmanager; reopen its details before retrying")
			}
			if v.err != nil {
				return v
			}
		}
		if expire {
			v.silenceID = previous.ID
			v.err = client.ExpireSilence(ctx, previous.ID)
		} else {
			v.silenceID, v.err = client.SaveSilence(ctx, spec)
		}
		return v
	}
}

func (m *Model) onSilenceWritten(v silenceWritten, now time.Time) tea.Cmd {
	s := &m.silences
	if s.writeCancel == nil || s.writeID != v.id {
		return nil
	}
	s.writeCancel = nil
	if v.err != nil {
		s.notice = ""
		if s.editor != nil {
			s.editor.err = v.err
		} else {
			s.confirmErr = v.err
		}
		return nil
	}
	if v.expire || s.editor != nil && s.editor.original != nil && s.editor.original.ID != v.silenceID {
		oldID := s.detail.ID
		for i := range s.rows {
			if s.rows[i].ID == oldID {
				s.rows[i].Status.State = "expired"
				s.rows[i].EndsAt = now
				if s.rows[i].StartsAt.After(now) {
					s.rows[i].StartsAt = now
				}
			}
		}
		s.rebuild(s.selection(), s.row, now)
	}
	s.confirm, s.confirmErr = noSilenceConfirm, nil
	var detail tea.Cmd
	if v.expire {
		s.notice = "Silence expired"
		s.closeDetail()
	} else {
		s.wantedID = v.silenceID
		s.notice = "Silence created"
		if s.editor.original != nil {
			s.notice = "Silence updated"
			if s.detail.ID != v.silenceID {
				s.notice += " (Alertmanager assigned a new ID)"
			}
			updated := *s.detail
			updated.ID = v.silenceID
			s.detail = &updated
			detail = m.fetchSilenceDetail()
		} else {
			s.closeDetail()
		}
		if s.editor.fromAlert {
			m.alerts.closeAlertDetails()
			m.alerts.detailStatus = ""
			m.alerts.silenceNotice = s.notice
		}
	}
	s.closeEditor()
	cancelSilenceRequest(&s.listCancel)
	m.alerts.stop() // Do not reuse a snapshot requested before the write.
	return tea.Batch(detail, m.refreshSilences(), m.fetchAlerts())
}
