// Package ui is the cool-notes terminal interface.
package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/thinkbig1979/cool-notes/internal/config"
	"github.com/thinkbig1979/cool-notes/internal/editor"
	"github.com/thinkbig1979/cool-notes/internal/store"
)

const (
	saveDelay     = 300 * time.Millisecond
	pollInterval  = 500 * time.Millisecond
	multiClickGap = 400 * time.Millisecond
	flashFor      = 3 * time.Second
	wheelLines    = 3
)

type mode int

const (
	modeSetup mode = iota
	modeNotes
)

type modal int

const (
	modalNone modal = iota
	modalConfirmDelete
	modalConfirmReplace
	modalHelp
)

// Messages.
type (
	saveTickMsg struct{ gen uint64 }
	savedMsg    struct {
		gen uint64
		err error
	}
	pollTickMsg struct{}
	externalMsg struct {
		ext *store.External
		err error
	}
	flashDoneMsg struct{ id int }
	stateTickMsg struct{ gen uint64 }
)

type tab struct {
	ed *editor.Editor
}

// Options configure the app.
type Options struct {
	ConfigDir string
	Config    config.Config
	FileFlag  string // --file; skips first-run setup and isn't saved to config
}

// Model is the Bubble Tea model.
type Model struct {
	opts   Options
	mode   mode
	width  int // drawable area, inside the margin
	height int
	mx, my int // margin around the app, in cells

	themes   themeSet
	themeSel string // configured theme name
	darkBg   bool
	st       styles

	// setup
	input    *editor.Editor
	setupErr string

	// notes
	file      *store.File
	tabs      []*tab
	active    int
	tabOffset int
	hits      []hit

	modal        modal
	confirmIdx   int
	confirmFocus int // 0 = delete, 1 = cancel
	dialogHits   []hit
	dialogRect   [4]int // x, y, w, h

	clip string

	dragging   bool
	lastClick  time.Time
	lastClickX int
	lastClickY int
	clickCount int

	saveGen  uint64 // bumped by every change
	savedGen uint64 // gen of the last successful save
	saveSeq  uint64
	saveErr  error
	polling  bool

	stateKey string // last saved or scheduled session state
	stateGen uint64

	flash    string
	flashID  int
	flashErr bool

	deleted *deletedTab // last deleted note, restorable with Ctrl+Z until the next key
	// notes changed by the last replace all, undone together by Ctrl+Z until the next key
	replaced []*tab

	search *search // non-nil while the find bar is open
}

// deletedTab remembers a deleted note so it can be put back.
type deletedTab struct {
	t           *tab
	idx         int
	placeholder bool // deleting the last note added an empty one in its place
}

// New builds the model. If no notes file is known yet it starts in setup.
func New(opts Options) (*Model, error) {
	m := &Model{opts: opts, darkBg: true, width: 80, height: 24}
	var problems []string
	m.themes, problems = loadThemes(opts.ConfigDir)
	m.themeSel = opts.Config.Theme
	if m.themeSel == "" {
		m.themeSel = AutoTheme
	}
	m.applyTheme()

	path := opts.FileFlag
	if path == "" {
		path = opts.Config.File
	}
	if path == "" {
		m.mode = modeSetup
		m.input = editor.New()
		m.input.SingleLine = true
		m.input.SetText(config.ShortenPath(config.DefaultNotesPath()))
		m.input.MoveTo(editor.Pos{Col: 1 << 30}, false)
		return m, nil
	}
	if err := m.openNotes(path); err != nil {
		return nil, err
	}
	if len(problems) > 0 {
		m.flash, m.flashErr = "theme: "+problems[0], true
	}
	return m, nil
}

func (m *Model) applyTheme() {
	m.st = newStyles(m.themes.resolve(m.themeSel, m.darkBg))
}

// openNotes loads the notes file and restores where the user left off.
func (m *Model) openNotes(path string) error {
	f, err := store.Open(path)
	if err != nil {
		return err
	}
	notes, err := f.Load()
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	m.file = f
	m.mode = modeNotes
	m.tabs = nil
	for _, n := range notes {
		m.tabs = append(m.tabs, m.newTab(n))
	}
	if len(m.tabs) == 0 {
		m.tabs = append(m.tabs, m.newTab(""))
		if err := f.Save(0, m.texts()); err != nil {
			return fmt.Errorf("creating %s: %w", path, err)
		}
	}
	st := config.LoadState(m.opts.ConfigDir)
	if st.File == path {
		m.active = clampInt(st.Active, 0, len(m.tabs)-1)
		for i, c := range st.Cursors {
			if i < len(m.tabs) {
				m.tabs[i].ed.SetCursor(editor.Pos{Row: c.Row, Col: c.Col})
				m.tabs[i].ed.SetScroll(c.Scroll)
			}
		}
	}
	m.layout()
	return nil
}

