package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"cool-note/internal/config"
	"cool-note/internal/store"
)

func newTestModel(t *testing.T, notes ...string) *Model {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if notes != nil {
		if err := os.WriteFile(path, []byte(store.Serialize(notes)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m, err := New(Options{ConfigDir: filepath.Join(dir, "cfg"), FileFlag: path})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return m
}

func press(m *Model, keys ...string) {
	for _, k := range keys {
		var msg tea.KeyPressMsg
		switch k {
		case "ctrl+t":
			msg = tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl}
		case "ctrl+w":
			msg = tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl}
		case "alt+left":
			msg = tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt}
		case "f2":
			msg = tea.KeyPressMsg{Code: tea.KeyF2}
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		default:
			for _, r := range k {
				m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
			}
			continue
		}
		m.Update(msg)
	}
}

func fileNotes(t *testing.T, m *Model) []string {
	t.Helper()
	data, err := os.ReadFile(m.file.Path)
	if err != nil {
		t.Fatal(err)
	}
	return store.Parse(string(data))
}

func TestNewFileStartsWithOneSavedNote(t *testing.T) {
	m := newTestModel(t)
	if len(m.tabs) != 1 {
		t.Fatalf("tabs = %d", len(m.tabs))
	}
	if got := fileNotes(t, m); !reflect.DeepEqual(got, []string{""}) {
		t.Fatalf("file = %q", got)
	}
}

func TestNewTabAppendsAndCloseDeletesAfterConfirm(t *testing.T) {
	m := newTestModel(t, "first")
	press(m, "ctrl+t", "second")
	m.Flush()
	if got := fileNotes(t, m); !reflect.DeepEqual(got, []string{"first", "second"}) {
		t.Fatalf("after add: %q", got)
	}
	press(m, "ctrl+w")
	if m.modal != modalConfirmDelete {
		t.Fatal("no confirmation shown")
	}
	press(m, "esc")
	if len(m.tabs) != 2 || m.modal != modalNone {
		t.Fatal("esc should cancel")
	}
	press(m, "ctrl+w", "enter") // Delete is focused by default
	m.Flush()
	if got := fileNotes(t, m); !reflect.DeepEqual(got, []string{"first"}) {
		t.Fatalf("after delete: %q", got)
	}
	if m.active != 0 {
		t.Fatalf("active = %d", m.active)
	}
	// Deleting the last note leaves one empty note.
	press(m, "ctrl+w", "y")
	m.Flush()
	if len(m.tabs) != 1 || m.ed().Text() != "" {
		t.Fatalf("tabs after deleting last: %d %q", len(m.tabs), m.ed().Text())
	}
}

func TestTypingInConfirmDialogDoesNotEdit(t *testing.T) {
	m := newTestModel(t, "keep")
	press(m, "ctrl+w", "zz")
	if m.ed().Text() != "keep" || m.modal != modalConfirmDelete {
		t.Fatalf("text %q modal %v", m.ed().Text(), m.modal)
	}
}

func TestExternalChangeReloadsWhenClean(t *testing.T) {
	m := newTestModel(t, "a", "b")
	os.WriteFile(m.file.Path, []byte(store.Serialize([]string{"a edited", "b", "c"})), 0o600)
	ext, err := m.file.CheckExternal()
	if err != nil || ext == nil {
		t.Fatalf("no change detected: %v", err)
	}
	m.applyExternal(ext)
	if got := m.texts(); !reflect.DeepEqual(got, []string{"a edited", "b", "c"}) {
		t.Fatalf("tabs = %q", got)
	}
}

func TestExternalChangeWhileDirtyKeepsBoth(t *testing.T) {
	m := newTestModel(t, "mine")
	press(m, "X")
	if !m.dirty() {
		t.Fatal("expected unsaved edits")
	}
	theirs := store.Serialize([]string{"theirs"})
	os.WriteFile(m.file.Path, []byte(theirs), 0o600)
	ext, _ := m.file.CheckExternal()
	m.applyExternal(ext)
	m.Flush()
	if got := fileNotes(t, m); !reflect.DeepEqual(got, []string{"Xmine"}) {
		t.Fatalf("file = %q", got)
	}
	backups, _ := filepath.Glob(m.file.Path + ".conflict-*.bak")
	if len(backups) != 1 {
		t.Fatalf("backups = %v", backups)
	}
	if data, _ := os.ReadFile(backups[0]); string(data) != theirs {
		t.Fatalf("backup = %q", data)
	}
}

