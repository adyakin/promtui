package app

import (
	"strconv"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	"promtui/internal/alertmanager"
)

type silenceClipboard struct {
	input   *textinput.Model
	content string
	err     error
}

func silenceInputMessage(msg tea.Msg, input *textinput.Model) (tea.Msg, tea.Cmd) {
	switch v := msg.(type) {
	case tea.KeyPressMsg:
		if key.Matches(v, input.KeyMap.Paste) {
			return nil, func() tea.Msg {
				value, err := clipboard.ReadAll()
				return silenceClipboard{input, value, err}
			}
		}
	case silenceClipboard:
		if v.input != input || v.err != nil {
			return nil, nil
		}
		return tea.PasteMsg{Content: v.content}, nil
	}
	return msg, nil
}

func pasteSilenceValue(m *silenceMatcherDraft, paste tea.PasteMsg) tea.Msg {
	if m.quoted {
		value := []rune(m.value.Value())
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			m.value.SetCursor(min(max(1, m.value.Position()), len(value)-1))
		}
		paste.Content = strings.TrimSuffix(strings.TrimPrefix(strconv.Quote(paste.Content), `"`), `"`)
		return paste
	}
	if strings.ContainsFunc(paste.Content, unicode.IsControl) {
		value, pos := []rune(m.value.Value()), m.value.Position()
		combined := string(value[:pos]) + paste.Content + string(value[pos:])
		m.quoted = true
		m.value.Placeholder = `quoted value, e.g. "a\nb"`
		m.value.SetValue(strconv.Quote(combined))
		m.value.SetCursor(len([]rune(strconv.Quote(string(value[:pos])+paste.Content))) - 1)
		return nil
	}
	return paste
}

func (m *Model) openSilenceEditor(source *alertmanager.Silence, labels map[string]string, fromAlert bool) tea.Cmd {
	s := &m.silences
	if m.opt.Connection.AlertmanagerURL == "" {
		return nil
	}
	s.editor = newSilenceDraft(m.opt.SilenceCreator, source, labels, fromAlert, time.Now())
	s.notice = ""
	m.sizeSilenceInputs()
	return nil // The first focused control is the Start mode button.
}

func (m *Model) silencesKey(k tea.KeyPressMsg) tea.Cmd {
	s := &m.silences
	now := time.Now()
	key := k.String()
	if s.modal() {
		return m.silenceModalKey(key, k, now)
	}
	if s.search.Focused() {
		switch key {
		case "enter":
			s.filter = s.search.Value()
			s.search.Blur()
		case "esc":
			s.search.Blur()
			s.rebuild(s.anchor, s.anchorRow, now)
		default:
			return m.editSilenceInput(k)
		}
		return nil
	}
	switch key {
	case "?":
		s.help, s.helpOffset = true, 0
	case "q":
		return m.act("quit", "", 0)
	}
	if m.opt.Connection.AlertmanagerURL == "" {
		return nil
	}
	switch key {
	case "n":
		return m.openSilenceEditor(nil, nil, false)
	case "r":
		s.notice = ""
		return m.refreshSilences()
	case "e":
		s.includeExpired = !s.includeExpired
		s.rebuild(s.selection(), s.row, now)
	case "s":
		s.sort = nextViewSort(silenceSorts, s.sort)
		s.rebuild(s.selection(), s.row, now)
	case "/":
		if !s.lastOK.IsZero() {
			s.anchor, s.anchorRow = s.selection(), s.row
			s.search.SetValue(s.filter)
			s.search.CursorEnd()
			return s.search.Focus()
		}
	case "enter":
		if s.selection() != "" {
			row := s.visible[s.row]
			s.detail = &row
			s.detailOffset, s.notice = 0, ""
			return m.fetchSilenceDetail()
		}
	default:
		s.row = targetNavigate(key, s.row, len(s.visible), m.silencePage())
	}
	return nil
}

func (m *Model) silenceModalKey(key string, k tea.KeyPressMsg, now time.Time) tea.Cmd {
	s := &m.silences
	switch {
	case s.writeCancel != nil:
		// Keep the operation bound to this draft/connection until resolved.
		return nil
	case s.confirm != noSilenceConfirm:
		switch key {
		case "esc":
			s.confirm, s.confirmErr = noSilenceConfirm, nil
		case "enter":
			if s.confirm == confirmSilenceBroad {
				return m.submitSilence(now, true)
			}
			s.confirmErr = nil
			return m.writeSilence(alertmanager.SilenceSpec{}, s.detail, true)
		default:
			m.scrollSilenceDialog(key, now, &s.confirmOffset)
		}
	case s.showMatches:
		if key == "esc" {
			s.showMatches = false
		} else {
			m.scrollSilenceDialog(key, now, &s.matchesOffset)
		}
	case s.editor != nil:
		return m.silenceEditorKey(key, k, now)
	case s.help:
		if key == "esc" || key == "?" {
			s.help = false
		} else {
			m.scrollSilenceDialog(key, now, &s.helpOffset)
		}
	case s.detail != nil:
		switch key {
		case "esc":
			s.closeDetail()
		case "a":
			s.showMatches, s.matchesOffset = true, 0
			if s.detailMatches.err != nil && s.detailCancel == nil {
				return m.fetchSilenceMatches(false, s.detail.Matchers)
			}
		case "e":
			if s.detailCancel == nil && s.detailErr == nil {
				return m.openSilenceEditor(s.detail, nil, false)
			}
		case "x":
			if s.detailCancel == nil && s.detailErr == nil && s.detail.State(now) != "expired" {
				s.confirm, s.confirmOffset, s.confirmErr = confirmSilenceExpire, 0, nil
			}
		default:
			m.scrollSilenceDialog(key, now, &s.detailOffset)
		}
	}
	return nil
}