func (m *Model) newTab(text string) *tab {
	ed := editor.New()
	ed.SetText(text)
	return &tab{ed: ed}
}

func (m *Model) ed() *editor.Editor { return m.tabs[m.active].ed }

func (m *Model) texts() []string {
	out := make([]string, len(m.tabs))
	for i, t := range m.tabs {
		out[i] = t.ed.Text()
	}
	return out
}

func (m *Model) state() config.State {
	s := config.State{File: m.file.Path, Active: m.active}
	for _, t := range m.tabs {
		c := t.ed.Cursor()
		s.Cursors = append(s.Cursors, config.Cursor{Row: c.Row, Col: c.Col, Scroll: t.ed.Scroll()})
	}
	return s
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tea.RequestBackgroundColor}
	if m.mode == modeNotes {
		cmds = append(cmds, m.startPolling())
	}
	if m.flash != "" {
		cmds = append(cmds, m.flashTimer())
	}
	return tea.Batch(cmds...)
}

// --- saving ----------------------------------------------------------------

// changed schedules a debounced save.
func (m *Model) changed() tea.Cmd {
	m.saveGen++
	gen := m.saveGen
	return tea.Tick(saveDelay, func(time.Time) tea.Msg { return saveTickMsg{gen} })
}

// saveNow saves immediately, for structural changes like adding a note.
func (m *Model) saveNow() tea.Cmd {
	m.saveGen++
	return m.saveCmd()
}

func (m *Model) dirty() bool { return m.savedGen != m.saveGen }

func (m *Model) saveCmd() tea.Cmd {
	m.saveSeq++
	seq, gen := m.saveSeq, m.saveGen
	notes, st, file, dir := m.texts(), m.state(), m.file, m.opts.ConfigDir
	return func() tea.Msg {
		err := file.Save(seq, notes)
		if err == nil {
			config.SaveState(dir, st)
		}
		return savedMsg{gen: gen, err: err}
	}
}

// Flush saves synchronously. Called on quit.
func (m *Model) Flush() error {
	if m.mode != modeNotes || m.file == nil {
		return nil
	}
	m.saveSeq++
	if err := m.file.Save(m.saveSeq, m.texts()); err != nil {
		return err
	}
	m.savedGen = m.saveGen
	return config.SaveState(m.opts.ConfigDir, m.state())
}

func (m *Model) startPolling() tea.Cmd {
	if m.polling {
		return nil
	}
	m.polling = true
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return pollTickMsg{} })
}

func (m *Model) checkExternal() tea.Cmd {
	file := m.file
	return func() tea.Msg {
		ext, err := file.CheckExternal()
		return externalMsg{ext, err}
	}
}

// applyExternal handles the notes file being changed by another program.
func (m *Model) applyExternal(ext *store.External) tea.Cmd {
	if m.dirty() {
		// Our unsaved edits win; keep the outside version so nothing is lost.
		name, err := m.file.Backup(ext.Raw)
		if err != nil {
			return m.setFlash("file changed on disk, and backing it up failed: "+err.Error(), true)
		}
		return tea.Batch(m.saveNow(), m.setFlash("file changed on disk; kept your edits, theirs saved to "+config.ShortenPath(name), true))
	}
	notes := ext.Notes
	if len(notes) == 0 {
		notes = []string{""}
	}
	for i, n := range notes {
		if i < len(m.tabs) {
			m.tabs[i].ed.ReplaceText(n)
		} else {
			t := m.newTab(n)
			m.tabs = append(m.tabs, t)
		}
	}
	m.tabs = m.tabs[:len(notes)]
	m.active = clampInt(m.active, 0, len(m.tabs)-1)
	m.modal = modalNone
	m.deleted = nil
	m.replaced = nil
	m.search = nil
	m.layout()
	return m.setFlash("reloaded: file changed on disk", false)
}

