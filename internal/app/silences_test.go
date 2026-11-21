package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"promtui/internal/alertmanager"
	"promtui/internal/config"
)

func testSilence(id, state string, start, end time.Time) alertmanager.Silence {
	row := alertmanager.Silence{SilenceSpec: alertmanager.SilenceSpec{
		ID: id, StartsAt: start, EndsAt: end, CreatedBy: "ops " + id, Comment: "maintenance",
		Matchers: []alertmanager.Matcher{{Name: "alertname", Value: "Down", IsEqual: true}},
	}, UpdatedAt: start.Add(-time.Minute)}
	row.Status.State = state
	return row
}

func silenceKey(m *Model, code rune, mod tea.KeyMod) tea.Cmd {
	k := tea.KeyPressMsg{Code: code, Mod: mod}
	if mod == 0 && code >= 32 && code < 127 {
		k.Text = string(code)
	}
	_, cmd := m.Update(k)
	return cmd
}

func focusTestSilenceInput(t *testing.T, m *Model, label string) {
	t.Helper()
	for _, row := range m.silenceEditorGrid().rows {
		for _, field := range row.fields {
			if field.label == label {
				m.focusSilenceField(field.id)
				return
			}
		}
	}
	t.Fatalf("no field %q", label)
}

func TestSilenceList(t *testing.T) {
	now := time.Now().UTC()
	m := New(Options{Connection: config.Connection{AlertmanagerURL: "http://alertmanager.invalid"}}, nil)
	s := &m.silences
	t.Cleanup(s.stop)
	if silenceKey(m, '2', 0) == nil || m.activeTab != silencesTab || !strings.Contains(m.screen(), "Loading silences") {
		t.Fatal(m.screen())
	}
	pending := testSilence("next", "pending", now.Add(time.Minute), now.Add(10*time.Minute))
	active := testSilence("active", "active", now.Add(-time.Minute), now.Add(5*time.Minute))
	active.Matchers[0].Value = "HighCPU"
	expired := testSilence("past", "expired", now.Add(-time.Hour), now.Add(-time.Minute))
	m.onSilencesLoaded(silencesLoaded{s.listID, []alertmanager.Silence{expired, pending, active}, nil}, now)
	if s.counts != [3]int{1, 1, 1} || len(s.visible) != 2 || s.selection() != "active" {
		t.Fatalf("incorrect status counts or ordering: %v %v", s.counts, s.visible)
	}
	press(m, tea.KeyDown, 'e')
	if len(s.visible) != 3 || s.selection() != pending.ID || s.visible[2].ID != expired.ID {
		t.Fatal("expired toggle lost selection or moved expired ahead")
	}
	press(m, '/', 'h', 'c', 'p', tea.KeyEnter, 'e', 's', 's')
	if len(s.visible) != 1 || s.selection() != active.ID || s.sort != silenceAuthorSort || s.filter != "hcp" {
		t.Fatal("fuzzy search, filter, or sort failed")
	}
	press(m, '/', tea.KeyBackspace, 'z', tea.KeyEsc)
	if len(s.visible) != 1 || s.selection() != active.ID || s.filter != "hcp" {
		t.Fatal("cancel did not restore applied search")
	}
	refresh := m.refreshSilences()
	if refresh == nil {
		t.Fatal("cannot refresh")
	}
	m.onSilencesLoaded(silencesLoaded{id: s.listID, err: alertmanager.ErrAuth}, now)
	if s.selection() != active.ID || !strings.Contains(m.screen(), "Refresh failed") {
		t.Fatal("failed refresh discarded cached data")
	}
	_ = m.refreshSilences()
	m.onSilencesLoaded(silencesLoaded{s.listID, []alertmanager.Silence{pending, active, expired}, nil}, now)
	if s.selection() != active.ID {
		t.Fatal("refresh lost selected ID")
	}
	s.filter = ""
	m.onSilencesTick(now.Add(6 * time.Minute))
	if s.counts != [3]int{1, 0, 2} || s.selection() != pending.ID || len(s.visible) != 1 {
		t.Fatal("local time transition did not reclassify silences")
	}
}

