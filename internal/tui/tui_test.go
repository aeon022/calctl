package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/aeon022/calctl/internal/models"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
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

func TestEventMatches_FuzzyMatchesTitle(t *testing.T) {
	e := &models.Event{Title: "budgetctl release"}
	if !eventMatches(e, "bgt") {
		t.Error("expected fuzzy 'bgt' to match 'budgetctl release'")
	}
	if eventMatches(e, "xyz") {
		t.Error("expected 'xyz' not to match 'budgetctl release'")
	}
}

func TestEventMatches_FallsBackToLocationCalendarNotes(t *testing.T) {
	e := &models.Event{Title: "unrelated", Location: "Graz Office", Calendar: "Work", Notes: "about budgetctl"}
	if !eventMatches(e, "graz") {
		t.Error("expected a location substring match")
	}
	if !eventMatches(e, "work") {
		t.Error("expected a calendar substring match")
	}
	if !eventMatches(e, "budgetctl") {
		t.Error("expected a notes substring match")
	}
}

func TestBuildRows_PreservesDayGroupingRatherThanRankingByMatchQuality(t *testing.T) {
	// Regression test: unlike habctl's filterHabits, buildRows must NOT
	// re-sort by fuzzy match quality — events are grouped by day with
	// interleaved header rows, and re-ranking would scatter a single
	// day's events and fragment that grouping.
	today := time.Now()
	day1 := time.Date(today.Year(), today.Month(), today.Day(), 9, 0, 0, 0, today.Location())
	day2 := day1.AddDate(0, 0, 1)
	events := []models.Event{
		{ID: "1", Title: "budgetctl release", StartTime: day1, EndTime: day1.Add(time.Hour)},
		{ID: "2", Title: "budget review", StartTime: day2, EndTime: day2.Add(time.Hour)},
	}
	rows := buildRows(events, 0, 7, "budget")
	var order []string
	for _, r := range rows {
		if r.isHeader {
			order = append(order, "HEADER")
		} else {
			order = append(order, r.event.Title)
		}
	}
	// Expect: header, event, header, event — two contiguous day groups in
	// chronological order, not events re-sorted across days by score.
	if len(order) != 4 || order[0] != "HEADER" || order[1] != "budgetctl release" ||
		order[2] != "HEADER" || order[3] != "budget review" {
		t.Errorf("expected chronological day-grouped order, got %+v", order)
	}
}

func TestHighlightMatches_ColorsOnlyMatchedRunes(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	idxs := fuzzyMatchIndexes("bgt", "budgetctl")
	if idxs == nil {
		t.Fatal("expected 'bgt' to fuzzy-match 'budgetctl'")
	}
	out := highlightMatches("budgetctl", idxs, styleTitle)
	if out == styleTitle.Render("budgetctl") {
		t.Error("expected highlightMatches to differ from a plain render for a real match")
	}
}

func TestHighlightMatches_NoMatchRendersPlain(t *testing.T) {
	out := highlightMatches("hello", nil, styleTitle)
	if out != styleTitle.Render("hello") {
		t.Errorf("expected nil idxs to render plain, got %q want %q", out, styleTitle.Render("hello"))
	}
}
