package editor

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func key(code rune, mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Mod: mod}
}

func typeText(e *Editor, s string) {
	for _, r := range s {
		switch r {
		case '\n':
			e.HandleKey(key(tea.KeyEnter, 0))
		case '\t':
			e.HandleKey(key(tea.KeyTab, 0))
		default:
			e.HandleKey(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
	}
}

func newEd(text string, w, h int) *Editor {
	e := New()
	e.SetSize(w, h)
	e.SetText(text)
	return e
}

func plain(e *Editor) string {
	return e.View(Styles{Guides: false})
}

func TestTypingAndAutoIndent(t *testing.T) {
	e := newEd("", 40, 5)
	typeText(e, "list\n\titem one\nitem two")
	want := "list\n\titem one\n\titem two"
	if got := e.Text(); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if e.Cursor() != (Pos{2, 9}) {
		t.Fatalf("cursor %v", e.Cursor())
	}
}

func TestTabIndentsSelectionAndShiftTabDedents(t *testing.T) {
	e := newEd("a\nb\n\nc", 40, 5)
	e.MoveTo(Pos{0, 0}, false)
	e.MoveTo(Pos{3, 1}, true)
	e.HandleKey(key(tea.KeyTab, 0))
	if got := e.Text(); got != "\ta\n\tb\n\n\tc" {
		t.Fatalf("indent: %q", got)
	}
	if !e.HasSelection() {
		t.Fatal("selection lost after indent")
	}
	e.HandleKey(key(tea.KeyTab, tea.ModShift))
	if got := e.Text(); got != "a\nb\n\nc" {
		t.Fatalf("dedent: %q", got)
	}
	// Single line: tab inserts a tab at the cursor, shift+tab removes spaces too.
	e = newEd("    x", 40, 5)
	e.HandleKey(key(tea.KeyTab, tea.ModShift))
	if got := e.Text(); got != "x" {
		t.Fatalf("space dedent: %q", got)
	}
	e.MoveTo(Pos{0, 1}, false)
	e.HandleKey(key(tea.KeyTab, 0))
	if got := e.Text(); got != "x\t" {
		t.Fatalf("tab insert: %q", got)
	}
}

func TestSelectionEndingAtColumnZeroSkipsThatLine(t *testing.T) {
	e := newEd("a\nb\nc", 40, 5)
	e.MoveTo(Pos{0, 0}, false)
	e.MoveTo(Pos{2, 0}, true)
	e.Indent()
	if got := e.Text(); got != "\ta\n\tb\nc" {
		t.Fatalf("got %q", got)
	}
}

func TestSoftWrapAndVerticalMovement(t *testing.T) {
	e := newEd("hello world foo bar", 11, 5) // text width 10
	got := plain(e)
	lines := strings.Split(got, "\n")
	if strings.TrimRight(lines[0], " ") != "hello" || strings.TrimRight(lines[1], " ") != "world foo" || strings.TrimRight(lines[2], " ") != "bar" {
		t.Fatalf("wrap:\n%s", got)
	}
	e.MoveTo(Pos{0, 1}, false)
	e.HandleKey(key(tea.KeyDown, 0))
	if e.Cursor() != (Pos{0, 7}) {
		t.Fatalf("down within wrapped line: %v", e.Cursor())
	}
	e.HandleKey(key(tea.KeyDown, 0))
	if e.Cursor() != (Pos{0, 17}) {
		t.Fatalf("down again: %v", e.Cursor())
	}
}

func TestTabsRenderToTabStops(t *testing.T) {
	e := newEd("\tx\n\t\ty\nab\tz", 20, 3)
	lines := strings.Split(plain(e), "\n")
	want := []string{"    x", "        y", "ab  z"}
	for i, w := range want {
		if strings.TrimRight(lines[i], " ") != w {
			t.Fatalf("line %d = %q, want %q", i, lines[i], w)
		}
	}
	e.MoveTo(Pos{1, 2}, false)
	if x, y, _ := e.CursorView(); x != 8 || y != 1 {
		t.Fatalf("cursor view %d,%d", x, y)
	}
	guides := e.View(Styles{Guides: true, Guide: lipgloss.NewStyle()})
	if !strings.HasPrefix(strings.Split(guides, "\n")[1], "│   │   y") {
		t.Fatalf("guides: %q", guides)
	}
}

func TestUndoRedoCoalescesTyping(t *testing.T) {
	e := newEd("", 40, 5)
	typeText(e, "abc")
	typeText(e, " def")
	e.Undo()
	if got := e.Text(); got != "abc" {
		t.Fatalf("undo 1: %q", got)
	}
	e.Undo()
	if got := e.Text(); got != "" {
		t.Fatalf("undo 2: %q", got)
	}
	e.Redo()
	e.Redo()
	if got := e.Text(); got != "abc def" {
		t.Fatalf("redo: %q", got)
	}
}

func TestBackspaceDeleteJoinLines(t *testing.T) {
	e := newEd("ab\ncd", 40, 5)
	e.MoveTo(Pos{1, 0}, false)
	e.Backspace()
	if e.Text() != "abcd" || e.Cursor() != (Pos{0, 2}) {
		t.Fatalf("backspace join: %q %v", e.Text(), e.Cursor())
	}
	e.MoveTo(Pos{0, 1}, false)
	e.Delete()
	if e.Text() != "acd" {
		t.Fatalf("delete: %q", e.Text())
	}
	e = newEd("one two  three", 40, 5)
	e.MoveTo(Pos{0, 14}, false)
	e.DeleteWordBack()
	if e.Text() != "one two  " {
		t.Fatalf("word back: %q", e.Text())
	}
}

func TestMouseClickDragAndWord(t *testing.T) {
	e := newEd("\tfoo bar\nsecond", 40, 5)
	e.Click(5, 0, false) // "f" is drawn at x=4, "o" at 5
	if e.Cursor() != (Pos{0, 2}) {
		t.Fatalf("click: %v", e.Cursor())
	}
	e.Click(1, 0, false) // left half of the tab snaps before it
	if e.Cursor() != (Pos{0, 0}) {
		t.Fatalf("click tab: %v", e.Cursor())
	}
	e.Click(30, 1, false) // past end of line
	if e.Cursor() != (Pos{1, 6}) {
		t.Fatalf("click past end: %v", e.Cursor())
	}
	e.Click(4, 0, false)
	e.Drag(3, 1)
	if got := e.SelectedText(); got != "foo bar\nsec" {
		t.Fatalf("drag: %q", got)
	}
	e.SelectWordAt(9, 0)
	if got := e.SelectedText(); got != "bar" {
		t.Fatalf("word: %q", got)
	}
	e.SelectLineAt(0, 0)
	if got := e.SelectedText(); got != "\tfoo bar\n" {
		t.Fatalf("line: %q", got)
	}
}

func TestScrollFollowsCursor(t *testing.T) {
	e := newEd(strings.Repeat("line\n", 20), 20, 5)
	e.HandleKey(key(tea.KeyEnd, tea.ModCtrl))
	if _, y, ok := e.CursorView(); !ok || y != 4 {
		t.Fatalf("cursor after ctrl+end: y=%d ok=%v scroll=%d", y, ok, e.Scroll())
	}
	e.ScrollBy(-100)
	if e.Scroll() != 0 {
		t.Fatalf("scroll clamp: %d", e.Scroll())
	}
	if _, _, ok := e.CursorView(); ok {
		t.Fatal("cursor should be off screen after wheel scroll")
	}
}

func TestSelectionReplacedByTyping(t *testing.T) {
	e := newEd("hello world", 40, 5)
	e.MoveTo(Pos{0, 0}, false)
	e.MoveTo(Pos{0, 5}, true)
	typeText(e, "bye")
	if e.Text() != "bye world" {
		t.Fatalf("got %q", e.Text())
	}
}

func TestViewAlwaysFillsArea(t *testing.T) {
	e := newEd("日本語のテキスト and emoji 🙂 wrapping around", 12, 6)
	lines := strings.Split(plain(e), "\n")
	if len(lines) != 6 {
		t.Fatalf("got %d lines", len(lines))
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w != 12 {
			t.Errorf("line %d width %d: %q", i, w, l)
		}
	}
}

func TestHangingIndentOnWrappedLines(t *testing.T) {
	e := newEd("\tabc def ghi", 11, 4)
	lines := strings.Split(e.View(Styles{Guides: true}), "\n")
	want := []string{"│   abc", "│   def", "│   ghi"}
	for i, w := range want {
		if strings.TrimRight(lines[i], " ") != w {
			t.Fatalf("line %d = %q, want %q\n%s", i, lines[i], w, strings.Join(lines, "\n"))
		}
	}
	e.Click(5, 1, false) // the 'e' in "def"
	if e.Cursor() != (Pos{0, 6}) {
		t.Fatalf("click on continuation: %v", e.Cursor())
	}
	if x, y, _ := e.CursorView(); x != 5 || y != 1 {
		t.Fatalf("cursor view %d,%d", x, y)
	}
	e.HandleKey(key(tea.KeyUp, 0))
	if e.Cursor() != (Pos{0, 2}) {
		t.Fatalf("up from continuation: %v", e.Cursor())
	}
}

// Undo snapshots share line slices with the document, so every edit has to
// leave old slices untouched.
func TestUndoSnapshotsAreNotCorruptedByLaterEdits(t *testing.T) {
	e := newEd("hello\nworld", 40, 5)
	e.MoveTo(Pos{0, 5}, false)
	e.Backspace()
	e.Backspace()
	e.MoveTo(Pos{1, 0}, false)
	e.Delete()
	e.MoveTo(Pos{0, 3}, false)
	e.Delete() // joins lines
	typeText(e, "X")
	for e.Text() != "hello\nworld" && len(e.undo) > 0 {
		e.Undo()
	}
	if e.Text() != "hello\nworld" {
		t.Fatalf("undo chain ended at %q", e.Text())
	}
	for len(e.redo) > 0 {
		e.Redo()
	}
	if e.Text() != "helXorld" {
		t.Fatalf("redo chain ended at %q", e.Text())
	}
}

func TestReplaceRangeIsOneUndoStep(t *testing.T) {
	e := newEd("say hello there", 80, 5)
	e.ReplaceRange(Pos{0, 4}, Pos{0, 9}, "hi")
	if e.Text() != "say hi there" || e.Cursor() != (Pos{0, 6}) {
		t.Fatalf("text %q cursor %+v", e.Text(), e.Cursor())
	}
	typeText(e, "!")
	e.Undo()
	e.Undo()
	if e.Text() != "say hello there" {
		t.Fatalf("after undo: %q", e.Text())
	}
}