func TestStateRestoredOnReopen(t *testing.T) {
	m := newTestModel(t, "one", "two\nsecond line")
	press(m, "alt+left") // wraps to the last note
	m.ed().SetCursor(m.ed().Cursor())
	m.Flush()
	m2, err := New(Options{ConfigDir: m.opts.ConfigDir, FileFlag: m.file.Path})
	if err != nil {
		t.Fatal(err)
	}
	if m2.active != 1 {
		t.Fatalf("active = %d", m2.active)
	}
}

func TestViewFitsEverySize(t *testing.T) {
	notes := make([]string, 15)
	for i := range notes {
		notes[i] = fmt.Sprintf("Note number %d with a long title\n\tbody", i)
	}
	for _, size := range [][2]int{{20, 5}, {40, 10}, {60, 20}, {80, 24}, {200, 50}} {
		for _, mod := range []modal{modalNone, modalConfirmDelete, modalHelp} {
			m := newTestModel(t, notes...)
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			m.active = 14
			m.modal = mod
			lines := strings.Split(m.View().Content, "\n")
			if len(lines) != size[1] {
				t.Errorf("%v modal %d: %d lines", size, mod, len(lines))
			}
			for i, l := range lines {
				if w := lipgloss.Width(l); w != size[0] {
					t.Errorf("%v modal %d: line %d width %d", size, mod, i, w)
					break
				}
			}
		}
	}
}

func TestTabBarOverflowKeepsActiveVisible(t *testing.T) {
	notes := make([]string, 15)
	for i := range notes {
		notes[i] = fmt.Sprintf("Tab %02d", i)
	}
	m := newTestModel(t, notes...)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m.switchTab(14)
	bar := m.viewTabBar()
	if !strings.Contains(bar, "Tab 14") || !strings.Contains(bar, "‹") {
		t.Fatalf("bar = %q", bar)
	}
	if strings.Contains(bar, "Tab 00") {
		t.Fatal("first tab should be scrolled off")
	}
	m.switchTab(0)
	bar = m.viewTabBar()
	if !strings.Contains(bar, "Tab 00") || !strings.Contains(bar, "›") {
		t.Fatalf("bar = %q", bar)
	}
	// Clicking the › arrow reveals the next hidden tab.
	for _, h := range m.hits {
		if h.kind == hitNext {
			m.Update(tea.MouseClickMsg{X: h.x0, Y: 0, Button: tea.MouseLeft})
		}
	}
	if m.active == 0 {
		t.Fatal("› did not switch tab")
	}
}

func TestThemeCyclePersists(t *testing.T) {
	m := newTestModel(t, "x")
	press(m, "f2")
	cfg, err := config.Load(m.opts.ConfigDir)
	if err != nil || cfg.Theme != "catppuccin-mocha" {
		t.Fatalf("config theme = %q, %v", cfg.Theme, err)
	}
	for range len(m.themes.names()) - 1 {
		press(m, "f2")
	}
	if cfg, _ := config.Load(m.opts.ConfigDir); cfg.Theme != "" {
		t.Fatalf("back to auto should clear theme, got %q", cfg.Theme)
	}
}

func TestCustomThemeExtendsBuiltin(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "themes"), 0o700)
	os.WriteFile(filepath.Join(dir, "themes", "mine.json"), []byte(`{"extends":"nord","accent":"#ff0000"}`), 0o600)
	os.WriteFile(filepath.Join(dir, "themes", "bad.json"), []byte(`{`), 0o600)
	set, problems := loadThemes(dir)
	th, ok := set.find("mine")
	if !ok || th.Accent != "#ff0000" || th.Bar != "#2e3440" {
		t.Fatalf("theme = %+v", th)
	}
	if len(problems) != 1 || !strings.HasPrefix(problems[0], "bad.json") {
		t.Fatalf("problems = %v", problems)
	}
}
