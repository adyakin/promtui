package app

import (
	tea "charm.land/bubbletea/v2"
	"promtui/internal/programstatus"
)

type statusPacket string

type terminalStatus struct {
	pending string
	sending bool
	used    bool
}

func (m *Model) reportProgramStatus(state programstatus.State, message string) {
	if m.opt.ProgramStatus {
		m.programStatus.pending += programstatus.Report(state, message)
	}
}

// Tea handles RawMsg before passing it to Update. Keep just one packet in
// flight so concurrent commands cannot reorder query status transitions.
func (m *Model) flushProgramStatus(msg tea.Msg) tea.Cmd {
	s := &m.programStatus
	if raw, ok := msg.(tea.RawMsg); ok {
		if _, ok := raw.Msg.(statusPacket); ok {
			s.sending = false
			s.used = true
		}
	}
	if s.sending || s.pending == "" {
		return nil
	}
	packet := statusPacket(s.pending)
	s.pending, s.sending = "", true
	return tea.Raw(packet)
}

// CloseTerminal is called after Tea stops rendering, including on signal exit.
func (m *Model) CloseTerminal() string {
	m.stop()
	cleanup := m.CloseGraphics()
	if m.programStatus.used {
		cleanup += programstatus.Clear()
	}
	return cleanup
}