func (m *Model) setFlash(s string, isErr bool) tea.Cmd {
	m.flash, m.flashErr = s, isErr
	return m.flashTimer()
}

func (m *Model) flashTimer() tea.Cmd {
	m.flashID++
	id := m.flashID
	return tea.Tick(flashFor, func(time.Time) tea.Msg { return flashDoneMsg{id} })
}

// --- tabs ------------------------------------------------------------------

func (m *Model) addTab() tea.Cmd {
	m.tabs = append(m.tabs, m.newTab(""))
	m.active = len(m.tabs) - 1
	m.layout()
	return m.saveNow()
}

func (m *Model) askClose(i int) {
	m.modal = modalConfirmDelete
	m.confirmIdx = i
	m.confirmFocus = 0
}

func (m *Model) closeTab(i int) tea.Cmd {
	m.modal = modalNone
	if i < 0 || i >= len(m.tabs) {
		return nil
	}
	title := tabTitle(m.tabs[i].ed.Text())
	m.deleted = &deletedTab{t: m.tabs[i], idx: i}
	m.replaced = nil
	m.tabs = append(m.tabs[:i], m.tabs[i+1:]...)
	if len(m.tabs) == 0 {
		m.tabs = append(m.tabs, m.newTab(""))
		m.deleted.placeholder = true
	}
	if m.active > i || m.active >= len(m.tabs) {
		m.active--
	}
	m.active = clampInt(m.active, 0, len(m.tabs)-1)
	m.layout()
	return tea.Batch(m.saveNow(), m.setFlash("deleted “"+title+"”  ·  Ctrl+Z to restore", false))
}

// restoreTab puts back the last deleted note where it was.
func (m *Model) restoreTab() tea.Cmd {
	d := m.deleted
	m.deleted = nil
	if d.placeholder && len(m.tabs) == 1 && m.tabs[0].ed.Text() == "" {
		m.tabs = m.tabs[:0]
	}
	i := clampInt(d.idx, 0, len(m.tabs))
	m.tabs = append(m.tabs[:i], append([]*tab{d.t}, m.tabs[i:]...)...)
	m.active = i
	m.layout()
	return tea.Batch(m.saveNow(), m.setFlash("restored “"+tabTitle(d.t.ed.Text())+"”", false))
}

// moveTab moves the active note one place left (-1) or right (+1).
func (m *Model) moveTab(delta int) tea.Cmd {
	j := m.active + delta
	if j < 0 || j >= len(m.tabs) {
		return nil
	}
	m.tabs[m.active], m.tabs[j] = m.tabs[j], m.tabs[m.active]
	m.active = j
	m.layout()
	return m.saveNow()
}

func (m *Model) switchTab(i int) {
	if len(m.tabs) == 0 {
		return
	}
	m.active = (i%len(m.tabs) + len(m.tabs)) % len(m.tabs)
	m.dragging = false
	m.layout()
}

func tabTitle(body string) string {
	if t := store.Title(body); t != "" {
		return t
	}
	return "Untitled"
}

// --- layout ----------------------------------------------------------------

const (
	tabBarRows = 2 // tab bar plus a spacer line
	statusRows = 2 // hotkey bar plus status line
)

// resize fits the app to the terminal, keeping a side margin so the tab bar
// and status line don't touch the edges (e.g. a tmux pane border). There is no
// top or bottom margin, to keep the rows for notes. Small terminals get no
// margin.
func (m *Model) resize(w, h int) {
	m.mx, m.my = 0, 0
	if w >= 40 && h >= 12 {
		m.mx = 2
	}
	m.width, m.height = max(1, w-2*m.mx), max(1, h-2*m.my)
	m.layout()
}

// editorRect is the editor's area on screen, inside the margin.
func (m *Model) editorRect() (x, y, w, h int) {
	padL := 3
	if m.width < 50 {
		padL = 1
	}
	return padL, tabBarRows, max(1, m.width-padL-1), max(1, m.height-tabBarRows-statusRows)
}

func (m *Model) layout() {
	if m.mode != modeNotes {
		return
	}
	_, _, w, h := m.editorRect()
	for _, t := range m.tabs {
		t.ed.SetSize(w, h)
	}
	m.ed().EnsureVisible()
}

