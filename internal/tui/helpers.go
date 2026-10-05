package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/aeon022/calctl/internal/models"
	"github.com/sahilm/fuzzy"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

func buildRows(events []models.Event, weekOffset, daysAhead int, query string) []row {
	from := weekStart(weekOffset)
	today := startOfDay(time.Now())
	q := strings.ToLower(strings.TrimSpace(query))
	var rows []row

	for d := 0; d < daysAhead; d++ {
		day := from.AddDate(0, 0, d)
		dayEnd := day.Add(24*time.Hour - time.Second)

		var dayEvents []models.Event
		for _, e := range events {
			if e.StartTime.Before(day) || e.StartTime.After(dayEnd) {
				continue
			}
			if q != "" && !eventMatches(&e, q) {
				continue
			}
			dayEvents = append(dayEvents, e)
		}

		// while filtering, hide days without matches instead of stacking empty headers
		if q != "" && len(dayEvents) == 0 {
			continue
		}

		label := day.Format("Mon, Jan 02")
		if sameDay(day, today) {
			label = "TODAY — " + label
		}
		rows = append(rows, row{isHeader: true, label: label})

		if len(dayEvents) == 0 {
			rows = append(rows, row{event: &models.Event{Title: "(no events)"}})
		} else {
			for i := range dayEvents {
				e := dayEvents[i]
				rows = append(rows, row{event: &e})
			}
		}
	}
	return rows
}

// eventMatches reports whether q matches e — a fuzzy (subsequence) match on
// the title, or a plain substring match on location/calendar/notes for
// events the title fuzzy-match missed. Used only to decide inclusion;
// buildRows deliberately keeps events in their original day-grouped order
// rather than re-ranking by match quality (re-sorting would scatter a
// single day's events and fragment the "isHeader" day grouping).
func eventMatches(e *models.Event, q string) bool {
	if q != "" && len(fuzzy.Find(q, []string{e.Title})) > 0 {
		return true
	}
	return strings.Contains(strings.ToLower(e.Location), q) ||
		strings.Contains(strings.ToLower(e.Calendar), q) ||
		strings.Contains(strings.ToLower(e.Notes), q)
}

func removeByID(events []models.Event, id string) []models.Event {
	out := events[:0]
	for _, e := range events {
		if e.ID != id {
			out = append(out, e)
		}
	}
	return out
}

func weekStart(offset int) time.Time {
	now := startOfDay(time.Now())
	wd := int(now.Weekday())
	if wd == 0 {
		wd = 7
	}
	monday := now.AddDate(0, 0, -(wd - 1))
	return monday.AddDate(0, 0, offset*7)
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func daysWithEvents(events []models.Event) map[string]bool {
	m := make(map[string]bool)
	for _, e := range events {
		m[e.StartTime.Format("2006-01-02")] = true
	}
	return m
}

var shortDays = map[time.Weekday]string{
	time.Monday: "Mo", time.Tuesday: "Di", time.Wednesday: "Mi",
	time.Thursday: "Do", time.Friday: "Fr", time.Saturday: "Sa", time.Sunday: "So",
}

func shortWeekday(t time.Time) string {
	if s, ok := shortDays[t.Weekday()]; ok {
		return s
	}
	return t.Format("Mo")
}

// fuzzyMatchIndexes returns the rune indexes within s that q fuzzy-matched,
// or nil if q is empty or doesn't match at all.
func fuzzyMatchIndexes(q, s string) []int {
	if q == "" {
		return nil
	}
	matches := fuzzy.Find(q, []string{s})
	if len(matches) == 0 {
		return nil
	}
	return matches[0].MatchedIndexes
}

// highlightMatches renders s with the rune positions in idxs (from
// fuzzyMatchIndexes) styled via a warm, underlined variant of base, and
// every other character via base itself — fzf-style match highlighting.
//
// Renders one character at a time rather than nesting a highlighted span
// inside a single outer Render() call: lipgloss's Render() ends every
// string with a full SGR reset, so an inner Render() call's reset would
// wipe out the outer style for everything after the first highlighted
// character. Per-character rendering keeps every segment self-contained.
//
// idxs are indexes into s BEFORE any truncation — callers must resolve
// indexes against the same, untruncated string used to compute them.
func highlightMatches(s string, idxs []int, base lipgloss.Style) string {
	if len(idxs) == 0 {
		return base.Render(s)
	}
	hi := base.Foreground(colorAmber).Underline(true)
	matchSet := make(map[int]bool, len(idxs))
	for _, i := range idxs {
		matchSet[i] = true
	}
	var b strings.Builder
	for i, r := range []rune(s) {
		if matchSet[i] {
			b.WriteString(hi.Render(string(r)))
		} else {
			b.WriteString(base.Render(string(r)))
		}
	}
	return b.String()
}

func wordWrap(s string, width int) string {
	words := strings.Fields(s)
	var lines []string
	line := "  "
	for _, w := range words {
		if len(line)+len(w)+1 > width {
			lines = append(lines, line)
			line = "  " + w
		} else {
			if line == "  " {
				line += w
			} else {
				line += " " + w
			}
		}
	}
	if line != "  " {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// key renders a footer key-hint in the suite-wide "key:label" format —
// callers append the label text (no leading space) right after this call.
func key(k string) string {
	return styleStatusKey.Render(k + ":")
}

// motionThrottleFilter drops MouseMotionMsg messages arriving <16ms apart.
func motionThrottleFilter() func(tea.Model, tea.Msg) tea.Msg {
	var lastMotion time.Time
	return func(_ tea.Model, msg tea.Msg) tea.Msg {
		if _, ok := msg.(tea.MouseMotionMsg); !ok {
			return msg
		}
		now := time.Now()
		if now.Sub(lastMotion) < 16*time.Millisecond {
			return nil
		}
		lastMotion = now
		return msg
	}
}

// Run starts the TUI.
func Run() error {
	// WithFPS(30), not the 60 default: WithMouseAllMotion forces a full
	// re-render on every pixel of mouse movement, and 60fps of heavily
	// styled frames can outpace what the terminal can keep up with —
	// confirmed as the cause of a severe duplicate-content rendering bug
	// in notectl (same bubbletea setup). Halving the rate gives the
	// terminal breathing room.
	p := tea.NewProgram(New(), tea.WithFilter(motionThrottleFilter()), tea.WithFPS(30))
	_, err := p.Run()
	return err
}
