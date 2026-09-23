package editor

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// A keystroke costs one edit plus one full redraw.
func benchKeystroke(b *testing.B, lines int, msg tea.KeyPressMsg) {
	var sb strings.Builder
	for i := range lines {
		fmt.Fprintf(&sb, "\tline %d with some words that wrap around at eighty columns maybe and then some more\n", i)
	}
	e := newEd(sb.String(), 100, 40)
	e.SetCursor(Pos{lines / 2, 5})
	st := Styles{Guides: true}
	b.ResetTimer()
	for b.Loop() {
		e.HandleKey(msg)
		e.View(st)
		e.CursorView()
	}
}

func BenchmarkTypeChar20k(b *testing.B) {
	benchKeystroke(b, 20000, tea.KeyPressMsg{Code: 'x', Text: "x"})
}

func BenchmarkEnter20k(b *testing.B) { benchKeystroke(b, 20000, key(tea.KeyEnter, 0)) }