// --- update ----------------------------------------------------------------

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Mouse coordinates arrive in terminal cells; make them relative to the
	// area inside the margin, which is what everything else works in.
	switch mm := msg.(type) {
	case tea.MouseClickMsg:
		mm.X, mm.Y = mm.X-m.mx, mm.Y-m.my
		msg = mm
	case tea.MouseReleaseMsg:
		mm.X, mm.Y = mm.X-m.mx, mm.Y-m.my
		msg = mm
	case tea.MouseMotionMsg:
		mm.X, mm.Y = mm.X-m.mx, mm.Y-m.my
		msg = mm
	case tea.MouseWheelMsg:
		mm.X, mm.Y = mm.X-m.mx, mm.Y-m.my
		msg = mm
	}
	_, cmd := m.update(msg)
	if m.mode == modeNotes {
		if sc := m.stateChanged(); sc != nil {
			cmd = tea.Batch(cmd, sc)
		}
	}
	return m, cmd
}

// stateChanged schedules a save of the session state (active note, cursor)
// when it changed, so switching notes survives the terminal being closed.
func (m *Model) stateChanged() tea.Cmd {
	c := m.ed().Cursor()
	key := fmt.Sprint(m.active, len(m.tabs), c.Row, c.Col, m.ed().Scroll())
	if key == m.stateKey {
		return nil
	}
	m.stateKey = key
	m.stateGen++
	gen := m.stateGen
	return tea.Tick(saveDelay, func(time.Time) tea.Msg { return stateTickMsg{gen} })
}

func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil
	case tea.BackgroundColorMsg:
		m.darkBg = msg.IsDark()
		m.applyTheme()
		return m, nil
	case flashDoneMsg:
		if msg.id == m.flashID {
			m.flash = ""
		}
		return m, nil
	}
	if m.mode == modeSetup {
		return m.updateSetup(msg)
	}

	switch msg := msg.(type) {
	case stateTickMsg:
		if msg.gen == m.stateGen {
			st, dir := m.state(), m.opts.ConfigDir
			return m, func() tea.Msg { config.SaveState(dir, st); return nil }
		}
	case saveTickMsg:
		if msg.gen == m.saveGen {
			return m, m.saveCmd()
		}
	case savedMsg:
		m.saveErr = msg.err
		if msg.err == nil && msg.gen > m.savedGen {
			m.savedGen = msg.gen
		}
	case pollTickMsg:
		m.polling = false
		return m, m.checkExternal()
	case externalMsg:
		cmds := []tea.Cmd{m.startPolling()}
		if msg.err == nil && msg.ext != nil {
			cmds = append(cmds, m.applyExternal(msg.ext))
		}
		return m, tea.Batch(cmds...)
	case tea.KeyPressMsg:
		return m, m.handleKey(msg)
	case tea.PasteMsg:
		m.deleted = nil
		m.replaced = nil
		if m.search != nil {
			m.search.query.InsertText(msg.Content)
			return m, m.searchUpdated()
		}
		if m.modal == modalNone {
			return m, m.edit(func(e *editor.Editor) { e.InsertText(msg.Content) })
		}
	case tea.MouseClickMsg:
		return m, m.handleClick(msg.Mouse())
	case tea.MouseMotionMsg:
		if m.dragging && m.modal == modalNone {
			mo := msg.Mouse()
			ex, ey, _, _ := m.editorRect()
			m.ed().Drag(mo.X-ex, mo.Y-ey)
		}
	case tea.MouseReleaseMsg:
		m.dragging = false
	case tea.MouseWheelMsg:
		return m, m.handleWheel(msg.Mouse())
	}
	return m, nil
}

// edit runs f on the active editor and schedules a save if the text changed.
func (m *Model) edit(f func(*editor.Editor)) tea.Cmd {
	e := m.ed()
	before := e.Version()
	f(e)
	if e.Version() != before {
		return m.changed()
	}
	return nil
}

func (m *Model) quit() tea.Cmd {
	if err := m.Flush(); err != nil {
		// Don't lose work: stay open and say why.
		return m.setFlash("could not save, not quitting: "+err.Error(), true)
	}
	return tea.Quit
}

