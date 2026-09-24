// Package editor is a soft-wrapping multi-line text editor widget with
// keyboard and mouse support, selection and undo.
package editor

import (
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// Pos is a position in the buffer: a line index and a rune index in it.
type Pos struct{ Row, Col int }

// Before reports whether p comes before q.
func (p Pos) Before(q Pos) bool {
	return p.Row < q.Row || (p.Row == q.Row && p.Col < q.Col)
}

type editKind int

const (
	editNone editKind = iota
	editInsert
	editDelete
	editOther
)

// snapshot shares line slices with the live document. That is safe because
// edits never modify a line slice in place: they build a new one.
type snapshot struct {
	lines [][]rune
	cur   Pos
}

const (
	maxUndo      = 500
	undoCoalesce = time.Second
	defaultTabW  = 4
)

// Editor holds one document. The zero value is not usable; call New.
type Editor struct {
	lines  [][]rune
	cur    Pos
	anchor Pos
	sel    bool // anchor is active
	goalX  int  // preferred visual column for vertical moves, -1 if none

	width, height int
	scroll        int // first visible visual line
	TabWidth      int
	SingleLine    bool // newlines are replaced by spaces

	wraps [][]int // per line: rune index where each visual segment starts

	undo, redo []snapshot
	lastKind   editKind
	lastEdit   time.Time

	version uint64
}

// New returns an empty editor.
func New() *Editor {
	return &Editor{lines: [][]rune{{}}, goalX: -1, TabWidth: defaultTabW, width: 80, height: 24}
}

// SetText replaces the whole document and clears history.
func (e *Editor) SetText(s string) {
	e.lines = splitLines(s)
	e.wraps = nil
	e.undo, e.redo = nil, nil
	e.lastKind = editNone
	e.sel = false
	e.SetCursor(e.cur)
	e.version++
}

// ReplaceText replaces the document as an undoable edit, keeping the cursor
// as close to where it was as possible.
func (e *Editor) ReplaceText(s string) {
	if s == e.Text() {
		return
	}
	e.pushUndo(editOther)
	e.lines = splitLines(s)
	e.wraps = nil
	e.sel = false
	e.SetCursor(e.cur)
	e.changed()
}

// ReplaceRange replaces the text from a to b, which must be on one line, with
// s as its own undo step, and leaves the cursor after the new text.
func (e *Editor) ReplaceRange(a, b Pos, s string) {
	a, b = e.clamp(a), e.clamp(b)
	if a.Row != b.Row || b.Col < a.Col {
		return
	}
	e.pushUndo(editOther)
	line := e.lines[a.Row]
	r := []rune(strings.ReplaceAll(s, "\n", " "))
	nl := make([]rune, 0, len(line)-(b.Col-a.Col)+len(r))
	nl = append(append(append(nl, line[:a.Col]...), r...), line[b.Col:]...)
	e.lines[a.Row] = nl
	e.invalidate(a.Row)
	e.sel = false
	e.SetCursor(Pos{a.Row, a.Col + len(r)})
	e.changed()
}

// Text returns the document.
func (e *Editor) Text() string {
	var b strings.Builder
	for i, l := range e.lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(string(l))
	}
	return b.String()
}

// Version changes whenever the text changes.
func (e *Editor) Version() uint64 { return e.version }

// Cursor returns the cursor position.
func (e *Editor) Cursor() Pos { return e.cur }

// Scroll returns the first visible visual line.
func (e *Editor) Scroll() int { return e.scroll }

// SetCursor moves the cursor, clamped to the document, and scrolls to it.
func (e *Editor) SetCursor(p Pos) {
	e.cur = e.clamp(p)
	e.goalX = -1
	e.EnsureVisible()
}

// SetScroll sets the first visible visual line, clamped.
func (e *Editor) SetScroll(n int) {
	e.scroll = n
	e.clampScroll()
}

// SetSize sets the visible area in cells.
func (e *Editor) SetSize(w, h int) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	if w != e.width {
		e.wraps = nil
	}
	e.width, e.height = w, h
	e.clampScroll()
}

// Size returns the visible area in cells.
func (e *Editor) Size() (int, int) { return e.width, e.height }

func splitLines(s string) [][]rune {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	parts := strings.Split(s, "\n")
	lines := make([][]rune, len(parts))
	for i, p := range parts {
		lines[i] = []rune(p)
	}
	return lines
}

