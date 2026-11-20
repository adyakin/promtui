package app

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	"promtui/internal/config"
	"promtui/internal/history"
	"strings"
	"testing"
)

func TestCancelIgnoresLateCompletionAndKeepsResults(t *testing.T) {
	m := New(Options{}, nil)
	m.summary = "previous result"
	ctx, cancel := context.WithCancel(t.Context())
	m.cancel = cancel
	id := m.id
	m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if ctx.Err() == nil {
		t.Fatal("request still active")
	}
	m.Update(completed{id: id})
	if m.summary != "previous result" || m.status != "Cancelled" {
		t.Fatal(m.View())
	}
	m.Update(completed{id: m.id, err: errors.New("bad expression")})
	if m.summary != "previous result" || !strings.Contains(m.status, "bad expression") {
		t.Fatal(m.View())
	}
}
func TestRecallAcrossConnectionsRestoresDraft(t *testing.T) {
	m := New(Options{Connection: config.Connection{Name: "A"}}, []history.Entry{{Connection: "B", Query: "up"}, {Connection: "A", Query: "sum(\nup)"}})
	m.editor.SetValue("unfinished")
	m.recall(1)
	if m.editor.Value() != "up" {
		t.Fatal(m.editor.Value())
	}
	m.recall(1)
	if m.editor.Value() != "sum(\nup)" {
		t.Fatal(m.editor.Value())
	}
	m.recall(-1)
	if m.editor.Value() != "up" {
		t.Fatal(m.editor.Value())
	}
	m.recall(-1)
	if m.editor.Value() != "unfinished" {
		t.Fatal(m.editor.Value())
	}
}

func TestQuitFlushesCoalescedHistory(t *testing.T) {
	m := New(Options{HistoryLimit: 2, HistoryPath: t.TempDir() + "/history.jsonl"}, nil)
	m.entries = []history.Entry{{Query: "a"}}
	first := m.save()
	m.entries = history.Add(m.entries, history.Entry{Query: "b"}, 2)
	if m.save() != nil {
		t.Fatal("concurrent snapshot writes")
	}
	_, quit := m.Update(tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl})
	if quit != nil {
		t.Fatal("quit before save")
	}
	_, second := m.Update(first())
	if second == nil {
		t.Fatal("pending snapshot dropped")
	}
	_, quit = m.Update(second())
	if quit == nil {
		t.Fatal("did not quit")
	}
	got, err := history.Load(m.opt.HistoryPath, 2)
	if err != nil || len(got) != 2 || got[0].Query != "b" {
		t.Fatal(got, err)
	}
}
