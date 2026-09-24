package ui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/thinkbig1979/cool-notes/internal/editor"
)

func TestCtrlZRestoresDeletedNote(t *testing.T) {
	m := newTestModel(t, "a", "b", "c")
	m.switchTab(1)
	press(m, "ctrl+w", "y")
	if got := m.texts(); !reflect.DeepEqual(got, []string{"a", "c"}) {
		t.Fatalf("after delete: %q", got)
	}
	if !strings.Contains(m.flash, "Ctrl+Z to restore") {
		t.Fatalf("flash = %q", m.flash)
	}
	press(m, "ctrl+z")
	m.Flush()
	if got := fileNotes(t, m); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("after restore: %q", got)
	}
	if m.active != 1 {
		t.Fatalf("active = %d, want the restored note", m.active)
	}
}

func TestCtrlZRestoresOnlyNote(t *testing.T) {
	m := newTestModel(t, "only")
	press(m, "ctrl+w", "y", "ctrl+z")
	if got := m.texts(); !reflect.DeepEqual(got, []string{"only"}) {
		t.Fatalf("got %q, want the note back without the empty placeholder", got)
	}
}

func TestRestoreExpiresAfterAnotherKey(t *testing.T) {
	m := newTestModel(t, "a", "b")
	press(m, "ctrl+w", "y", "x", "ctrl+z")
	// The x was typed into "b"; Ctrl+Z now undoes that edit instead.
	if got := m.texts(); !reflect.DeepEqual(got, []string{"b"}) {
		t.Fatalf("got %q", got)
	}
}

func TestMoveTabReordersNotes(t *testing.T) {
	m := newTestModel(t, "a", "b", "c")
	press(m, "alt+shift+right") // at the first note
	m.Flush()
	if got := fileNotes(t, m); !reflect.DeepEqual(got, []string{"b", "a", "c"}) {
		t.Fatalf("after right: %q", got)
	}
	if m.active != 1 {
		t.Fatalf("active = %d, should follow the moved note", m.active)
	}
	press(m, "alt+shift+left", "alt+shift+left") // second press is a no-op at the edge
	if got := m.texts(); !reflect.DeepEqual(got, []string{"a", "b", "c"}) || m.active != 0 {
		t.Fatalf("after left: %q active %d", got, m.active)
	}
}

func TestSearchAcrossNotes(t *testing.T) {
	m := newTestModel(t, "Groceries\n\tmilk", "Ideas\n\tMilkshake bar", "Nothing here")
	press(m, "ctrl+f", "milk")
	if m.search == nil || len(m.search.matches) != 2 {
		t.Fatalf("matches = %+v", m.search)
	}
	// Case-insensitive; first match is in the note where the search started.
	if m.active != 0 || m.ed().SelectedText() != "milk" {
		t.Fatalf("active %d selected %q", m.active, m.ed().SelectedText())
	}
	press(m, "enter")
	if m.active != 1 || m.ed().SelectedText() != "Milk" {
		t.Fatalf("next: active %d selected %q", m.active, m.ed().SelectedText())
	}
	press(m, "enter") // wraps around
	if m.active != 0 {
		t.Fatalf("wrap: active %d", m.active)
	}
	press(m, "shift+enter")
	if m.active != 1 {
		t.Fatalf("previous: active %d", m.active)
	}
	press(m, "esc")
	if m.search != nil || m.ed().SelectedText() != "Milk" {
		t.Fatal("esc should close the bar and keep the match selected")
	}
	if got := m.texts()[0]; got != "Groceries\n\tmilk" {
		t.Fatalf("search typing leaked into a note: %q", got)
	}
}

func TestSearchNoMatchesAndEditing(t *testing.T) {
	m := newTestModel(t, "alpha", "beta")
	press(m, "ctrl+f", "zzz")
	if len(m.search.matches) != 0 || m.search.cur != -1 {
		t.Fatalf("matches = %+v", m.search.matches)
	}
	if v := m.View(); !strings.Contains(v.Content, "no matches") {
		t.Fatal("no-match message not shown")
	}
	press(m, "backspace", "backspace", "backspace", "eta")
	if len(m.search.matches) != 1 || m.active != 1 {
		t.Fatalf("after edit: %+v active %d", m.search.matches, m.active)
	}
}