func (e *Editor) clamp(p Pos) Pos {
	p.Row = max(0, min(p.Row, len(e.lines)-1))
	p.Col = max(0, min(p.Col, len(e.lines[p.Row])))
	return p
}

func (e *Editor) changed() {
	e.version++
	e.EnsureVisible()
}

func (e *Editor) invalidate(row int) {
	if row < len(e.wraps) {
		e.wraps[row] = nil
	}
}

// structural marks every wrap as stale, for wholesale document changes.
func (e *Editor) structural() { e.wraps = nil }

// splice keeps the wrap cache aligned after lines [at, at+removed) were
// replaced by inserted new lines; only those lines get rewrapped.
func (e *Editor) splice(at, removed, inserted int) {
	if e.wraps == nil || len(e.wraps) != len(e.lines)-inserted+removed {
		e.wraps = nil
		return
	}
	fresh := make([][]int, inserted)
	e.wraps = append(e.wraps[:at], append(fresh, e.wraps[at+removed:]...)...)
}

// --- selection -------------------------------------------------------------

// HasSelection reports whether a non-empty range is selected.
func (e *Editor) HasSelection() bool { return e.sel && e.anchor != e.cur }

// Selection returns the ordered selection bounds.
func (e *Editor) Selection() (Pos, Pos, bool) {
	if !e.HasSelection() {
		return e.cur, e.cur, false
	}
	if e.anchor.Before(e.cur) {
		return e.anchor, e.cur, true
	}
	return e.cur, e.anchor, true
}

// SelectedText returns the selected text, or "" if nothing is selected.
func (e *Editor) SelectedText() string {
	a, b, ok := e.Selection()
	if !ok {
		return ""
	}
	if a.Row == b.Row {
		return string(e.lines[a.Row][a.Col:b.Col])
	}
	var s strings.Builder
	s.WriteString(string(e.lines[a.Row][a.Col:]))
	for r := a.Row + 1; r < b.Row; r++ {
		s.WriteByte('\n')
		s.WriteString(string(e.lines[r]))
	}
	s.WriteByte('\n')
	s.WriteString(string(e.lines[b.Row][:b.Col]))
	return s.String()
}

// SelectAll selects the whole document.
func (e *Editor) SelectAll() {
	e.anchor = Pos{}
	e.sel = true
	last := len(e.lines) - 1
	e.cur = Pos{last, len(e.lines[last])}
	e.EnsureVisible()
}

// ClearSelection drops the selection without moving the cursor.
func (e *Editor) ClearSelection() { e.sel = false }

func (e *Editor) deleteSelection() bool {
	a, b, ok := e.Selection()
	if !ok {
		e.sel = false
		return false
	}
	head := e.lines[a.Row][:a.Col]
	tail := e.lines[b.Row][b.Col:]
	merged := make([]rune, 0, len(head)+len(tail))
	merged = append(append(merged, head...), tail...)
	e.lines = append(e.lines[:a.Row+1], e.lines[b.Row+1:]...)
	e.lines[a.Row] = merged
	e.cur = a
	e.sel = false
	e.splice(a.Row, b.Row-a.Row+1, 1)
	return true
}

// --- editing ---------------------------------------------------------------

func (e *Editor) pushUndo(kind editKind) {
	now := time.Now()
	coalesce := kind != editOther && kind == e.lastKind && now.Sub(e.lastEdit) < undoCoalesce
	if !coalesce {
		e.undo = append(e.undo, e.current())
		if len(e.undo) > maxUndo {
			e.undo = e.undo[1:]
		}
	}
	e.redo = nil
	e.lastKind = kind
	e.lastEdit = now
}

func (e *Editor) restore(s snapshot) {
	e.lines = append([][]rune(nil), s.lines...)
	e.structural()
	e.sel = false
	e.cur = e.clamp(s.cur)
	e.goalX = -1
	e.lastKind = editNone
	e.changed()
}

func (e *Editor) current() snapshot {
	return snapshot{append([][]rune(nil), e.lines...), e.cur}
}

// without returns a copy of l with l[i:j] removed.
func without(l []rune, i, j int) []rune {
	out := make([]rune, 0, len(l)-(j-i))
	return append(append(out, l[:i]...), l[j:]...)
}

