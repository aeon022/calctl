package tui

import (
	"fmt"
	"hash/fnv"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/aeon022/calctl/internal/models"
	"github.com/aeon022/missionctl-core/theme"
	"github.com/aeon022/missionctl-core/ui"
)

// ── Calendar colors ───────────────────────────────────────────────────────────

// calPalette are the dot colors calendars get: the theme's semantic colors
// plus ANSI magenta/cyan variants (never red — red means "needs attention").
var calPalette = []color.Color{
	theme.BlueV2, theme.GreenV2, theme.AmberV2,
	lipgloss.Color("13"), lipgloss.Color("14"), lipgloss.Color("5"), lipgloss.Color("6"), lipgloss.Color("10"),
}

// calColor maps a calendar name to a stable palette color (same name, same
// color on every run and in every view).
func calColor(name string) color.Color {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(name))))
	return calPalette[int(h.Sum32())%len(calPalette)]
}

func calDot(name string) string {
	if strings.TrimSpace(name) == "" {
		return lipgloss.NewStyle().Foreground(theme.SubtleV2).Render("●")
	}
	return lipgloss.NewStyle().Foreground(calColor(name)).Render("●")
}

// ── Week strip ────────────────────────────────────────────────────────────────

// dayCounts counts events per local calendar day ("2006-01-02").
func dayCounts(events []models.Event) map[string]int {
	m := make(map[string]int)
	for _, e := range events {
		m[e.StartTime.Format("2006-01-02")]++
	}
	return m
}

type stripSpan struct {
	day    time.Time
	x0, x1 int // [x0, x1) screen columns
}

// stripLayout is the rendered week strip plus its clickable regions.
type stripLayout struct {
	text         string
	days         []stripSpan
	prev0, prev1 int
	next0, next1 int
}

// weekStrip renders "◀ KW41  Mon 05  Tue 06 [Wed 07]  … ▶" as pills: the
// selected day is the active pill, days with events show their count, days
// without are dimmed and today is marked in amber. If the full form doesn't
// fit width it degrades: counts off → short names ("We 07") → numbers only.
func weekStrip(width int, ws, today, selected time.Time, counts map[string]int) stripLayout {
	var out stripLayout
	for level := 0; level <= 4; level++ {
		out = buildStrip(ws, today, selected, counts, level)
		if lipgloss.Width(out.text) <= width {
			return out
		}
	}
	return out
}

func buildStrip(ws, today, selected time.Time, counts map[string]int, level int) stripLayout {
	var l stripLayout
	arrow := lipgloss.NewStyle().Foreground(theme.BlueV2)
	var b strings.Builder
	x := 0
	write := func(s string) { b.WriteString(s); x += lipgloss.Width(s) }

	if level < 4 {
		write("  ")
	}
	l.prev0 = x
	write(arrow.Render("◀"))
	l.prev1 = x
	if level < 3 {
		_, kw := ws.ISOWeek()
		write(" " + lipgloss.NewStyle().Foreground(theme.MutedV2).Render(fmt.Sprintf("KW%02d", kw)) + "  ")
	} else {
		write(" ")
	}
	for i := 0; i < 7; i++ {
		d := ws.AddDate(0, 0, i)
		n := counts[d.Format("2006-01-02")]
		label := d.Format("Mon 02")
		switch level {
		case 1:
		case 2:
			label = d.Format("Mon")[:2] + d.Format(" 02")
		case 3, 4:
			label = d.Format("02")
		}
		if level == 0 && n > 0 {
			label += fmt.Sprintf(" ·%d", n)
		}
		var cell string
		switch {
		case level == 4: // last resort for very narrow terminals: bare numbers, no padding
			cell = lipgloss.NewStyle().Foreground(theme.SubtleV2).Render(label)
			if sameDay(d, selected) {
				cell = lipgloss.NewStyle().Bold(true).Foreground(theme.BlueV2).Render(label)
			}
		case sameDay(d, selected):
			cell = ui.Pill(label, ui.Info)
		case sameDay(d, today):
			cell = lipgloss.NewStyle().Bold(true).Foreground(theme.AmberV2).Padding(0, 1).Render(label)
		case n > 0:
			cell = lipgloss.NewStyle().Padding(0, 1).Render(label)
		default:
			cell = lipgloss.NewStyle().Foreground(theme.SubtleV2).Padding(0, 1).Render(label)
		}
		x0 := x
		write(cell)
		l.days = append(l.days, stripSpan{day: d, x0: x0, x1: x})
		if i < 6 {
			write(" ")
		}
	}
	if level < 4 {
		write("  ")
	}
	l.next0 = x
	write(arrow.Render("▶"))
	l.next1 = x
	l.text = b.String()
	return l
}