func TestSearchStartsFromSelection(t *testing.T) {
	m := newTestModel(t, "one two one")
	m.ed().MoveTo(editor.Pos{Col: 8}, false)
	m.ed().MoveTo(editor.Pos{Col: 11}, true)
	press(m, "ctrl+f")
	if m.search.query.Text() != "one" || m.search.cur != 1 {
		t.Fatalf("query %q cur %d", m.search.query.Text(), m.search.cur)
	}
}

func TestFindAllUnicode(t *testing.T) {
	got := findAll([]string{"Ünïcode ünï", "x"}, "ÜNÏ")
	want := []match{
		{0, editor.Pos{Col: 0}, editor.Pos{Col: 3}},
		{0, editor.Pos{Col: 8}, editor.Pos{Col: 11}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestReplaceInCurrentNote(t *testing.T) {
	m := newTestModel(t, "cat cat", "cat")
	press(m, "ctrl+r", "cat", "tab", "dog")
	if len(m.search.matches) != 2 {
		t.Fatalf("should only search the current note: %+v", m.search.matches)
	}
	press(m, "enter")
	if got := m.texts(); !reflect.DeepEqual(got, []string{"dog cat", "cat"}) {
		t.Fatalf("after one replace: %q", got)
	}
	if m.active != 0 || m.ed().SelectedText() != "cat" {
		t.Fatalf("should move to the next match: active %d selected %q", m.active, m.ed().SelectedText())
	}
	press(m, "alt+a")
	if got := m.texts(); !reflect.DeepEqual(got, []string{"dog dog", "cat"}) {
		t.Fatalf("after replace all: %q", got)
	}
	if m.search != nil || !strings.Contains(m.flash, "Ctrl+Z to undo") {
		t.Fatalf("bar should close with an undo hint, flash %q", m.flash)
	}
	press(m, "ctrl+z")
	if got := m.texts(); !reflect.DeepEqual(got, []string{"dog cat", "cat"}) {
		t.Fatalf("after undo: %q", got)
	}
}

func TestReplaceAllNotesAsksAndUndoesTogether(t *testing.T) {
	m := newTestModel(t, "a x", "x", "none")
	press(m, "ctrl+r", "x", "tab", "y", "alt+n", "alt+a")
	if m.modal != modalConfirmReplace {
		t.Fatalf("replace across notes should ask first, modal %d", m.modal)
	}
	if v := m.View(); !strings.Contains(v.Content, "2 matches in 2 notes") {
		t.Fatal("dialog should say how much changes")
	}
	press(m, "y")
	m.Flush()
	if got := fileNotes(t, m); !reflect.DeepEqual(got, []string{"a y", "y", "none"}) {
		t.Fatalf("after replace all: %q", got)
	}
	press(m, "ctrl+z")
	if got := m.texts(); !reflect.DeepEqual(got, []string{"a x", "x", "none"}) {
		t.Fatalf("one Ctrl+Z should undo every note: %q", got)
	}
}

func TestReplaceSkipsItsOwnOutput(t *testing.T) {
	m := newTestModel(t, "a a")
	press(m, "ctrl+r", "a", "tab", "aa", "enter", "enter")
	if got := m.texts()[0]; got != "aa aa" {
		t.Fatalf("got %q", got)
	}
}

func TestTabInFindOpensReplace(t *testing.T) {
	m := newTestModel(t, "one", "one")
	press(m, "ctrl+f", "one")
	if len(m.search.matches) != 2 {
		t.Fatalf("find searches all notes: %+v", m.search.matches)
	}
	press(m, "tab")
	if m.search.replace == nil || !m.search.onReplace || len(m.search.matches) != 1 {
		t.Fatalf("tab should open replace on the current note: %+v", m.search)
	}
	press(m, "zero", "tab", "s")
	if m.search.query.Text() != "ones" || m.search.replace.Text() != "zero" {
		t.Fatalf("query %q replace %q", m.search.query.Text(), m.search.replace.Text())
	}
}

func TestSearchLeavesNoStaleSelection(t *testing.T) {
	m := newTestModel(t, "xs", "second")
	press(m, "ctrl+f", "s")
	if m.active != 0 || m.ed().SelectedText() != "s" {
		t.Fatalf("first match: active %d selected %q", m.active, m.ed().SelectedText())
	}
	press(m, "e") // "se" is only in the second note
	if m.active != 1 || m.tabs[0].ed.HasSelection() {
		t.Fatalf("active %d, first note still has a selection", m.active)
	}
}
