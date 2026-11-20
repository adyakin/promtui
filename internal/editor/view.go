package editor

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/prometheus/prometheus/promql/parser"
)

func Position(e textarea.Model) int {
	lines := strings.Split(e.Value(), "\n")
	n := 0
	for _, l := range lines[:e.Line()] {
		n += len(l) + 1
	}
	return n + len(string([]rune(lines[e.Line()])[:e.Column()]))
}
func styles(s string, pos int) map[int]int {
	out := map[int]int{}
	stack := []Token{}
	pairs := map[int]int{}
	for _, t := range scan(s) {
		v := t.Text
		color := 7
		switch {
		case v[0] == '#':
			color = 8
		case quoted(v):
			color = 2
		case parser.Functions[v] != nil:
			color = 6
		case slices.Contains(words, v):
			color = 5
		case v[0] >= '0' && v[0] <= '9':
			color = 3
		case strings.Contains("([{", v):
			stack = append(stack, t)
		case strings.Contains(")]}", v) && len(stack) > 0:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if strings.Index("([{", a.Text) == strings.Index(")]}", v) {
				pairs[t.Start] = a.Start
				pairs[a.Start] = t.Start
			}
		case !ident(v):
			color = 4
		}
		for i := t.Start; i < t.End; i++ {
			out[i] = color
		}
	}
	for _, p := range []int{pos, pos - 1} {
		if other, ok := pairs[p]; ok {
			out[p], out[other] = 14, 14
			break
		}
	}
	return out
}

// The textarea handles editing; this viewport paints real source offsets, so
// ANSI sequences never enter the buffer or interfere with cursor movement.
func View(e textarea.Model, width int) (rows []string, cursorX, cursorY int) {
	s := e.Value()
	lines := strings.Split(s, "\n")
	pos := Position(e)
	colors := styles(s, pos)
	start := max(0, e.Line()-e.Height()+1)
	off := 0
	for _, l := range lines[:start] {
		off += len(l) + 1
	}
	for i := start; i < min(len(lines), start+e.Height()); i++ {
		prefix := fmt.Sprintf("%3d │ ", i+1)
		room := max(1, width-ansi.StringWidth(prefix))
		var b strings.Builder
		left := 0
		if i == e.Line() {
			col := ansi.StringWidth(lines[i][:pos-off])
			left = max(0, col-room+2)
			cursorX, cursorY = ansi.StringWidth(prefix)+col-left, i-start
		}
		last := -1
		for j, r := range lines[i] + " " {
			style := colors[off+j]
			if style == 0 {
				style = 7
			}
			if style != last {
				base := 30
				if style >= 8 {
					base = 90
				}
				fmt.Fprintf(&b, "\x1b[0;%dm", base+style%8)
				last = style
			}
			if unicode.IsControl(r) {
				r = ' '
			}
			b.WriteRune(r)
		}
		b.WriteString("\x1b[0m")
		line := ansi.Cut(b.String(), left, left+room)
		if s == "" {
			line += " " + lipgloss.NewStyle().Faint(true).Render("Enter PromQL")
		}
		rows = append(rows, ansi.Truncate(prefix+line, width, ""))
		off += len(lines[i]) + 1
	}
	return
}