// Undo reverts the last edit group.
func (e *Editor) Undo() {
	if len(e.undo) == 0 {
		return
	}
	e.redo = append(e.redo, e.current())
	s := e.undo[len(e.undo)-1]
	e.undo = e.undo[:len(e.undo)-1]
	e.restore(s)
}

// Redo re-applies an undone edit group.
func (e *Editor) Redo() {
	if len(e.redo) == 0 {
		return
	}
	e.undo = append(e.undo, e.current())
	s := e.redo[len(e.redo)-1]
	e.redo = e.redo[:len(e.redo)-1]
	e.restore(s)
}

// InsertText inserts s at the cursor, replacing any selection.
func (e *Editor) InsertText(s string) {
	if s == "" {
		return
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	if e.SingleLine {
		s = strings.ReplaceAll(s, "\n", " ")
	}
	paste := len([]rune(s)) > 1
	if paste || strings.ContainsAny(s, " \t\n") || e.HasSelection() {
		e.lastKind = editNone // whitespace and pastes start a new undo group
	}
	e.pushUndo(editInsert)
	if paste {
		e.lastKind = editOther // and so does whatever follows a paste
	}
	e.deleteSelection()

	parts := strings.Split(s, "\n")
	line := e.lines[e.cur.Row]
	tail := append([]rune(nil), line[e.cur.Col:]...)
	first := []rune(parts[0])
	newLine := append(line[:e.cur.Col:e.cur.Col], first...)
	if len(parts) == 1 {
		e.lines[e.cur.Row] = append(newLine, tail...) // newLine is a fresh slice
		e.cur.Col += len(first)
		e.invalidate(e.cur.Row)
	} else {
		e.lines[e.cur.Row] = newLine
		extra := make([][]rune, 0, len(parts)-1)
		for _, p := range parts[1:] {
			extra = append(extra, []rune(p))
		}
		lastLen := len(extra[len(extra)-1])
		extra[len(extra)-1] = append(extra[len(extra)-1], tail...)
		e.lines = append(e.lines[:e.cur.Row+1], append(extra, e.lines[e.cur.Row+1:]...)...)
		e.splice(e.cur.Row, 1, 1+len(extra))
		e.cur = Pos{e.cur.Row + len(extra), lastLen}
	}
	e.goalX = -1
	e.changed()
}

// Newline splits the line at the cursor, carrying over its indentation.
func (e *Editor) Newline() {
	if e.SingleLine {
		return
	}
	indent := leadingWhitespace(e.lines[e.cur.Row])
	if e.HasSelection() {
		a, _, _ := e.Selection()
		indent = leadingWhitespace(e.lines[a.Row])
	}
	if len(indent) > e.cur.Col && !e.HasSelection() {
		indent = indent[:e.cur.Col]
	}
	e.InsertText("\n" + string(indent))
}

func leadingWhitespace(l []rune) []rune {
	i := 0
	for i < len(l) && (l[i] == ' ' || l[i] == '\t') {
		i++
	}
	return l[:i]
}

// Backspace deletes the selection or the rune before the cursor.
func (e *Editor) Backspace() {
	if e.HasSelection() {
		e.pushUndo(editOther)
		e.deleteSelection()
		e.changed()
		return
	}
	if e.cur.Col == 0 && e.cur.Row == 0 {
		return
	}
	e.pushUndo(editDelete)
	e.sel = false
	if e.cur.Col > 0 {
		e.lines[e.cur.Row] = without(e.lines[e.cur.Row], e.cur.Col-1, e.cur.Col)
		e.cur.Col--
		e.invalidate(e.cur.Row)
	} else {
		prev := e.lines[e.cur.Row-1]
		col := len(prev)
		e.lines[e.cur.Row-1] = append(prev[:col:col], e.lines[e.cur.Row]...)
		e.lines = append(e.lines[:e.cur.Row], e.lines[e.cur.Row+1:]...)
		e.splice(e.cur.Row-1, 2, 1)
		e.cur = Pos{e.cur.Row - 1, col}
	}
	e.goalX = -1
	e.changed()
}

// Delete deletes the selection or the rune after the cursor.
func (e *Editor) Delete() {
	if e.HasSelection() {
		e.pushUndo(editOther)
		e.deleteSelection()
		e.changed()
		return
	}
	l := e.lines[e.cur.Row]
	if e.cur.Col == len(l) && e.cur.Row == len(e.lines)-1 {
		return
	}
	e.pushUndo(editDelete)
	e.sel = false
	if e.cur.Col < len(l) {
		e.lines[e.cur.Row] = without(l, e.cur.Col, e.cur.Col+1)
		e.invalidate(e.cur.Row)
	} else {
		e.lines[e.cur.Row] = append(l[:len(l):len(l)], e.lines[e.cur.Row+1]...)
		e.lines = append(e.lines[:e.cur.Row+1], e.lines[e.cur.Row+2:]...)
		e.splice(e.cur.Row, 2, 1)
	}
	e.goalX = -1
	e.changed()
}

// DeleteWordBack deletes back to the previous word start.
func (e *Editor) DeleteWordBack() {
	if !e.HasSelection() {
		e.anchor = e.cur
		e.sel = true
		e.cur = e.wordLeft(e.cur)
	}
	e.Backspace()
}

// DeleteWordForward deletes forward to the next word end.
func (e *Editor) DeleteWordForward() {
	if !e.HasSelection() {
		e.anchor = e.cur
		e.sel = true
		e.cur = e.wordRight(e.cur)
	}
	e.Delete()
}

// Indent inserts a tab. With a selection spanning several lines it indents
// every selected line instead.
func (e *Editor) Indent() {
	a, b, ok := e.Selection()
	if !ok || a.Row == b.Row {
		e.InsertText("\t")
		return
	}
	e.pushUndo(editOther)
	last := b.Row
	if b.Col == 0 {
		last-- // a selection ending at column 0 doesn't include that line
	}
	for r := a.Row; r <= last; r++ {
		if len(e.lines[r]) == 0 {
			continue
		}
		e.lines[r] = append([]rune{'\t'}, e.lines[r]...)
		e.invalidate(r)
		if r == e.anchor.Row {
			e.anchor.Col++
		}
		if r == e.cur.Row {
			e.cur.Col++
		}
	}
	e.changed()
}

// Dedent removes one level of indentation (a tab or up to TabWidth spaces)
// from the cursor line or every selected line.
func (e *Editor) Dedent() {
	a, b, ok := e.Selection()
	first, last := e.cur.Row, e.cur.Row
	if ok {
		first, last = a.Row, b.Row
		if b.Col == 0 && b.Row > a.Row {
			last--
		}
	}
	e.pushUndo(editOther)
	changed := false
	for r := first; r <= last; r++ {
		n := dedentWidth(e.lines[r], e.TabWidth)
		if n == 0 {
			continue
		}
		e.lines[r] = e.lines[r][n:]
		e.invalidate(r)
		changed = true
		if r == e.anchor.Row {
			e.anchor.Col = max(0, e.anchor.Col-n)
		}
		if r == e.cur.Row {
			e.cur.Col = max(0, e.cur.Col-n)
		}
	}
	if changed {
		e.changed()
	} else {
		e.undo = e.undo[:len(e.undo)-1]
	}
}

func dedentWidth(l []rune, tabW int) int {
	if len(l) > 0 && l[0] == '\t' {
		return 1
	}
	n := 0
	for n < len(l) && n < tabW && l[n] == ' ' {
		n++
	}
	return n
}

// --- movement --------------------------------------------------------------

func (e *Editor) startMove(extend bool) {
	if extend && !e.sel {
		e.anchor = e.cur
		e.sel = true
	}
	if !extend {
		e.sel = false
	}
	e.lastKind = editNone
}

// MoveTo moves the cursor, optionally extending the selection.
func (e *Editor) MoveTo(p Pos, extend bool) {
	e.startMove(extend)
	e.cur = e.clamp(p)
	e.goalX = -1
	e.EnsureVisible()
}

func (e *Editor) left(p Pos) Pos {
	if p.Col > 0 {
		return Pos{p.Row, p.Col - 1}
	}
	if p.Row > 0 {
		return Pos{p.Row - 1, len(e.lines[p.Row-1])}
	}
	return p
}

func (e *Editor) right(p Pos) Pos {
	if p.Col < len(e.lines[p.Row]) {
		return Pos{p.Row, p.Col + 1}
	}
	if p.Row < len(e.lines)-1 {
		return Pos{p.Row + 1, 0}
	}
	return p
}

func isWord(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

func (e *Editor) runeAt(p Pos) (rune, bool) {
	if p.Col < len(e.lines[p.Row]) {
		return e.lines[p.Row][p.Col], true
	}
	return '\n', false
}

func (e *Editor) wordLeft(p Pos) Pos {
	p = e.left(p)
	for {
		r, _ := e.runeAt(p)
		if isWord(r) || (p.Row == 0 && p.Col == 0) {
			break
		}
		p = e.left(p)
	}
	for p.Col > 0 && isWord(e.lines[p.Row][p.Col-1]) {
		p.Col--
	}
	return p
}

func (e *Editor) wordRight(p Pos) Pos {
	last := Pos{len(e.lines) - 1, len(e.lines[len(e.lines)-1])}
	for p != last {
		r, _ := e.runeAt(p)
		if isWord(r) {
			break
		}
		p = e.right(p)
	}
	for p.Col < len(e.lines[p.Row]) && isWord(e.lines[p.Row][p.Col]) {
		p.Col++
	}
	return p
}

func (e *Editor) vertical(delta int, extend bool) {
	vl, x := e.visualPos(e.cur)
	if e.goalX < 0 {
		e.goalX = x
	}
	goal := e.goalX
	e.startMove(extend)
	target := vl + delta
	switch {
	case target < 0:
		e.cur = Pos{}
	case target >= e.totalVisual():
		last := len(e.lines) - 1
		e.cur = Pos{last, len(e.lines[last])}
	default:
		e.cur = e.posAtVisual(target, goal)
	}
	e.goalX = goal
	e.EnsureVisible()
}

func (e *Editor) horizontal(p Pos, extend bool) {
	e.startMove(extend)
	e.cur = p
	e.goalX = -1
	e.EnsureVisible()
}

// HandleKey applies an editing or movement key. It returns false for keys the
// editor does not use.
func (e *Editor) HandleKey(msg tea.KeyPressMsg) bool {
	k := msg.Key()
	shift := k.Mod&tea.ModShift != 0
	switch msg.Keystroke() {
	case "left", "shift+left":
		if e.HasSelection() && !shift {
			a, _, _ := e.Selection()
			e.horizontal(a, false)
		} else {
			e.horizontal(e.left(e.cur), shift)
		}
	case "right", "shift+right":
		if e.HasSelection() && !shift {
			_, b, _ := e.Selection()
			e.horizontal(b, false)
		} else {
			e.horizontal(e.right(e.cur), shift)
		}
	case "up", "shift+up":
		e.vertical(-1, shift)
	case "down", "shift+down":
		e.vertical(1, shift)
	case "ctrl+left", "ctrl+shift+left", "alt+b":
		e.horizontal(e.wordLeft(e.cur), shift)
	case "ctrl+right", "ctrl+shift+right", "alt+f":
		e.horizontal(e.wordRight(e.cur), shift)
	case "home", "shift+home":
		indent := len(leadingWhitespace(e.lines[e.cur.Row]))
		col := indent
		if e.cur.Col == indent {
			col = 0
		}
		e.horizontal(Pos{e.cur.Row, col}, shift)
	case "end", "shift+end", "ctrl+e":
		e.horizontal(Pos{e.cur.Row, len(e.lines[e.cur.Row])}, shift)
	case "ctrl+home", "ctrl+shift+home":
		e.horizontal(Pos{}, shift)
	case "ctrl+end", "ctrl+shift+end":
		last := len(e.lines) - 1
		e.horizontal(Pos{last, len(e.lines[last])}, shift)
	case "pgup", "shift+pgup":
		e.vertical(-max(1, e.height-1), shift)
	case "pgdown", "shift+pgdown":
		e.vertical(max(1, e.height-1), shift)
	case "enter":
		e.Newline()
	case "backspace", "shift+backspace":
		e.Backspace()
	case "delete":
		e.Delete()
	case "ctrl+backspace", "alt+backspace", "ctrl+h":
		e.DeleteWordBack()
	case "ctrl+delete", "alt+delete", "alt+d":
		e.DeleteWordForward()
	case "tab":
		e.Indent()
	case "shift+tab":
		e.Dedent()
	case "ctrl+z":
		e.Undo()
	case "ctrl+y", "ctrl+shift+z":
		e.Redo()
	default:
		if k.Text != "" && k.Mod&(tea.ModCtrl|tea.ModAlt) == 0 {
			e.InsertText(k.Text)
			return true
		}
		return false
	}
	return true
}