func TestSilenceDraftValidation(t *testing.T) {
	now := time.Date(2026, 1, 20, 15, 0, 13, 123, time.UTC)
	for _, tt := range []struct {
		in   string
		want time.Duration
	}{{"30m", 30 * time.Minute}, {"7d", 7 * 24 * time.Hour}, {"1d2h30m", 26*time.Hour + 30*time.Minute},
		{".5d", 12 * time.Hour}, {"1.2.3d", 0}, {"300000d", 0}} {
		got, err := parseSilenceDuration(tt.in)
		if got != tt.want || (err == nil) != (tt.want != 0) {
			t.Fatalf("%q: %v, %v", tt.in, got, err)
		}
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ in, want string }{
		{"2026-03-08 02:30", "does not exist"}, {"2026-11-01 01:30", "ambiguous"}, {"2026-2-01 04:30", localSilencePlaceholder},
	} {
		if _, err := parseSilenceTime(tt.in, loc); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Fatalf("%q: %v", tt.in, err)
		}
	}
	at, err := parseSilenceTime("2026-11-01 02:30", loc)
	if err != nil || at.UTC().Hour() != 7 || at.UTC().Minute() != 30 {
		t.Fatalf("wrong timezone conversion: %v, %v", at, err)
	}
	d := newSilenceDraft("", nil, map[string]string{"state": "user-label", "spaced": " a\tvalue  ", "alertname": "Down"}, true, now)
	if _, _, err := d.spec(now); err == nil || !strings.Contains(err.Error(), "creator") {
		t.Fatal("missing creator not rejected")
	}
	d.creator.SetValue("ops")
	if _, _, err := d.spec(now); err == nil || !strings.Contains(err.Error(), "comment") {
		t.Fatal("missing comment not rejected")
	}
	d.comment.SetValue("planned")
	for _, in := range []string{"-2h", "0h", "nope"} {
		d.duration.SetValue(in)
		if _, _, err := d.spec(now); err == nil {
			t.Fatalf("invalid duration %q accepted", in)
		}
	}
	d.duration.SetValue("7d")
	later := now.Add(time.Hour)
	spec, broad, err := d.spec(later)
	if err != nil || broad || !spec.StartsAt.Equal(later) || spec.EndsAt.Sub(later) != 7*24*time.Hour ||
		spec.Matchers[1].Value != " a\tvalue  " || spec.Matchers[2].Name != "state" {
		t.Fatalf("unexpected alert draft: %+v %v", spec, err)
	}
	d.scheduled, d.until = true, true
	future := now.Add(4 * time.Hour).Truncate(time.Minute)
	d.start.SetValue(future.Local().Format(localSilenceTime))
	d.end.SetValue(future.Add(time.Hour).Local().Format(localSilenceTime))
	spec, _, err = d.spec(now)
	if err != nil || !spec.StartsAt.Equal(future) || spec.EndsAt.Sub(future) != time.Hour || spec.StartsAt.Location() != time.UTC {
		t.Fatalf("unexpected scheduled draft: %+v %v", spec, err)
	}
	d.end.SetValue(d.start.Value())
	if _, _, err := d.spec(now); err == nil {
		t.Fatal("end equal to start accepted")
	}
	active := testSilence("active", "active", now.Add(-time.Minute), now.Add(time.Hour))
	active.Comment = "old\ncomment"
	edit := newSilenceDraft("default", &active, nil, false, now)
	if spec, _, err := edit.spec(now); err != nil || spec.ID != active.ID || !spec.StartsAt.Equal(active.StartsAt) ||
		!spec.EndsAt.Equal(active.EndsAt) || spec.Comment != active.Comment || spec.CreatedBy != active.CreatedBy {
		t.Fatalf("editing changed untouched fields: %+v %v", spec, err)
	}
	active.Status.State = "expired"
	copy := newSilenceDraft("default", &active, nil, false, now)
	if spec, _, err := copy.spec(now); err != nil || spec.ID != "" || spec.Comment != active.Comment ||
		!spec.StartsAt.Equal(now) || !slices.Equal(spec.Matchers, active.Matchers) {
		t.Fatalf("unexpected expired copy: %+v %v", spec, err)
	}
}

