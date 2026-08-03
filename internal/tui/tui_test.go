package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/aeon022/calctl/internal/models"
	"github.com/aeon022/missionctl-core/palette"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestCommandPalette_TypeFilterAndExecute(t *testing.T) {
	m := New()
	m.width, m.height = 100, 30
	m.loading = false

	mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	m = mi.(Model)
	if !m.inPalette {
		t.Fatal("expected inPalette after ':'")
	}

	for _, r := range "syn" {
		mi, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = mi.(Model)
	}
	matches := palette.Match(paletteCommands, m.paletteInput.Value())
	if len(matches) == 0 || matches[0].Name != "sync" {
		t.Fatalf("expected 'sync' to be the top match for query %q, got %v", m.paletteInput.Value(), matches)
	}

	mi, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = mi.(Model)
	if m.inPalette {
		t.Error("expected palette to close after executing a command")
	}
	if !m.syncing {
		t.Error("expected 'sync' command to replay 's' and start syncing")
	}
}

func TestCommandPalette_EscCloses(t *testing.T) {
	m := New()
	m.width, m.height = 100, 30
	m.loading = false
	mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	m = mi.(Model)

	mi, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = mi.(Model)
	if m.inPalette {
		t.Error("expected esc to close the palette")
	}
}

// TestCommandPalette_ManyHeadersDoesNotOverflow is a regression test for a
// real bug caught via live tmux testing: visibleRows windows by row COUNT,
// but a header row costs 2 physical lines against that budget. With many
// single-event days (many headers packed close together) plus the
// palette's own 8 lines eating into the same budget, the rendered view
// exceeded the terminal height and pushed the palette's own input line off
// the top of the screen.
func TestCommandPalette_ManyHeadersDoesNotOverflow(t *testing.T) {
	m := New()
	m.width, m.height = 100, 30
	m.loading = false

	// 25 distinct days, one event each — worst case for the row/line
	// mismatch (every event row is preceded by its own 2-line header).
	for i := 0; i < 25; i++ {
		day := time.Date(2026, 8, 1+i, 0, 0, 0, 0, time.UTC)
		m.rows = append(m.rows,
			row{isHeader: true, label: day.Format("Mon, Jan 02")},
			row{event: &models.Event{ID: string(rune('a' + i)), Title: "Event", StartTime: day, EndTime: day}},
		)
	}

	mi, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	m = mi.(Model)

	lines := strings.Split(m.View(), "\n")
	if len(lines) > m.height {
		t.Errorf("rendered view is %d lines, exceeds terminal height %d — palette input would be pushed off screen", len(lines), m.height)
	}
	if !strings.Contains(m.View(), "command…") {
		t.Error("expected the palette input line to still be visible in the rendered view")
	}
}

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
	// buildRows(events, 0, 7, ...) windows this-week Monday..Sunday (7 days
	// from weekStart(0)) — anchor day1 to that Monday rather than "today",
	// so day1/day2 both stay inside the window regardless of which real
	// weekday the test happens to run on (using "today" broke every Sunday,
	// since day2 = "tomorrow" would fall in the next Mon-Sun week).
	day1 := weekStart(0).Add(9 * time.Hour)
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