// ── Month mini-calendar ───────────────────────────────────────────────────────

// monthGrid renders month as a 7-column grid (weeks start Monday): a title,
// the weekday header and one line per week. Each cell is 3 wide: the day
// number plus a • when counts says the day has events; today is a filled
// block, the selected day bold-underlined. ponytail: counts only knows the
// loaded window (this week + daysAhead), so dots outside it are missing.
func monthGrid(month, today, selected time.Time, counts map[string]int) []string {
	first := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, month.Location())
	muted := lipgloss.NewStyle().Foreground(theme.MutedV2)
	dot := lipgloss.NewStyle().Foreground(theme.BlueV2).Render("•")
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(theme.MutedV2).Render(first.Format("January 2006")),
		muted.Render("Mo Tu We Th Fr Sa Su"),
	}
	lead := (int(first.Weekday()) + 6) % 7
	days := first.AddDate(0, 1, -1).Day()
	var cur strings.Builder
	cells := 0
	flush := func() {
		if cells == 0 {
			return
		}
		lines = append(lines, strings.TrimRight(cur.String(), " "))
		cur.Reset()
		cells = 0
	}
	for i := 0; i < lead; i++ {
		cur.WriteString("   ")
		cells++
	}
	for d := 1; d <= days; d++ {
		day := time.Date(first.Year(), first.Month(), d, 0, 0, 0, 0, first.Location())
		num := fmt.Sprintf("%2d", d)
		switch {
		case sameDay(day, today):
			num = lipgloss.NewStyle().Bold(true).Foreground(theme.OnAccentV2).Background(theme.BlueV2).Render(num)
		case sameDay(day, selected):
			num = lipgloss.NewStyle().Bold(true).Underline(true).Render(num)
		}
		mark := " "
		if counts[day.Format("2006-01-02")] > 0 {
			mark = dot
		}
		cur.WriteString(num + mark)
		cells++
		if cells == 7 {
			flush()
		}
	}
	flush()
	return lines
}

// ── Now line / past events ────────────────────────────────────────────────────

// nowIndex is where the "now" line goes among today's events (all-day first,
// then timed in start order): before the first timed event that starts after
// now, or at the end when every timed event has started. -1 when today has no
// timed events (all-day only or none) — then there is nothing to separate.
func nowIndex(dayEvents []models.Event, now time.Time) int {
	timed := false
	for i, e := range dayEvents {
		if e.AllDay {
			continue
		}
		timed = true
		if e.StartTime.After(now) {
			return i
		}
	}
	if !timed {
		return -1
	}
	return len(dayEvents)
}

// isPast reports whether e is over: it ended before now (all-day events
// count as over once their day has ended).
func isPast(e *models.Event, now time.Time) bool {
	if e.AllDay {
		return !e.EndTime.After(now) && !sameDay(e.StartTime, now)
	}
	return e.EndTime.Before(now)
}

// ── Side panel ────────────────────────────────────────────────────────────────

// composeSide stacks the detail lines (top, cut to fit) and the mini calendar
// (pinned to the bottom) into exactly h lines. When h is too small for both,
// the calendar is dropped and the details get the room.
func composeSide(detail, cal []string, h int) []string {
	if h < 1 {
		return nil
	}
	room := h - len(cal) - 1
	if room < 4 {
		cal, room = nil, h
	}
	out := make([]string, 0, h)
	for i := 0; i < room; i++ {
		l := ""
		if i < len(detail) {
			l = detail[i]
		}
		out = append(out, l)
	}
	if len(cal) > 0 {
		out = append(out, "")
		out = append(out, cal...)
	}
	return out
}