func TestSilenceCreatePreviewAndRetry(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /am/api/v2/silences":
			fmt.Fprint(w, `[]`)
		case "GET /am/api/v2/alerts":
			if r.URL.Query().Get("silenced") != "true" || r.URL.Query().Get("inhibited") != "true" {
				t.Error("preview omits suppressed alerts")
			}
			fmt.Fprint(w, `[{"labels":{"alertname":"Down"},"status":{"silencedBy":["old"],"state":"suppressed"}},
				{"labels":{"alertname":"Downstream"},"status":{"state":"active"}}]`)
		case "POST /am/api/v2/silences":
			n := attempts.Add(1)
			var payload alertmanager.SilenceSpec
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.ID != "" ||
				payload.CreatedBy != "ops" || payload.Comment != "maintenance p123" {
				t.Errorf("unexpected payload %+v %v", payload, err)
			}
			if n == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
			} else {
				fmt.Fprint(w, `{"silenceID":"new"}`)
			}
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	conn := config.Connection{Name: "test", URL: "http://metrics.invalid", AlertmanagerURL: server.URL + "/am", InsecureSkipVerify: true}
	m := New(Options{Connection: conn, Connections: []config.Connection{conn}, SilenceCreator: "ops"}, nil)
	t.Cleanup(m.silences.stop)
	conn.InsecureSkipVerify = false
	strict := New(Options{Connection: conn}, nil)
	t.Cleanup(strict.silences.stop)
	if msg := silenceKey(strict, '2', 0)().(silencesLoaded); msg.err == nil {
		t.Fatal("TLS verification was not applied by default")
	}
	if msg := silenceKey(m, '2', 0)().(silencesLoaded); msg.err != nil {
		t.Fatal(msg.err)
	} else {
		m.Update(msg)
	}
	press(m, 'n')
	s, d := &m.silences, m.silences.editor
	focusTestSilenceInput(t, m, "Comment: ")
	m.Update(tea.PasteMsg{Content: "maintenance "})
	press(m, 'p', '1', '2', '3')
	pressAlt(m, '0')
	pressAlt(m, 'c')
	if s.showMatches || m.pickingConnection || m.activeTab != silencesTab || d.comment.Value() != "maintenance p123" {
		t.Fatal("typing or modal isolation failed")
	}
	d.matchers[0].name.SetValue("alertname")
	d.matchers[0].value.SetValue("Down")
	preview := silenceKey(m, 'p', tea.ModCtrl)
	if preview == nil || !s.showMatches || !strings.Contains(m.screen(), "Loading matching alerts") {
		t.Fatal(m.screen())
	}
	first := preview().(silenceMatchesLoaded)
	m.Update(first)
	if !d.preview.valid || len(d.preview.rows) != 1 || len(d.preview.rows[0].Status.SilencedBy) != 1 {
		t.Fatalf("preview excluded silenced alert or overmatched: %+v", d.preview)
	}
	press(m, tea.KeyEsc)
	g := m.silenceEditorGrid()
	for _, row := range g.rows {
		for _, f := range row.fields {
			if f.input == &d.matchers[0].value {
				m.focusSilenceField(f.id)
			}
		}
	}
	d.matchers[0].value.CursorEnd()
	press(m, 'x')
	m.Update(first)
	if d.preview.valid || d.matchers[0].value.Value() != "Downx" {
		t.Fatal("matcher change failed or kept a stale preview")
	}
	press(m, tea.KeyBackspace)
	m.Update(tea.PasteMsg{Content: "\t\n"})
	if values, err := d.matcherValues(); err != nil || values[0].Value != "Down\t\n" {
		t.Fatalf("special characters were changed: %v, %v", values, err)
	}
	d.matchers[0].quoted = false
	d.matchers[0].value.SetValue("Down")
	submit := silenceKey(m, tea.KeyEnter, tea.ModCtrl)
	if submit == nil || silenceKey(m, 'g', tea.ModCtrl) != nil || attempts.Load() != 0 {
		t.Fatal("submission blocked synchronously or allowed duplicates")
	}
	msg := submit().(silenceWritten)
	m.Update(msg)
	if attempts.Load() != 1 || s.editor != d || d.comment.Value() != "maintenance p123" ||
		!strings.Contains(m.screen(), "HTTP 503") || s.notice != "" {
		t.Fatal(m.screen())
	}
	retry := silenceKey(m, 'g', tea.ModCtrl)().(silenceWritten)
	_, refresh := m.Update(retry)
	if attempts.Load() != 2 || s.editor != nil || s.wantedID != "new" || s.notice != "Silence created" ||
		refresh == nil || s.listCancel == nil || m.alerts.cancel == nil {
		t.Fatal("confirmed creation did not close editor and refresh")
	}
	m.alerts.stop()
}

