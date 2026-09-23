package editor

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
)

// textWidth leaves one spare column so the cursor fits after a full segment.
func (e *Editor) textWidth() int { return max(1, e.width-1) }

// cellWidth is the width of r when drawn at visual column x.
func (e *Editor) cellWidth(r rune, x int) int {
	if r == '\t' {
		return e.TabWidth - x%e.TabWidth
	}
	if w := runewidth.RuneWidth(r); w > 0 {
		return w
	}
	if r < ' ' {
		return 1 // control characters are drawn as a placeholder
	}
	return 0
}

// hang is how far wrapped continuation lines of row are indented: the width
// of the line's leading whitespace, capped so some text always fits.
func (e *Editor) hang(row int) int {
	x := 0
	for _, r := range leadingWhitespace(e.lines[row]) {
		x += e.cellWidth(r, x)
	}
	return min(x, e.textWidth()/2)
}

// segX is the column a visual segment's text starts at.
func (e *Editor) segX(row, k int) int {
	if k == 0 {
		return 0
	}
	return e.hang(row)
}

// wrap returns the rune index where each visual segment of line row starts.
// Lines break after whitespace when possible, otherwise mid-word.
func (e *Editor) wrap(row int) []int {
	if len(e.wraps) != len(e.lines) {
		e.wraps = make([][]int, len(e.lines))
	}
	if w := e.wraps[row]; w != nil {
		return w
	}
	line := e.lines[row]
	width := e.textWidth()
	starts := []int{0}
	hang := e.hang(row)
	segStart, x, lastBreak := 0, 0, -1
	for i := 0; i < len(line); i++ {
		w := e.cellWidth(line[i], x)
		if x+w > width && i > segStart {
			brk := i
			if lastBreak > segStart {
				brk = lastBreak
			}
			starts = append(starts, brk)
			segStart, lastBreak, x = brk, -1, hang
			for j := segStart; j < i; j++ {
				x += e.cellWidth(line[j], x)
			}
			w = e.cellWidth(line[i], x)
		}
		x += w
		if line[i] == ' ' || line[i] == '\t' {
			lastBreak = i + 1
		}
	}
	e.wraps[row] = starts
	return starts
}

func (e *Editor) segmentEnd(row, k int) int {
	starts := e.wrap(row)
	if k+1 < len(starts) {
		return starts[k+1]
	}
	return len(e.lines[row])
}

func segmentOf(starts []int, col int) int {
	k := 0
	for k+1 < len(starts) && starts[k+1] <= col {
		k++
	}
	return k
}

// VisualLines is the number of screen lines the document takes up.
func (e *Editor) VisualLines() int { return e.totalVisual() }

func (e *Editor) totalVisual() int {
	n := 0
	for r := range e.lines {
		n += len(e.wrap(r))
	}
	return n
}

// visualPos returns the visual line and column of p.
func (e *Editor) visualPos(p Pos) (int, int) {
	vl := 0
	for r := 0; r < p.Row; r++ {
		vl += len(e.wrap(r))
	}
	starts := e.wrap(p.Row)
	k := segmentOf(starts, p.Col)
	x := e.segX(p.Row, k)
	for _, r := range e.lines[p.Row][starts[k]:p.Col] {
		x += e.cellWidth(r, x)
	}
	return vl + k, x
}

// locate maps a visual line to its line and segment.
func (e *Editor) locate(vl int) (row, k int) {
	for r := range e.lines {
		n := len(e.wrap(r))
		if vl < n {
			return r, vl
		}
		vl -= n
	}
	last := len(e.lines) - 1
	return last, len(e.wrap(last)) - 1
}

// posAtVisual returns the buffer position drawn at visual line vl, column x.
func (e *Editor) posAtVisual(vl, x int) Pos {
	row, k := e.locate(max(0, vl))
	starts := e.wrap(row)
	start, end := starts[k], e.segmentEnd(row, k)
	cx := e.segX(row, k)
	for i := start; i < end; i++ {
		w := e.cellWidth(e.lines[row][i], cx)
		if x < cx+(w+1)/2 {
			return Pos{row, i}
		}
		cx += w
	}
	if k+1 < len(starts) && end > start {
		return Pos{row, end - 1} // stay on this visual line
	}
	return Pos{row, end}
}

// EnsureVisible scrolls so the cursor is on screen.
func (e *Editor) EnsureVisible() {
	vl, _ := e.visualPos(e.cur)
	if vl < e.scroll {
		e.scroll = vl
	}
	if vl >= e.scroll+e.height {
		e.scroll = vl - e.height + 1
	}
	e.clampScroll()
}

func (e *Editor) clampScroll() {
	e.scroll = max(0, min(e.scroll, e.totalVisual()-e.height))
}

// ScrollBy scrolls the view without moving the cursor.
func (e *Editor) ScrollBy(n int) {
	e.scroll += n
	e.clampScroll()
}

// CursorView returns the cursor's cell inside the visible area, and whether
// it is visible at all.
func (e *Editor) CursorView() (x, y int, ok bool) {
	vl, x := e.visualPos(e.cur)
	y = vl - e.scroll
	return x, y, y >= 0 && y < e.height
}

// --- mouse -----------------------------------------------------------------

// PosAt maps a cell in the visible area to a buffer position.
func (e *Editor) PosAt(x, y int) Pos {
	vl := e.scroll + y
	if vl < 0 {
		return Pos{}
	}
	if vl >= e.totalVisual() {
		last := len(e.lines) - 1
		return Pos{last, len(e.lines[last])}
	}
	return e.posAtVisual(vl, max(0, x))
}

