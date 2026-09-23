package ui

import (
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/thinkbig1979/cool-notes/internal/editor"
)

// search is the find bar: a case-insensitive text search across all notes.
type search struct {
	query   *editor.Editor
	matches []match
	cur     int // index into matches, -1 if none
	// where the search started, so typing finds the first match from there
	originTab int
	origin    editor.Pos
}

// match is one occurrence of the query: note index and rune positions.
type match struct {
	tab        int
	start, end editor.Pos
}

func (a match) before(tab int, p editor.Pos) bool {
	return a.tab < tab || (a.tab == tab && a.start.Before(p))
}

func (m *Model) openSearch() {
	if m.search != nil {
		m.searchStep(1)
		return
	}
	q := editor.New()
	q.SingleLine = true
	q.SetSize(1<<20, 1)
	s := &search{query: q, cur: -1, originTab: m.active, origin: m.ed().Cursor()}
	// Start from the selection, so selecting a word and pressing Ctrl+F finds it.
	if sel := m.ed().SelectedText(); sel != "" && !strings.Contains(sel, "\n") {
		q.SetText(sel)
		q.SelectAll()
		a, _, _ := m.ed().Selection()
		s.origin = a
	}
	m.search = s
	m.searchUpdated()
}

// findAll returns every occurrence of q in the notes, in reading order.
func findAll(notes []string, q string) []match {
	needle := []rune(strings.Map(unicode.ToLower, q))
	if len(needle) == 0 {
		return nil
	}
	var out []match
	for t, text := range notes {
		for row, line := range strings.Split(text, "\n") {
			hay := []rune(strings.Map(unicode.ToLower, line))
			for col := 0; col+len(needle) <= len(hay); col++ {
				if runesEqual(hay[col:col+len(needle)], needle) {
					out = append(out, match{t, editor.Pos{Row: row, Col: col}, editor.Pos{Row: row, Col: col + len(needle)}})
					col += len(needle) - 1
				}
			}
		}
	}
	return out
}

func runesEqual(a, b []rune) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// searchUpdated re-runs the search after the query changed and jumps to the
// first match at or after where the search started.
func (m *Model) searchUpdated() tea.Cmd {
	s := m.search
	s.matches = findAll(m.texts(), s.query.Text())
	s.cur = -1
	if len(s.matches) == 0 {
		return nil
	}
	s.cur = 0
	for i, mt := range s.matches {
		if !mt.before(s.originTab, s.origin) {
			s.cur = i
			break
		}
	}
	m.showMatch()
	return nil
}

// searchStep moves to the next (+1) or previous (-1) match, wrapping around.
func (m *Model) searchStep(dir int) {
	s := m.search
	if len(s.matches) == 0 {
		return
	}
	s.cur = (s.cur + dir + len(s.matches)) % len(s.matches)
	m.showMatch()
}

// showMatch opens the note with the current match and selects it.
func (m *Model) showMatch() {
	mt := m.search.matches[m.search.cur]
	if mt.tab != m.active {
		m.switchTab(mt.tab)
	}
	m.ed().MoveTo(mt.start, false)
	m.ed().MoveTo(mt.end, true)
}

func (m *Model) searchKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.Keystroke() {
	case "esc":
		m.search = nil
	case "enter", "down", "ctrl+f", "f3":
		m.searchStep(1)
	case "shift+enter", "up", "shift+f3":
		m.searchStep(-1)
	case "ctrl+a":
		m.search.query.SelectAll()
	case "tab", "shift+tab":
	default:
		// Anything else not meant for the query closes the bar and goes to the note.
		k := msg.Key()
		if k.Mod&(tea.ModCtrl|tea.ModAlt) != 0 && !isQueryEditKey(msg.Keystroke()) {
			m.search = nil
			return m.handleKey(msg)
		}
		before := m.search.query.Version()
		m.search.query.HandleKey(msg)
		if m.search.query.Version() != before {
			return m.searchUpdated()
		}
	}
	return nil
}

// isQueryEditKey reports whether a modified key edits the find query.
func isQueryEditKey(ks string) bool {
	switch ks {
	case "ctrl+left", "ctrl+right", "ctrl+shift+left", "ctrl+shift+right",
		"ctrl+backspace", "alt+backspace", "ctrl+h", "ctrl+delete", "alt+delete",
		"ctrl+z", "ctrl+y", "ctrl+shift+z", "ctrl+e", "alt+b", "alt+f", "alt+d":
		return true
	}
	return false
}

// viewSearch draws the find bar in place of the hotkey bar.
func (m *Model) viewSearch() (string, int) {
	st := m.st
	s := m.search
	label := st.hintKey.Render(" Find ") + " "
	var info string
	switch {
	case s.query.Text() == "":
		info = st.hintLabel.Render("type to search all notes  ·  Esc close")
	case len(s.matches) == 0:
		info = st.statusErr.UnsetBackground().Render("no matches")
	default:
		info = st.hintLabel.Render(fmt.Sprintf("%d/%d  ·  Enter next  ·  Shift+Enter previous  ·  Esc close", s.cur+1, len(s.matches)))
	}
	q := s.query.Text()
	qw := ansi.StringWidth(q)
	line := " " + label + st.text.Render(q) + "   " + info
	cursorX := 1 + ansi.StringWidth(" Find ") + 1 + min(qw, ansiCursorCol(s.query))
	return fit(line, m.width), cursorX
}

// ansiCursorCol is the display column of the query's cursor.
func ansiCursorCol(q *editor.Editor) int {
	r := []rune(q.Text())
	c := min(q.Cursor().Col, len(r))
	return ansi.StringWidth(string(r[:c]))
}