func TestSilenceDetailsUpdateAndExpire(t *testing.T) {
	now := time.Now().UTC()
	initial := testSilence("old", "active", now.Add(-time.Hour), now.Add(time.Hour))
	changed := initial
	changed.UpdatedAt = changed.UpdatedAt.Add(time.Second)
	written := initial
	written.ID = "replacement"
	var updates, deletes atomic.Int32
	var current atomic.Pointer[alertmanager.Silence]
	current.Store(&initial)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v2/silences":
			_ = json.NewEncoder(w).Encode([]alertmanager.Silence{*current.Load()})
		case "GET /api/v2/silence/old", "GET /api/v2/silence/replacement":
			_ = json.NewEncoder(w).Encode(current.Load())
		case "POST /api/v2/silences":
			updates.Add(1)
			var payload alertmanager.SilenceSpec
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.ID != initial.ID || payload.Comment != "edited" {
				t.Errorf("incorrect update: %+v %v", payload, err)
			}
			fmt.Fprint(w, `{"silenceID":"replacement"}`)
		case "GET /api/v2/alerts":
			fmt.Fprint(w, `[]`)
		case "DELETE /api/v2/silence/replacement":
			deletes.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	m := New(Options{Connection: config.Connection{AlertmanagerURL: server.URL}}, nil)
	s := &m.silences
	t.Cleanup(s.stop)
	t.Cleanup(m.alerts.stop)
	load := silenceKey(m, '2', 0)().(silencesLoaded)
	m.Update(load)
	detail := silenceKey(m, tea.KeyEnter, 0)().(silenceDetailLoaded)
	_, fetch := m.Update(detail)
	m.Update(fetch())
	press(m, 'a')
	if screen := ansi.Strip(m.screen()); !strings.Contains(screen, "No alerts match (0)") || !strings.Contains(screen, "future alerts") {
		t.Fatal(screen)
	}
	press(m, tea.KeyEsc, 'e')
	s.editor.comment.SetValue("edited")
	current.Store(&changed)
	m.Update(silenceKey(m, 'g', tea.ModCtrl)())
	if updates.Load() != 0 || s.editor == nil || !strings.Contains(s.editor.err.Error(), "changed") {
		t.Fatal("stale edit reached the write endpoint")
	}
	current.Store(&initial)
	msg := silenceKey(m, tea.KeyEnter, tea.ModCtrl)().(silenceWritten)
	current.Store(&written)
	_, cmd := m.Update(msg)
	if updates.Load() != 1 || s.editor != nil || s.detail.ID != written.ID || cmd == nil {
		t.Fatal("update did not follow Alertmanager's replacement ID")
	}
	for _, c := range cmd().(tea.BatchMsg) {
		m.Update(c())
	}
	if s.selection() != written.ID || s.detail.State(now) != "active" {
		t.Fatal("update did not refresh the list and details")
	}
	press(m, 'x', tea.KeyEsc, 'x')
	if s.confirm != confirmSilenceExpire || deletes.Load() != 0 || !strings.Contains(m.screen(), "replacement") {
		t.Fatal("expiration bypassed confirmation")
	}
	expire := silenceKey(m, tea.KeyEnter, 0)
	if expire == nil || silenceKey(m, tea.KeyEnter, 0) != nil {
		t.Fatal("expiration accepted duplicate confirmation")
	}
	m.Update(expire())
	if deletes.Load() != 1 || s.detail != nil || s.confirm != noSilenceConfirm || len(s.visible) != 0 {
		t.Fatal("expired silence remained in the default view")
	}
	press(m, 'e')
	if len(s.visible) != 1 || s.visible[0].State(time.Now()) != "expired" {
		t.Fatal("expired entry was not retained")
	}
}

