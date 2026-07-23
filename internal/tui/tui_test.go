package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHelpOverlay_OpenScrollClose(t *testing.T) {
	m := New()
	m.width = 100
	m.height = 30
	m.loading = false

	mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = mi.(Model)
	if m.view != viewHelp {
		t.Fatalf("expected viewHelp after '?', got %v", m.view)
	}
	if m.helpVP.TotalLineCount() == 0 {
		t.Fatal("expected help content to be populated")
	}

	before := m.helpVP.ScrollPercent()
	for i := 0; i < 5; i++ {
		mi, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = mi.(Model)
	}
	if m.helpVP.ScrollPercent() <= before {
		t.Errorf("expected scroll to advance after pressing j, stayed at %v", before)
	}

	mi, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = mi.(Model)
	if m.view != viewList {
		t.Errorf("expected esc to close help back to viewList, got %v", m.view)
	}
}

func TestHelpOverlay_FitsWithinBackgroundHeight(t *testing.T) {
	m := New()
	m.width = 100
	m.height = 30
	m.loading = false
	m = m.openHelp()

	bg := m.assembleFrame(m.renderList())
	bgLines := len(strings.Split(bg, "\n"))
	if m.helpPopH > bgLines {
		t.Errorf("popup height %d exceeds background height %d", m.helpPopH, bgLines)
	}
}

func TestHelpOverlay_PopupBorderColumnIsConsistent(t *testing.T) {
	// Regression test: calctl's list view can render short lines (e.g. an
	// empty-state message) that are much narrower than the terminal width.
	// A background-padding bug in the shared overlay package let those
	// short lines throw the popup's left border out of alignment on
	// exactly that row.
	m := New()
	m.width = 100
	m.height = 30
	m.loading = false
	m = m.openHelp()

	out := m.View()
	lines := strings.Split(out, "\n")
	col := -1
	for i, l := range lines {
		idx := strings.IndexAny(l, "╭│╰")
		if idx < 0 {
			continue
		}
		if col == -1 {
			col = idx
			continue
		}
		if idx != col {
			t.Errorf("line %d: popup border at column %d, want %d (same as other rows)", i, idx, col)
		}
	}
	if col == -1 {
		t.Fatal("expected to find popup border characters in the rendered view")
	}
}
