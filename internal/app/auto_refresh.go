package app

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
)

func refreshDeadline(lastDone time.Time, interval time.Duration) time.Time {
	if lastDone.IsZero() || interval <= 0 {
		return time.Time{}
	}
	return lastDone.Add(interval)
}

func refreshDue(deadline, now time.Time) bool {
	return !deadline.IsZero() && !now.Before(deadline)
}

type paneRefreshTick uint64

// Only the visible pane has a timer. Its ID rejects messages already queued
// when a manual refresh, tab change, or connection switch replaces the timer.
type paneRefreshTimer struct {
	id     paneRefreshTick
	due    time.Time
	cancel context.CancelFunc
}

func (t *paneRefreshTimer) stop() {
	if t.cancel != nil {
		t.cancel()
		t.cancel, t.due = nil, time.Time{}
	}
}

func (m *Model) schedulePaneTick(tab tabID, due, now time.Time) tea.Cmd {
	if m.quitting || m.activeTab != tab {
		return nil
	}
	t := &m.paneTimer
	if due.Equal(t.due) {
		return nil
	}
	t.stop()
	if due.IsZero() {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.id++
	t.due, t.cancel = due, cancel
	id, delay := t.id, max(0, due.Sub(now))
	return func() tea.Msg {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			return id
		}
	}
}

func (m *Model) onPaneRefreshTick(id paneRefreshTick, now time.Time) tea.Cmd {
	if m.quitting || m.paneTimer.cancel == nil || m.paneTimer.id != id {
		return nil
	}
	m.paneTimer.stop()
	switch m.activeTab {
	case alertsTab:
		return m.onAlertsTick(now)
	case rulesTab:
		return m.onRulesTick(now)
	case targetsTab:
		return m.onTargetsTick(now)
	case silencesTab:
		return m.onSilencesTick(now)
	case routingTab:
		return m.onRoutingTick(now)
	}
	return nil
}