func TestSilenceEditorAlertNavigationAndLayout(t *testing.T) {
	conn := config.Connection{AlertmanagerURL: "http://alertmanager.invalid"}
	m := New(Options{Connection: conn}, nil)
	m.activeTab = alertsTab
	now := time.Now()
	row := testAlert("test", "Down", "critical", "suppressed", now)
	row.Labels["env"] = "prod"
	m.alerts.rows = []alertRow{newAlertRow(conn.AlertmanagerURL, row)}
	m.alerts.lastOK = now
	m.alerts.rebuild("", 0, now)
	press(m, 's')
	if m.alerts.sort != sortAlertNewest || m.silences.editor != nil {
		t.Fatal("s stopped sorting the Alerts list")
	}
	press(m, tea.KeyEnter, tea.KeyDown, 's')
	d := m.silences.editor
	if d == nil || !d.fromAlert || len(d.matchers) != len(row.Labels) {
		t.Fatal("missing actual alert labels in draft")
	}
	for _, r := range d.matchers {
		if _, ok := row.Labels[r.name.Value()]; !ok || r.operator != 0 {
			t.Fatal("unexpected label or operator")
		}
	}
	offset := m.alerts.offset
	matches := len(d.matchers)
	focusTestSilenceInput(t, m, "Add matcher")
	press(m, tea.KeyEnter)
	focusTestSilenceInput(t, m, "Remove")
	press(m, tea.KeyEnter)
	if len(d.matchers) != matches {
		t.Fatal("could not add and remove a matcher")
	}
	for _, size := range [][2]int{{24, 10}, {48, 14}, {80, 24}, {140, 35}} {
		m.resize(size[0], size[1])
		focusTestSilenceInput(t, m, "Comment: ")
		view := m.View()
		if view.Cursor == nil || view.Cursor.Y <= 1 || view.Cursor.Y >= size[1]-2 {
			t.Fatalf("focused field is inaccessible at %v", size)
		}
		screen := ansi.Strip(view.Content)
		rows := strings.Split(screen, "\n")
		if len(rows) != size[1] || !strings.Contains(rows[len(rows)-1], "Esc Back") {
			t.Fatalf("overlay changed layout/footer at %v:\n%s", size, screen)
		}
		for _, line := range rows {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("overflow at %v: %q", size, line)
			}
		}
	}
	press(m, tea.KeyEsc)
	if m.silences.editor != nil || m.alerts.dialog != alertDetails || m.alerts.offset != offset || m.activeTab != alertsTab {
		t.Fatal("cancel did not restore the same alert details")
	}
}

func TestSilenceStaleResponsesAfterConnectionSwitch(t *testing.T) {
	a := config.Connection{Name: "a", AlertmanagerURL: "http://a.invalid"}
	b := config.Connection{Name: "b", AlertmanagerURL: "http://b.invalid"}
	m := New(Options{Connection: a, Connections: []config.Connection{a, b}}, nil)
	s := &m.silences
	t.Cleanup(s.stop)
	old := silenceKey(m, '2', 0)
	id := s.listID
	press(m, 'c', tea.KeyDown, tea.KeyEnter)
	if obsolete := old().(silencesLoaded); !errors.Is(obsolete.err, context.Canceled) || s.listID == id {
		t.Fatalf("switch did not invalidate old request: %+v", obsolete)
	}
	m.Update(silencesLoaded{id, []alertmanager.Silence{testSilence("old", "active", time.Now(), time.Now().Add(time.Hour))}, nil})
	if len(s.rows) != 0 || s.listCancel == nil || m.opt.Connection.Name != "b" {
		t.Fatal("obsolete response replaced the new connection list")
	}
}

func TestSilencePreviewFailureAndBroadConfirmation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v2/alerts":
			w.WriteHeader(http.StatusServiceUnavailable)
		case "POST /api/v2/silences":
			fmt.Fprint(w, `{"silenceID":"broad"}`)
		default:
			t.Errorf("unexpected request to %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	m := New(Options{Connection: config.Connection{AlertmanagerURL: server.URL}, SilenceCreator: "ops"}, nil)
	s := &m.silences
	t.Cleanup(s.stop)
	m.activeTab = silencesTab
	press(m, 'n')
	d := s.editor
	d.comment.SetValue("broad example")
	d.matchers[0].name.SetValue("job")
	d.matchers[0].value.SetValue("unrelated")
	d.matchers[0].operator = 1 // != also matches missing jobs.
	m.Update(silenceKey(m, 'p', 0)())
	if d.preview.valid || !strings.Contains(m.screen(), "Preview failed") || strings.Contains(m.screen(), "No alerts match") {
		t.Fatal(m.screen())
	}
	press(m, tea.KeyEsc)
	if silenceKey(m, 'g', tea.ModCtrl) != nil || s.confirm != confirmSilenceBroad || s.writeCancel != nil {
		t.Fatal("broad silence was submitted without explicit confirmation")
	}
	press(m, tea.KeyEsc)
	if s.editor != d || d.comment.Value() != "broad example" || s.confirm != noSilenceConfirm {
		t.Fatal("broad cancellation discarded the draft or got stuck")
	}
	press(m, 'g') // Plain g on a control is not submit.
	_ = silenceKey(m, 'g', tea.ModCtrl)
	cmd := silenceKey(m, tea.KeyEnter, 0)
	if cmd == nil {
		t.Fatal("confirmation did not start the write")
	}
	if msg := cmd().(silenceWritten); msg.err != nil || msg.silenceID != "broad" {
		t.Fatalf("confirmed write failed: %+v", msg)
	}
}
