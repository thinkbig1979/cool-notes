package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/thinkbig1979/cool-notes/internal/editor"
)

type action int

const (
	actNew action = iota
	actDelete
	actNext
	actCopy
	actCut
	actUndo
	actIndent
	actTheme
	actFind
	actHelp
	actQuit
)

// hint is one entry in the hotkey bar.
type hint struct {
	key, label string
	act        action
	prio       int  // lower survives longer when space runs out
	right      bool // pinned to the right edge
}

// hints returns the hotkey bar entries for the current state, in display
// order.
func (m *Model) hints() []hint {
	h := []hint{
		{"^T", "new", actNew, 0, false},
		{"^W", "delete", actDelete, 1, false},
		{"Alt+←→", "switch", actNext, 2, false},
		{"^F", "find", actFind, 3, false},
	}
	if m.ed().HasSelection() {
		h = append(h,
			hint{"^C", "copy", actCopy, 2, false},
			hint{"^X", "cut", actCut, 3, false},
		)
	}
	return append(h,
		hint{"^Z", "undo", actUndo, 5, false},
		hint{"Tab", "indent", actIndent, 6, false},
		hint{"F2", "theme", actTheme, 7, false},
		hint{"F1", "help", actHelp, 0, true},
		hint{"^Q", "quit", actQuit, 4, true},
	)
}

func hintWidth(h hint) int { return lipgloss.Width(h.key) + 2 + 1 + lipgloss.Width(h.label) }

const hintGap = 2

// viewHints draws the hotkey bar on row y and records a click target for
// each hint. Low-priority hints are dropped until the rest fit.
func (m *Model) viewHints(y int) string {
	st := m.st
	all := m.hints()
	keep := make([]bool, len(all))
	for i := range keep {
		keep[i] = true
	}
	width := func() int { // margins, hints, and a gap between neighbours
		w, n := 1+1, 0
		for i, h := range all {
			if keep[i] {
				w += hintWidth(h)
				n++
			}
		}
		return w + max(0, n-1)*hintGap
	}
	for width() > m.width {
		drop, worst := -1, -1
		for i, h := range all {
			if keep[i] && h.prio > worst && !(h.act == actHelp) {
				drop, worst = i, h.prio
			}
		}
		if drop < 0 {
			break
		}
		keep[drop] = false
	}

	var left, right []hint
	for i, h := range all {
		if !keep[i] {
			continue
		}
		if h.right {
			right = append(right, h)
		} else {
			left = append(left, h)
		}
	}

	var b strings.Builder
	x := 0
	gap := strings.Repeat(" ", hintGap)
	put := func(h hint) {
		x0 := x
		b.WriteString(st.hintKey.Render(" " + h.key + " "))
		b.WriteString(st.hintLabel.Render(" " + h.label))
		x += hintWidth(h)
		m.hits = append(m.hits, hit{y, x0, x, hitAction, int(h.act)})
	}
	b.WriteByte(' ')
	x++
	for _, h := range left {
		put(h)
		b.WriteString(gap)
		x += hintGap
	}
	rw := 0
	for _, h := range right {
		rw += hintWidth(h) + hintGap
	}
	if pad := m.width - x - rw + hintGap - 1; pad > 0 {
		b.WriteString(strings.Repeat(" ", pad))
		x += pad
	}
	for i, h := range right {
		if i > 0 {
			b.WriteString(gap)
			x += hintGap
		}
		put(h)
	}
	return fit(b.String(), m.width)
}

// runAction does what a clicked hint says.
func (m *Model) runAction(a action) tea.Cmd {
	if a != actUndo {
		m.replaced = nil
	}
	switch a {
	case actNew:
		return m.addTab()
	case actDelete:
		m.askClose(m.active)
	case actNext:
		m.switchTab(m.active + 1)
	case actCopy:
		return m.copy(false)
	case actCut:
		return m.copy(true)
	case actUndo:
		if m.deleted != nil {
			return m.restoreTab()
		}
		if m.replaced != nil {
			return m.undoReplaceAll()
		}
		return m.edit(func(e *editor.Editor) { e.Undo() })
	case actFind:
		m.openSearch()
	case actIndent:
		return m.edit(func(e *editor.Editor) { e.Indent() })
	case actTheme:
		return m.cycleTheme(1)
	case actHelp:
		m.modal = modalHelp
	case actQuit:
		return m.quit()
	}
	return nil
}