func (m *Model) silenceEditorKey(key string, k tea.KeyPressMsg, now time.Time) tea.Cmd {
	if k.Mod == tea.ModAlt && (k.Code == 'c' || k.Code >= '0' && k.Code <= '3') {
		return nil // These shell shortcuts are disabled while a draft is open.
	}
	s, d := &m.silences, m.silences.editor
	g := m.silenceEditorGrid()
	field := g.field(d.focus)
	switch key {
	case "esc":
		s.closeEditor()
	case "tab", "shift+tab":
		delta := 1
		if key == "shift+tab" {
			delta = -1
		}
		return m.focusSilenceField((d.focus + g.count + delta) % g.count)
	case "enter":
		if field.input == nil {
			switch field.action {
			case silenceToggleStart:
				d.scheduled = !d.scheduled
			case silenceToggleEnd:
				d.until = !d.until
			case silenceCycleOp:
				row := &d.matchers[field.matcher]
				row.operator = (row.operator + 1) % len(silenceOperators)
				d.preview.reset()
			case silenceRemoveMatcher:
				d.matchers = append(d.matchers[:field.matcher], d.matchers[field.matcher+1:]...)
				d.preview.reset()
				d.focus = min(d.focus-3, m.silenceEditorGrid().count-1)
			case silenceAddMatcher:
				d.matchers = append(d.matchers, newSilenceMatcher(alertmanager.Matcher{IsEqual: true}))
				d.preview.reset()
			}
			if field.action != silencePreviewAction {
				d.err = nil
				return m.focusSilenceField(d.focus)
			}
			return m.previewSilence()
		}
	case "p":
		if field.input != nil {
			return m.editSilenceInput(k)
		}
		return m.previewSilence()
	case "ctrl+p":
		return m.previewSilence()
	case "ctrl+enter", "ctrl+g":
		return m.submitSilence(now, false)
	default:
		return m.editSilenceInput(k)
	}
	return nil
}

func (m *Model) previewSilence() tea.Cmd {
	s := &m.silences
	s.showMatches, s.matchesOffset = true, 0
	matchers, err := s.editor.matcherValues()
	if err != nil {
		s.editor.preview.reset()
		s.editor.preview.err = err
		return nil
	}
	return m.fetchSilenceMatches(true, matchers)
}

func (m *Model) submitSilence(now time.Time, confirmed bool) tea.Cmd {
	s, d := &m.silences, m.silences.editor
	spec, broad, err := d.spec(now)
	d.err = err
	if err != nil {
		s.confirm = noSilenceConfirm
		return nil
	}
	if broad && !confirmed {
		s.confirm, s.confirmOffset = confirmSilenceBroad, 0
		return nil
	}
	s.confirm = noSilenceConfirm
	return m.writeSilence(spec, d.original, false)
}

func (m *Model) editSilenceInput(msg tea.Msg) tea.Cmd {
	s := &m.silences
	if s.editor != nil {
		if s.writeCancel != nil || s.confirm != noSilenceConfirm || s.showMatches {
			return nil
		}
		d := s.editor
		field := m.silenceEditorGrid().field(d.focus)
		if field.input == nil {
			return nil
		}
		before := field.input.Value()
		msg, cmd := silenceInputMessage(msg, field.input)
		if paste, ok := msg.(tea.PasteMsg); ok && field.matcher >= 0 && field.input == &d.matchers[field.matcher].value {
			msg = pasteSilenceValue(&d.matchers[field.matcher], paste)
		}
		if msg != nil {
			*field.input, cmd = field.input.Update(msg)
		}
		if before != field.input.Value() {
			d.err = nil
			if field.matcher >= 0 {
				d.preview.reset()
			}
		}
		return cmd
	}
	if m.activeTab == silencesTab && s.search.Focused() && !s.modal() {
		msg, cmd := silenceInputMessage(msg, &s.search)
		if msg != nil {
			s.search, cmd = s.search.Update(msg)
			s.rebuild(s.selection(), s.row, time.Now())
		}
		return cmd
	}
	return nil
}