// Click places the cursor at a cell. With extend, it extends the selection.
func (e *Editor) Click(x, y int, extend bool) {
	p := e.PosAt(x, y)
	if extend {
		e.startMove(true)
	} else {
		e.lastKind = editNone
		e.anchor = p
		e.sel = true
	}
	e.cur = p
	e.goalX = -1
}

// Drag extends the selection to a cell, scrolling when dragged past an edge.
func (e *Editor) Drag(x, y int) {
	if y < 0 {
		e.ScrollBy(-1)
		y = 0
	} else if y >= e.height {
		e.ScrollBy(1)
		y = e.height - 1
	}
	if !e.sel {
		e.anchor = e.cur
		e.sel = true
	}
	e.cur = e.PosAt(x, y)
	e.goalX = -1
}

// SelectWordAt selects the word (or run of non-word runes) at a cell.
func (e *Editor) SelectWordAt(x, y int) {
	p := e.PosAt(x, y)
	line := e.lines[p.Row]
	if len(line) == 0 {
		e.MoveTo(p, false)
		return
	}
	i := min(p.Col, len(line)-1)
	same := func(r rune) bool { return isWord(r) == isWord(line[i]) && (r == ' ') == (line[i] == ' ') }
	a, b := i, i+1
	for a > 0 && same(line[a-1]) {
		a--
	}
	for b < len(line) && same(line[b]) {
		b++
	}
	e.anchor, e.cur, e.sel = Pos{p.Row, a}, Pos{p.Row, b}, true
	e.goalX = -1
}

// SelectLineAt selects the whole line at a cell, including its newline.
func (e *Editor) SelectLineAt(x, y int) {
	p := e.PosAt(x, y)
	e.anchor, e.sel = Pos{p.Row, 0}, true
	if p.Row+1 < len(e.lines) {
		e.cur = Pos{p.Row + 1, 0}
	} else {
		e.cur = Pos{p.Row, len(e.lines[p.Row])}
	}
	e.goalX = -1
}

// --- rendering -------------------------------------------------------------

// Styles controls how the editor draws.
type Styles struct {
	Text      lipgloss.Style
	Selection lipgloss.Style
	Guide     lipgloss.Style // indentation guides on leading tabs
	Guides    bool
}

type runKind int

const (
	runText runKind = iota
	runSel
	runGuide
)

// View draws the visible area: exactly height lines, each width cells wide.
func (e *Editor) View(st Styles) string {
	var out strings.Builder
	selA, selB, hasSel := e.Selection()
	row, k := e.locate(e.scroll)
	if e.scroll >= e.totalVisual() {
		row = len(e.lines) // nothing to draw
	}

	var run strings.Builder
	kind := runText
	flush := func() {
		if run.Len() == 0 {
			return
		}
		s := run.String()
		switch kind {
		case runSel:
			out.WriteString(st.Selection.Render(s))
		case runGuide:
			out.WriteString(st.Guide.Render(s))
		default:
			out.WriteString(st.Text.Render(s))
		}
		run.Reset()
	}
	emit := func(k runKind, s string) {
		if k != kind {
			flush()
			kind = k
		}
		run.WriteString(s)
	}
	selected := func(p Pos) bool {
		return hasSel && !p.Before(selA) && p.Before(selB)
	}

	for y := 0; y < e.height; y++ {
		if y > 0 {
			out.WriteByte('\n')
		}
		if row >= len(e.lines) {
			out.WriteString(strings.Repeat(" ", e.width))
			continue
		}
		line := e.lines[row]
		starts := e.wrap(row)
		start, end := starts[k], e.segmentEnd(row, k)
		lead := leadingWhitespace(line)
		indent := len(lead)
		x := 0
		if k > 0 {
			// Hanging indent: repeat the line's indentation (and guides).
			hang := e.segX(row, k)
			for _, r := range lead {
				w := e.cellWidth(r, x)
				if x+w > hang {
					break
				}
				if r == '\t' && st.Guides {
					emit(runGuide, "│"+strings.Repeat(" ", w-1))
				} else {
					emit(runText, strings.Repeat(" ", w))
				}
				x += w
			}
			if x < hang {
				emit(runText, strings.Repeat(" ", hang-x))
				x = hang
			}
		}
		for i := start; i < end; i++ {
			r := line[i]
			w := e.cellWidth(r, x)
			rk := runText
			if selected(Pos{row, i}) {
				rk = runSel
			}
			switch {
			case r == '\t':
				if rk == runText && st.Guides && i < indent && k == 0 {
					emit(runGuide, "│"+strings.Repeat(" ", w-1))
				} else {
					emit(rk, strings.Repeat(" ", w))
				}
			case r < ' ' || r == 0x7f:
				emit(rk, "·")
			default:
				emit(rk, string(r))
			}
			x += w
		}
		// Show a selected newline as one highlighted cell.
		if k == len(starts)-1 && row+1 < len(e.lines) && selected(Pos{row, len(line)}) && x < e.width {
			emit(runSel, " ")
			x++
		}
		flush()
		if x < e.width {
			out.WriteString(strings.Repeat(" ", e.width-x))
		}
		kind = runText
		if k+1 < len(starts) {
			k++
		} else {
			row, k = row+1, 0
		}
	}
	return out.String()
}