func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	ks := msg.Keystroke()
	if ks == "ctrl+q" {
		return m.quit()
	}
	if ks == "ctrl+z" && m.deleted != nil && m.modal == modalNone && m.search == nil {
		return m.restoreTab()
	}
	if ks == "ctrl+z" && m.replaced != nil && m.modal == modalNone && m.search == nil {
		return m.undoReplaceAll()
	}
	m.deleted = nil
	m.replaced = nil
	if m.search != nil && m.modal == modalNone {
		return m.searchKey(msg)
	}
	switch m.modal {
	case modalHelp:
		m.modal = modalNone
		return nil
	case modalConfirmDelete:
		switch ks {
		case "y", "Y":
			return m.closeTab(m.confirmIdx)
		case "n", "N", "esc":
			m.modal = modalNone
		case "enter", "space":
			if m.confirmFocus == 0 {
				return m.closeTab(m.confirmIdx)
			}
			m.modal = modalNone
		case "left", "right", "tab", "shift+tab", "h", "l":
			m.confirmFocus = 1 - m.confirmFocus
		}
		return nil
	case modalConfirmReplace:
		switch ks {
		case "y", "Y":
			return m.doReplaceAll()
		case "n", "N", "esc":
			m.modal = modalNone
		case "enter", "space":
			if m.confirmFocus == 0 {
				return m.doReplaceAll()
			}
			m.modal = modalNone
		case "left", "right", "tab", "shift+tab", "h", "l":
			m.confirmFocus = 1 - m.confirmFocus
		}
		return nil
	}

	switch ks {
	case "ctrl+t", "ctrl+n":
		return m.addTab()
	case "ctrl+w":
		m.askClose(m.active)
	case "alt+right", "ctrl+pgdown", "ctrl+tab", "alt+l":
		m.switchTab(m.active + 1)
	case "alt+left", "ctrl+pgup", "ctrl+shift+tab", "alt+h":
		m.switchTab(m.active - 1)
	case "alt+1", "alt+2", "alt+3", "alt+4", "alt+5", "alt+6", "alt+7", "alt+8":
		if n := int(ks[len(ks)-1] - '1'); n < len(m.tabs) {
			m.switchTab(n)
		}
	case "alt+9":
		m.switchTab(len(m.tabs) - 1)
	case "alt+shift+left", "ctrl+shift+pgup":
		return m.moveTab(-1)
	case "alt+shift+right", "ctrl+shift+pgdown":
		return m.moveTab(1)
	case "ctrl+f":
		m.openSearch()
	case "ctrl+r":
		m.openReplace()
	case "f1":
		m.modal = modalHelp
	case "f2", "shift+f2":
		step := 1
		if ks == "shift+f2" {
			step = -1
		}
		return m.cycleTheme(step)
	case "ctrl+a":
		m.ed().SelectAll()
	case "ctrl+c":
		return m.copy(false)
	case "ctrl+x":
		return m.copy(true)
	case "ctrl+v":
		return m.edit(func(e *editor.Editor) { e.InsertText(m.clip) })
	case "esc":
		m.ed().ClearSelection()
	default:
		return m.edit(func(e *editor.Editor) { e.HandleKey(msg) })
	}
	return nil
}

func (m *Model) copy(cut bool) tea.Cmd {
	text := m.ed().SelectedText()
	if text == "" {
		return nil
	}
	m.clip = text
	cmds := []tea.Cmd{tea.SetClipboard(text)}
	if cut {
		cmds = append(cmds, m.edit(func(e *editor.Editor) { e.Backspace() }))
	}
	return tea.Batch(cmds...)
}

func (m *Model) cycleTheme(step int) tea.Cmd {
	m.themeSel = m.themes.next(m.themeSel, step)
	m.applyTheme()
	cfg := m.opts.Config
	cfg.Theme = m.themeSel
	if m.themeSel == AutoTheme {
		cfg.Theme = ""
	}
	m.opts.Config = cfg
	label := m.themeSel
	if label == AutoTheme {
		label = "auto (" + m.st.theme.Name + ")"
	}
	if err := config.Save(m.opts.ConfigDir, cfg); err != nil {
		return m.setFlash("theme "+label+" (not saved: "+err.Error()+")", true)
	}
	return m.setFlash("theme: "+label+"  ·  F2 next, Shift+F2 previous", false)
}

// --- mouse -----------------------------------------------------------------

type hitKind int

const (
	hitTab hitKind = iota
	hitClose
	hitNew
	hitPrev
	hitNext
	hitConfirm
	hitCancel
	hitAction // hotkey bar entry; idx is the action
)

// hit is a clickable region on one screen row.
type hit struct {
	y, x0, x1 int
	kind      hitKind
	idx       int
}

