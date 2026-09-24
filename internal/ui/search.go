package ui

import (
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/thinkbig1979/cool-notes/internal/editor"
)

// search is the find bar: a case-insensitive text search across all notes,
// with an optional replace field.
type search struct {
	query   *editor.Editor
	replace *editor.Editor // nil until replace is opened
	// typing goes to the replace field instead of the query
	onReplace bool
	// With replace open, only note is searched unless allNotes is set.
	allNotes bool
	note     *tab
	matches  []match
	cur      int // index into matches, -1 if none
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
	q := newField()
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

func newField() *editor.Editor {
	f := editor.New()
	f.SingleLine = true
	f.SetSize(1<<20, 1)
	return f
}

// field is the input that typing goes to.
func (s *search) field() *editor.Editor {
	if s.onReplace {
		return s.replace
	}
	return s.query
}

// openReplace opens the find bar with its replace field, searching the
// current note. With the bar already open it moves to the replace field.
func (m *Model) openReplace() {
	if m.search == nil {
		m.openSearch()
	}
	s := m.search
	m.flash = "" // the status line shows the replace keys
	if s.replace != nil {
		s.onReplace = true
		return
	}
	s.replace = newField()
	s.onReplace = s.query.Text() != ""
	s.allNotes = false
	m.searchScopeChanged()
}

// toggleScope switches replace between the current note and all notes.
func (m *Model) toggleScope() {
	m.search.allNotes = !m.search.allNotes
	m.searchScopeChanged()
}

// searchScopeChanged re-runs the search from the cursor in the current note.
func (m *Model) searchScopeChanged() {
	s := m.search
	s.note = m.tabs[m.active]
	s.originTab = m.active
	s.origin, _, _ = m.ed().Selection()
	m.searchUpdated()
}

// inScope reports whether matches in note i count.
func (m *Model) inScope(i int) bool {
	s := m.search
	return s.replace == nil || s.allNotes || m.tabs[i] == s.note
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
	s.matches = s.matches[:0]
	for _, mt := range findAll(m.texts(), s.query.Text()) {
		if m.inScope(mt.tab) {
			s.matches = append(s.matches, mt)
		}
	}
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
		// Don't leave a stale match selected in the note we move away from.
		m.ed().ClearSelection()
		m.switchTab(mt.tab)
	}
	m.ed().MoveTo(mt.start, false)
	m.ed().MoveTo(mt.end, true)
}

func (m *Model) searchKey(msg tea.KeyPressMsg) tea.Cmd {
	s := m.search
	switch msg.Keystroke() {
	case "esc":
		m.search = nil
	case "enter":
		if s.replace != nil {
			return m.replaceOne()
		}
		m.searchStep(1)
	case "down", "ctrl+f", "f3":
		m.searchStep(1)
	case "shift+enter", "up", "shift+f3":
		m.searchStep(-1)
	case "ctrl+r":
		m.openReplace()
	case "tab", "shift+tab":
		if s.replace == nil {
			m.openReplace()
			s.onReplace = true
		} else {
			s.onReplace = !s.onReplace
		}
	case "alt+a":
		if s.replace != nil {
			return m.replaceAll()
		}
	case "alt+n":
		if s.replace != nil {
			m.toggleScope()
		}
	case "ctrl+a":
		s.field().SelectAll()
	default:
		// Anything else not meant for the query closes the bar and goes to the note.
		k := msg.Key()
		if k.Mod&(tea.ModCtrl|tea.ModAlt) != 0 && !isQueryEditKey(msg.Keystroke()) {
			m.search = nil
			return m.handleKey(msg)
		}
		f := s.field()
		before := f.Version()
		f.HandleKey(msg)
		if f == s.query && f.Version() != before {
			return m.searchUpdated()
		}
	}
	return nil
}

// replaceOne replaces the current match and moves on to the next one.
func (m *Model) replaceOne() tea.Cmd {
	s := m.search
	if s.cur < 0 {
		return nil
	}
	mt := s.matches[s.cur]
	if mt.tab != m.active {
		m.switchTab(mt.tab)
	}
	cmd := m.edit(func(e *editor.Editor) { e.ReplaceRange(mt.start, mt.end, s.replace.Text()) })
	// Continue after the new text, so a replacement containing the query
	// isn't found again.
	s.originTab, s.origin = mt.tab, m.ed().Cursor()
	m.searchUpdated()
	return cmd
}

