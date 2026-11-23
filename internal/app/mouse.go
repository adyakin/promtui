package app

import (
	"image"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const doubleClickInterval = 350 * time.Millisecond
const tableDataStart = 3 // Body starts with two summary rows and a column header.

type mouseID struct {
	list, item string
}

type mouseClick struct {
	id mouseID
	at time.Time
}

type mouseHit struct {
	box              image.Rectangle
	id               mouseID
	selectItem, open func() tea.Cmd
	wheel            func(string) tea.Cmd
}

type mouseState struct {
	hits []mouseHit // Built by the same render pass that positions the controls.
	last mouseClick
}

func (m *Model) mouseModal() {
	clear(m.mouse.hits)
	m.mouse.hits = m.mouse.hits[:0]
}

func (m *Model) mouseDialog(x, y, width, rows int, wheel func(string) tea.Cmd) {
	m.mouseModal()
	m.mouseRegion(image.Rect(x+1, y+1, x+width-1, y+1+rows), wheel)
}

func (m *Model) mouseTable(list string, first, end int, id func(int) string, selectRow func(int), open bool, wheel func(string) tea.Cmd) {
	const bodyY = 2 // Screen header and frame title.
	m.mouseRegion(image.Rect(1, 2, m.width-1, m.height-3), wheel)
	var activate func() tea.Cmd
	if open {
		activate = func() tea.Cmd { return m.paneKey("enter") }
	}
	m.mouseRows(1, bodyY+tableDataStart, m.width-2, first, end, list, id, func(i int) tea.Cmd {
		selectRow(i)
		return nil
	}, activate)
}

func (m *Model) mousePaneEnter() tea.Cmd { return m.paneKey("enter") }

// withoutOpen leaves selection available while a text filter owns Enter.
func withoutOpen(blocked bool, open func() tea.Cmd) func() tea.Cmd {
	if blocked {
		return nil
	}
	return open
}

func (m *Model) mouseRegion(box image.Rectangle, wheel func(string) tea.Cmd) {
	if m.opt.Mouse && wheel != nil && !box.Empty() {
		m.mouse.hits = append(m.mouse.hits, mouseHit{box: box, wheel: wheel})
	}
}

func (m *Model) mouseItem(box image.Rectangle, id mouseID, selectItem, open func() tea.Cmd) {
	if m.opt.Mouse && !box.Empty() {
		m.mouse.hits = append(m.mouse.hits, mouseHit{box: box, id: id, selectItem: selectItem, open: open})
	}
}

// Register visible list entries, not headers, padding rows or borders.
func (m *Model) mouseRows(x, y, width, first, end int, list string, id func(int) string, selectRow func(int) tea.Cmd, open func() tea.Cmd) {
	if !m.opt.Mouse {
		return
	}
	for i := first; i < end; i++ {
		line := y + i - first
		m.mouseItem(image.Rect(x, line, x+width, line+1), mouseID{list, id(i)}, func() tea.Cmd { return selectRow(i) }, open)
	}
}

func mouseKey(key string) tea.KeyPressMsg {
	switch key {
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	default:
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	}
}

func (m *Model) paneKey(key string) tea.Cmd {
	cmd, _ := m.shellKey(mouseKey(key))
	return cmd
}

func (m *Model) promQLMouseKey(key string) tea.Cmd {
	cmd, _ := m.key(mouseKey(key))
	return cmd
}

func (m *Model) mouseLinks(x, y int, rows []string) {
	if !m.opt.Mouse {
		return
	}
	// Use the OSC 8 spans emitted by the detail renderer, including its
	// close/reopen pairs at wrap boundaries. Match terminal cell widths.
	const prefix = "\x1b]8;"
	for i, row := range rows {
		col := x
		for {
			before, rest, ok := strings.Cut(row, prefix)
			if !ok {
				break
			}
			col += ansi.StringWidth(before)
			params, tail, ok := strings.Cut(rest, "\x07")
			if !ok {
				break
			}
			_, dest, _ := strings.Cut(params, ";")
			text, _, _ := strings.Cut(tail, prefix)
			if dest != "" {
				width := ansi.StringWidth(text)
				m.mouseItem(image.Rect(col, y+i, col+width, y+i+1), mouseID{}, func() tea.Cmd { return m.copyDetailURL(dest) }, nil)
			}
			row = tail
		}
	}
}

func (m *Model) handleMouse(msg tea.MouseMsg, now time.Time) tea.Cmd {
	if !m.opt.Mouse || m.quitting || m.activeTab != promQLTab && (m.chart.displayed.cols != 0 || m.chart.sending) {
		return nil
	}
	p := msg.Mouse()
	point := image.Pt(p.X, p.Y)
	if !point.In(m.chart.plot) {
		m.chart.hover = time.Time{}
	}
	switch msg.(type) {
	case tea.MouseMotionMsg:
		if m.canInspectGraph() && point.In(m.chart.plot) && m.chart.displayed == m.graphPlace() {
			m.chart.hover = inspectionTime(m.chart.result, p.X-m.chart.plot.Min.X, m.chart.plot.Dx())
		}
	case tea.MouseWheelMsg:
		m.mouse.last = mouseClick{}
		if p.Mod != 0 || p.Button != tea.MouseWheelUp && p.Button != tea.MouseWheelDown {
			return nil
		}
		key := "up"
		if p.Button == tea.MouseWheelDown {
			key = "down"
		}
		for i := len(m.mouse.hits) - 1; i >= 0; i-- {
			hit := m.mouse.hits[i]
			if hit.wheel != nil && point.In(hit.box) {
				return hit.wheel(key)
			}
		}
	case tea.MouseClickMsg:
		last := m.mouse.last
		m.mouse.last = mouseClick{}
		// The terminal owns modified clicks (including native link opening).
		if p.Button != tea.MouseLeft || p.Mod != 0 {
			return nil
		}
		for i := len(m.mouse.hits) - 1; i >= 0; i-- {
			hit := m.mouse.hits[i]
			if hit.selectItem == nil || !point.In(hit.box) {
				continue
			}
			cmd := hit.selectItem()
			if hit.open == nil {
				return cmd
			}
			elapsed := now.Sub(last.at)
			if hit.id == last.id && elapsed >= 0 && elapsed <= doubleClickInterval {
				return tea.Batch(cmd, hit.open())
			}
			m.mouse.last = mouseClick{hit.id, now}
			return cmd
		}
	}
	return nil
}
