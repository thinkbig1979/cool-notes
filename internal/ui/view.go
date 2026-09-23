package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"cool-note/internal/config"
	"cool-note/internal/editor"
)

const tabTitleMax = 22

// View implements tea.Model.
func (m *Model) View() tea.View {
	var content string
	var cursor *tea.Cursor
	title := "cool-note"
	if m.mode == modeSetup {
		content, cursor = m.viewSetup()
	} else {
		content, cursor = m.viewNotes()
		title = tabTitle(m.ed().Text()) + " · cool-note"
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = title
	if cursor != nil {
		cursor.Shape = tea.CursorBar
		cursor.Color = col(m.st.theme.Accent)
		v.Cursor = cursor
	}
	return v
}

func fit(s string, w int) string {
	if gap := w - lipgloss.Width(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return ansi.Truncate(s, w, "")
}

func (m *Model) viewNotes() (string, *tea.Cursor) {
	ex, ey, ew, _ := m.editorRect()
	e := m.ed()

	lines := make([]string, 0, m.height)
	lines = append(lines, m.viewTabBar(), strings.Repeat(" ", m.width))
	body := strings.Split(e.View(editor.Styles{
		Text: m.st.text, Selection: m.st.selection, Guide: m.st.guide, Guides: true,
	}), "\n")
	if e.Text() == "" && len(body) > 0 {
		hint := "Start typing. The first line becomes the tab title."
		if lipgloss.Width(hint) > ew {
			hint = "Start typing…"
		}
		body[0] = fit(m.st.guide.Italic(true).Render(hint), ew)
	}
	left := strings.Repeat(" ", ex)
	right := strings.Repeat(" ", max(0, m.width-ex-ew))
	for _, l := range body {
		lines = append(lines, left+l+right)
	}
	for len(lines) < m.height-statusRows {
		lines = append(lines, strings.Repeat(" ", m.width))
	}
	lines = append(lines[:max(0, m.height-statusRows)], m.viewHints(m.height-2), m.viewStatus())
	base := strings.Join(lines, "\n")

	var cursor *tea.Cursor
	switch m.modal {
	case modalConfirmDelete:
		base = m.overlay(base, m.viewConfirm())
	case modalHelp:
		base = m.overlay(base, m.viewHelp())
	default:
		if x, y, ok := e.CursorView(); ok {
			cursor = tea.NewCursor(ex+x, ey+y)
		}
	}
	return base, cursor
}

// --- tab bar ---------------------------------------------------------------

func (m *Model) viewTabBar() string {
	st := m.st
	type part struct{ text, close string }
	parts := make([]part, len(m.tabs))
	widths := make([]int, len(m.tabs))
	for i, t := range m.tabs {
		parts[i] = part{" " + tabLabel(tabTitle(t.ed.Text()), tabTitleMax) + " ", "× "}
		widths[i] = lipgloss.Width(parts[i].text) + 2
	}
	const newW, arrowW = 3, 2
	avail := m.width - 1 - newW - 1

	span := func(from, to int) int { // width of tabs [from,to) with gaps
		w := 0
		for i := from; i < to; i++ {
			w += widths[i] + 1
		}
		return w
	}
	first, last := 0, len(m.tabs)
	if span(0, len(m.tabs)) > avail {
		room := avail - 2*arrowW
		if m.active < m.tabOffset {
			m.tabOffset = m.active
		}
		for m.tabOffset < m.active && span(m.tabOffset, m.active+1) > room {
			m.tabOffset++
		}
		first, last = m.tabOffset, m.tabOffset
		for last < len(m.tabs) && span(first, last+1) <= room {
			last++
		}
		last = max(last, first+1)
	} else {
		m.tabOffset = 0
	}

	m.hits = m.hits[:0]
	var b strings.Builder
	x := 0
	put := func(s string, style lipgloss.Style) int {
		b.WriteString(style.Render(s))
		w := lipgloss.Width(s)
		x += w
		return w
	}
	put(" ", st.bar)
	if first > 0 {
		x0 := x
		put("‹ ", st.tabArrow)
		m.hits = append(m.hits, hit{0, x0, x, hitPrev, first - 1})
	}
	for i := first; i < last; i++ {
		ts, cs := st.tab, st.tabClose
		if i == m.active {
			ts, cs = st.activeTab, st.activeClose
		}
		x0 := x
		put(parts[i].text, ts)
		m.hits = append(m.hits, hit{0, x0, x, hitTab, i})
		x0 = x
		put(parts[i].close, cs)
		m.hits = append(m.hits, hit{0, x0, x, hitClose, i})
		put(" ", st.bar)
	}
	if last < len(m.tabs) {
		x0 := x
		put(" ›", st.tabArrow)
		m.hits = append(m.hits, hit{0, x0, x, hitNext, last})
	}
	x0 := x
	put(" + ", st.newTab)
	m.hits = append(m.hits, hit{0, x0, x, hitNew, 0})
	if x < m.width {
		put(strings.Repeat(" ", m.width-x), st.bar)
	}
	return ansi.Truncate(b.String(), m.width, "")
}

// --- status bar ------------------------------------------------------------

func (m *Model) viewStatus() string {
	st := m.st
	var left string
	switch {
	case m.saveErr != nil:
		left = st.statusErr.Render(" ✗ save failed: " + m.saveErr.Error() + " ")
	case m.dirty():
		left = st.statusBusy.Render(" ● saving ")
	default:
		left = st.statusOk.Render(" ✓ saved  ")
	}
	switch {
	case m.flash != "" && m.flashErr:
		left += st.statusErr.Render(" " + m.flash)
	case m.flash != "":
		left += st.status.Render(" " + m.flash)
	default:
		left += st.statusMuted.Render(" " + config.ShortenPath(m.file.Path))
	}

	c := m.ed().Cursor()
	right := st.status.Render(fmt.Sprintf("Ln %d, Col %d", c.Row+1, c.Col+1)) +
		st.statusMuted.Render("  ·  ") +
		st.status.Render(fmt.Sprintf("note %d/%d ", m.active+1, len(m.tabs)))
	rw := lipgloss.Width(right)
	if rw+10 > m.width {
		right, rw = "", 0
	}
	lw := m.width - rw
	if rw > 0 {
		lw -= 2 // keep the two halves apart
		right = st.bar.Render("  ") + right
	}
	left = ansi.Truncate(left, lw, st.statusMuted.Render("…"))
	if gap := lw - lipgloss.Width(left); gap > 0 {
		left += st.bar.Render(strings.Repeat(" ", gap))
	}
	return left + right
}

// --- dialogs ---------------------------------------------------------------

// overlay draws box centred on base and remembers where it went.
func (m *Model) overlay(base, box string) string {
	w, h := lipgloss.Width(box), lipgloss.Height(box)
	x, y := max(0, (m.width-w)/2), max(0, (m.height-h)/2)
	m.dialogRect = [4]int{x, y, w, h}
	for i := range m.dialogHits {
		m.dialogHits[i].y += y
		m.dialogHits[i].x0 += x
		m.dialogHits[i].x1 += x
	}
	out := lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(box).X(x).Y(y).Z(1),
	).Render()
	return m.clipFrame(out)
}

// clipFrame forces a frame to exactly the terminal size, so a dialog bigger than
// a small terminal is cut off instead of pushing the layout around.
func (m *Model) clipFrame(frame string) string {
	lines := strings.Split(frame, "\n")
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	for len(lines) < m.height {
		lines = append(lines, "")
	}
	for i, l := range lines {
		lines[i] = fit(l, m.width)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) insideDialog(x, y int) bool {
	r := m.dialogRect
	return x >= r[0] && x < r[0]+r[2] && y >= r[1] && y < r[1]+r[3]
}

func (m *Model) viewConfirm() string {
	st := m.st
	title := "Untitled"
	if m.confirmIdx < len(m.tabs) {
		title = tabLabel(tabTitle(m.tabs[m.confirmIdx].ed.Text()), 36)
	}
	del, cancel := st.button, st.button
	if m.confirmFocus == 0 {
		del = st.dangerFocus
	} else {
		cancel = st.buttonFocus
	}
	delBtn, cancelBtn := del.Render("Delete"), cancel.Render("Cancel")
	gap := st.dialogText.Render("  ")
	lines := []string{
		st.dialogTitle.Render("Delete this note?"),
		"",
		st.dialogText.Render("“" + title + "” will be removed from"),
		st.dialogMuted.Render(filepath.Base(m.file.Path) + ". This can't be undone."),
		"",
		delBtn + gap + cancelBtn,
		"",
		st.dialogMuted.Render("y delete  ·  n / esc cancel"),
	}
	// Button hits relative to the box: border (1) + top padding (1), and
	// border (1) + left padding (3).
	const top, leftPad, buttonRow = 2, 4, 5
	dw := lipgloss.Width(delBtn)
	m.dialogHits = []hit{
		{top + buttonRow, leftPad, leftPad + dw, hitConfirm, 0},
		{top + buttonRow, leftPad + dw + 2, leftPad + dw + 2 + lipgloss.Width(cancelBtn), hitCancel, 0},
	}
	box := st.dialog.BorderForeground(col(st.theme.Danger))
	return box.Render(strings.Join(lines, "\n"))
}

var helpKeys = [][2]string{
	{"Ctrl+T", "new note"},
	{"Ctrl+W", "delete note"},
	{"Alt+← →  Ctrl+PgUp/PgDn", "switch note"},
	{"Alt+1…9", "jump to note"},
	{"Tab  Shift+Tab", "indent / dedent"},
	{"Shift+arrows", "select"},
	{"Ctrl+A", "select all"},
	{"Ctrl+C  Ctrl+X  Ctrl+V", "copy / cut / paste"},
	{"Ctrl+Z  Ctrl+Y", "undo / redo"},
	{"Ctrl+← →", "move by word"},
	{"F2  Shift+F2", "next / previous theme"},
	{"Ctrl+Q", "quit"},
	{"", ""},
	{"Mouse", "click tab, × to delete, + for new"},
	{"", "click, drag, double/triple-click to select"},
}

func (m *Model) viewHelp() string {
	st := m.st
	m.dialogHits = nil
	kw := 0
	for _, k := range helpKeys {
		kw = max(kw, lipgloss.Width(k[0]))
	}
	lines := []string{st.dialogTitle.Render("Keys"), ""}
	keyStyle := st.dialogText.Foreground(col(st.theme.Accent))
	for _, k := range helpKeys {
		lines = append(lines, keyStyle.Render(fmt.Sprintf("%-*s", kw, k[0]))+st.dialogText.Render("   ")+st.dialogText.Render(k[1]))
	}
	lines = append(lines, "", st.dialogMuted.Render("Notes save as you type. Any key closes this."))
	return st.dialog.Render(strings.Join(lines, "\n"))
}

// --- setup -----------------------------------------------------------------

const setupInputWidth = 56

func (m *Model) viewSetup() (string, *tea.Cursor) {
	st := m.st
	iw := min(setupInputWidth, max(10, m.width-8))
	m.input.SetSize(iw, 1)
	m.input.SetSize(iw, min(3, m.input.VisualLines())) // long paths wrap
	m.input.EnsureVisible()
	input := st.input.Width(iw + 4).Render(m.input.View(editor.Styles{Text: st.text, Selection: st.selection}))

	accent := lipgloss.NewStyle().Foreground(col(st.theme.Accent)).Bold(true)
	muted := lipgloss.NewStyle().Foreground(col(st.theme.Muted))
	text := lipgloss.NewStyle().Foreground(col(st.theme.Text))
	errLine := ""
	if m.setupErr != "" {
		errLine = lipgloss.NewStyle().Foreground(col(st.theme.Danger)).Render("✗ " + m.setupErr)
	}
	parts := []string{
		accent.Render("cool-note"),
		"",
		text.Render("Where should your notes be stored?"),
		muted.Render("All notes live in this one plain-text file. You can edit it in any editor."),
		"",
		input,
		errLine,
		muted.Render("Enter to confirm  ·  Esc to quit"),
	}
	card := lipgloss.JoinVertical(lipgloss.Left, parts...)
	cw, ch := lipgloss.Width(card), lipgloss.Height(card)
	x0, y0 := max(0, (m.width-cw)/2), max(0, (m.height-ch)/2)
	screen := lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top,
		lipgloss.NewStyle().MarginLeft(x0).MarginTop(y0).Render(card))

	const inputRow = 5 // index of the input in parts
	cx, cy, _ := m.input.CursorView()
	return screen, tea.NewCursor(x0+2+cx, y0+inputRow+1+cy)
}