func findHit(hits []hit, x, y int) (hit, bool) {
	for _, h := range hits {
		if y == h.y && x >= h.x0 && x < h.x1 {
			return h, true
		}
	}
	return hit{}, false
}

func (m *Model) handleClick(mo tea.Mouse) tea.Cmd {
	switch m.modal {
	case modalHelp:
		m.modal = modalNone
		return nil
	case modalConfirmDelete, modalConfirmReplace:
		if h, ok := findHit(m.dialogHits, mo.X, mo.Y); ok {
			if h.kind == hitConfirm && m.modal == modalConfirmReplace {
				return m.doReplaceAll()
			}
			if h.kind == hitConfirm {
				return m.closeTab(m.confirmIdx)
			}
			m.modal = modalNone
		} else if !m.insideDialog(mo.X, mo.Y) {
			m.modal = modalNone
		}
		return nil
	}

	if h, ok := findHit(m.hits, mo.X, mo.Y); ok {
		if m.search != nil && h.kind != hitAction {
			m.search = nil
		}
		switch {
		case h.kind == hitAction:
			return m.runAction(action(h.idx))
		case h.kind == hitNew:
			return m.addTab()
		case h.kind == hitClose || (h.kind == hitTab && mo.Button == tea.MouseMiddle):
			m.askClose(h.idx)
		case h.kind == hitTab, h.kind == hitPrev, h.kind == hitNext:
			m.switchTab(h.idx)
		}
		return nil
	}

	ex, ey, ew, eh := m.editorRect()
	if mo.Button != tea.MouseLeft || mo.Y < ey || mo.Y >= ey+eh {
		return nil
	}
	m.search = nil
	x, y := min(mo.X-ex, ew), mo.Y-ey
	now := time.Now()
	if now.Sub(m.lastClick) < multiClickGap && mo.X == m.lastClickX && mo.Y == m.lastClickY {
		m.clickCount++
	} else {
		m.clickCount = 1
	}
	m.lastClick, m.lastClickX, m.lastClickY = now, mo.X, mo.Y
	switch m.clickCount {
	case 1:
		m.ed().Click(x, y, mo.Mod&tea.ModShift != 0)
		m.dragging = true
	case 2:
		m.ed().SelectWordAt(x, y)
	default:
		m.ed().SelectLineAt(x, y)
		m.clickCount = 0
	}
	return nil
}

func (m *Model) handleWheel(mo tea.Mouse) tea.Cmd {
	if m.modal != modalNone {
		return nil
	}
	up := mo.Button == tea.MouseWheelUp
	down := mo.Button == tea.MouseWheelDown
	if mo.Y == 0 {
		if up {
			m.switchTab(m.active - 1)
		} else if down {
			m.switchTab(m.active + 1)
		}
		return nil
	}
	if up {
		m.ed().ScrollBy(-wheelLines)
	} else if down {
		m.ed().ScrollBy(wheelLines)
	}
	return nil
}

// --- setup -----------------------------------------------------------------

func (m *Model) updateSetup(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.Keystroke() {
		case "ctrl+q", "ctrl+c", "esc":
			return m, tea.Quit
		case "enter":
			return m, m.finishSetup()
		case "tab", "shift+tab":
			return m, nil
		case "ctrl+a":
			m.input.SelectAll()
			return m, nil
		}
		m.setupErr = ""
		m.input.HandleKey(msg)
	case tea.PasteMsg:
		m.input.InsertText(msg.Content)
	}
	return m, nil
}

func (m *Model) finishSetup() tea.Cmd {
	path, err := config.ExpandPath(m.input.Text())
	if err == nil {
		err = m.openNotes(path)
	}
	if err == nil {
		cfg := m.opts.Config
		cfg.File = path
		m.opts.Config = cfg
		err = config.Save(m.opts.ConfigDir, cfg)
	}
	if err != nil {
		m.mode = modeSetup
		m.setupErr = err.Error()
		return nil
	}
	return tea.Batch(m.startPolling(), m.setFlash("notes file: "+config.ShortenPath(path), false))
}

func clampInt(v, lo, hi int) int { return max(lo, min(v, hi)) }

// tabLabel shortens a title for the tab bar.
func tabLabel(title string, maxRunes int) string {
	r := []rune(title)
	if len(r) <= maxRunes {
		return title
	}
	return strings.TrimRight(string(r[:maxRunes-1]), " ") + "…"
}