// replaceAll asks before replacing across notes, then replaces every match.
func (m *Model) replaceAll() tea.Cmd {
	if len(m.search.matches) == 0 {
		return nil
	}
	if m.search.allNotes {
		m.modal = modalConfirmReplace
		m.confirmFocus = 0
		return nil
	}
	return m.doReplaceAll()
}

// doReplaceAll replaces every match as one undo step per note, closes the
// bar, and remembers the notes so Ctrl+Z can undo them all at once.
func (m *Model) doReplaceAll() tea.Cmd {
	s := m.search
	m.modal = modalNone
	byTab := map[int][]match{}
	var order []int
	for _, mt := range s.matches {
		if byTab[mt.tab] == nil {
			order = append(order, mt.tab)
		}
		byTab[mt.tab] = append(byTab[mt.tab], mt)
	}
	repl := s.replace.Text()
	m.replaced = m.replaced[:0]
	for _, i := range order {
		t := m.tabs[i]
		t.ed.ReplaceText(replaceMatches(t.ed.Text(), byTab[i], repl))
		m.replaced = append(m.replaced, t)
	}
	m.deleted = nil
	m.search = nil
	msg := fmt.Sprintf("replaced %s", plural(len(s.matches), "match", "matches"))
	if len(order) > 1 {
		msg += " in " + plural(len(order), "note", "notes")
	}
	return tea.Batch(m.saveNow(), m.setFlash(msg+"  ·  Ctrl+Z to undo", false))
}

// undoReplaceAll reverts the last replace all in every note it touched.
func (m *Model) undoReplaceAll() tea.Cmd {
	for _, t := range m.replaced {
		t.ed.Undo()
	}
	n := len(m.replaced)
	m.replaced = nil
	msg := "undid replace"
	if n > 1 {
		msg += " in " + plural(n, "note", "notes")
	}
	return tea.Batch(m.saveNow(), m.setFlash(msg, false))
}

// replaceMatches replaces ms, which are all in text and in reading order.
func replaceMatches(text string, ms []match, repl string) string {
	lines := strings.Split(text, "\n")
	r := []rune(repl)
	for i := len(ms) - 1; i >= 0; i-- {
		mt := ms[i]
		l := []rune(lines[mt.start.Row])
		nl := append(append(append([]rune{}, l[:mt.start.Col]...), r...), l[mt.end.Col:]...)
		lines[mt.start.Row] = string(nl)
	}
	return strings.Join(lines, "\n")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
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

// viewSearch draws the find bar in place of the hotkey bar and returns the
// cursor column in the field being typed into.
func (m *Model) viewSearch() (string, int) {
	st := m.st
	s := m.search
	where := "all notes"
	if s.replace != nil && !s.allNotes {
		where = "this note"
	}
	var info string
	switch {
	case s.query.Text() == "" && s.replace == nil:
		info = st.hintLabel.Render("type to search all notes  ·  Ctrl+R replace  ·  Esc close")
	case s.query.Text() == "":
		info = st.hintLabel.Render("type to search " + where)
	case len(s.matches) == 0:
		info = st.statusErr.UnsetBackground().Render("no matches in " + where)
	case s.replace != nil:
		info = st.hintLabel.Render(fmt.Sprintf("%d/%d in %s", s.cur+1, len(s.matches), where))
	default:
		info = st.hintLabel.Render(fmt.Sprintf("%d/%d  ·  Enter next  ·  Shift+Enter previous  ·  Esc close", s.cur+1, len(s.matches)))
	}
	line, cursorX := " ", 1
	add := func(label string, f *editor.Editor) {
		ls := st.hintLabel
		if f == s.field() {
			ls = st.hintKey
			cursorX = ansi.StringWidth(line) + ansi.StringWidth(label) + 1 + min(ansi.StringWidth(f.Text()), ansiCursorCol(f))
		}
		line += ls.Render(label) + " " + st.text.Render(f.Text()) + "   "
	}
	add(" Find ", s.query)
	if s.replace != nil {
		add(" Replace ", s.replace)
	}
	return fit(line+info, m.width), cursorX
}

// replaceHints is the replace bar's key help, shown on the status line.
func (m *Model) replaceHints() string {
	other := "all notes"
	if m.search.allNotes {
		other = "this note"
	}
	return "Enter replace · ↓ skip · Alt+A all · Alt+N " + other + " · Esc close"
}

// ansiCursorCol is the display column of the query's cursor.
func ansiCursorCol(q *editor.Editor) int {
	r := []rune(q.Text())
	c := min(q.Cursor().Col, len(r))
	return ansi.StringWidth(string(r[:c]))
}
