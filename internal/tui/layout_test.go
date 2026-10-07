package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/aeon022/calctl/internal/models"
	"github.com/aeon022/missionctl-core/tuitest"
	"github.com/charmbracelet/x/ansi"
)

func strip(s string) string { return ansi.Strip(s) }

func TestCalColorIsStableAndSpreadsCalendars(t *testing.T) {
	a, b := calColor("Arbeit"), calColor("  arbeit ")
	if fmt.Sprint(a) != fmt.Sprint(b) {
		t.Error("same calendar (case/space-insensitive) must always get the same color")
	}
	seen := map[string]bool{}
	for _, n := range []string{"Arbeit", "Privat", "Kalender", "Veranstaltungen", "Wanting & Gerwin", "Geburtstage", "Familie"} {
		seen[fmt.Sprint(calColor(n))] = true
	}
	if len(seen) < 4 {
		t.Errorf("seven calendars should spread over several palette colors, got %d", len(seen))
	}
	for _, c := range calPalette {
		if fmt.Sprint(c) == fmt.Sprint(lipgloss.Color("1")) || fmt.Sprint(c) == fmt.Sprint(lipgloss.Color("9")) {
			t.Error("red is reserved for urgency, not a calendar color")
		}
	}
	if strip(calDot("")) != "●" || strip(calDot("x")) != "●" {
		t.Error("dot glyph")
	}
}

func evAt(day time.Time, h, m, dur int, title string) models.Event {
	s := time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, day.Location())
	return models.Event{ID: title, Title: title, StartTime: s, EndTime: s.Add(time.Duration(dur) * time.Minute)}
}

func TestNowIndexBeforeBetweenAfterAndAllDay(t *testing.T) {
	day := time.Date(2026, 10, 7, 0, 0, 0, 0, time.Local)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)
	allDay := models.Event{Title: "Offsite", AllDay: true, StartTime: day, EndTime: day.AddDate(0, 0, 1)}
	morning, afternoon := evAt(day, 9, 0, 60, "Standup"), evAt(day, 15, 0, 60, "Review")

	cases := []struct {
		name   string
		events []models.Event
		want   int
	}{
		{"between past and upcoming", []models.Event{morning, afternoon}, 1},
		{"all upcoming → before the first timed one", []models.Event{afternoon}, 0},
		{"all started → at the end", []models.Event{morning}, 1},
		{"all-day events come first and are skipped", []models.Event{allDay, morning, afternoon}, 2},
		{"all-day only → nothing to separate", []models.Event{allDay}, -1},
		{"no events", nil, -1},
	}
	for _, c := range cases {
		if got := nowIndex(c.events, now); got != c.want {
			t.Errorf("%s: nowIndex = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestIsPast(t *testing.T) {
	day := time.Date(2026, 10, 7, 0, 0, 0, 0, time.Local)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)
	m, a := evAt(day, 9, 0, 60, "m"), evAt(day, 11, 30, 60, "running")
	if !isPast(&m, now) || isPast(&a, now) {
		t.Error("an event is past once it ended; a running one is not")
	}
	allToday := models.Event{AllDay: true, StartTime: day, EndTime: day.AddDate(0, 0, 1)}
	allYesterday := models.Event{AllDay: true, StartTime: day.AddDate(0, 0, -1), EndTime: day}
	if isPast(&allToday, now) || !isPast(&allYesterday, now) {
		t.Error("an all-day event is over only after its day")
	}
}

func rowDesc(rows []row) string {
	var out []string
	for _, r := range rows {
		switch {
		case r.event != nil:
			out = append(out, "E:"+r.event.Title)
		case r.kind == rowNow:
			out = append(out, "NOW")
		case r.kind == rowEmptyDay:
			out = append(out, "empty:"+r.day.Format("Mon"))
		default:
			out = append(out, fmt.Sprintf("day:%s/%d", r.day.Format("Mon"), r.count))
		}
	}
	return strings.Join(out, " ")
}

func TestBuildRowsCollapsesEmptyDaysAndPlacesNowLine(t *testing.T) {
	ws := weekStart(0)
	now := time.Date(ws.Year(), ws.Month(), ws.Day()+2, 12, 0, 0, 0, time.Local) // Wednesday noon
	evs := []models.Event{
		evAt(ws.AddDate(0, 0, 0), 10, 0, 30, "Mon1"),
		evAt(ws.AddDate(0, 0, 2), 9, 0, 60, "Past"),
		evAt(ws.AddDate(0, 0, 2), 15, 0, 60, "Later"),
	}
	rows := buildRowsAt(evs, 0, 4, "", now)
	want := "day:Mon/1 E:Mon1 empty:Tue day:Wed/2 E:Past NOW E:Later empty:Thu"
	if got := rowDesc(rows); got != want {
		t.Errorf("rows =\n %s\nwant\n %s", got, want)
	}
	if !strings.HasPrefix(rows[3].label, "TODAY — ") {
		t.Errorf("today's header is labelled TODAY: %q", rows[3].label)
	}
	for _, r := range rows {
		if r.kind == rowNow && r.label != "12:00" {
			t.Errorf("now line label = %q", r.label)
		}
		if r.isHeader == (r.event != nil) {
			t.Errorf("exactly header rows have no event: %+v", r)
		}
	}
	// a filter hides empty days and the now line stays only where today has matches
	if got := rowDesc(buildRowsAt(evs, 0, 4, "later", now)); got != "day:Wed/1 NOW E:Later" && got != "day:Wed/1 E:Later" {
		t.Errorf("filtered rows = %s", got)
	}
}

func TestMonthGridWeeksStartMondayLeapYearsAndDots(t *testing.T) {
	grid := func(month time.Time, today, sel time.Time, counts map[string]int) []string {
		var out []string
		for _, l := range monthGrid(month, today, sel, counts) {
			out = append(out, strip(l))
		}
		return out
	}
	far := time.Date(1999, 1, 1, 0, 0, 0, 0, time.Local)

	oct := grid(time.Date(2026, 10, 15, 0, 0, 0, 0, time.Local), far, far, map[string]int{"2026-10-08": 2, "2026-10-20": 1})
	if oct[0] != "October 2026" || oct[1] != "Mo Tu We Th Fr Sa Su" {
		t.Fatalf("title/header: %q / %q", oct[0], oct[1])
	}
	// 1 Oct 2026 is a Thursday: three blank cells, then " 1" under Th
	if oct[2] != "          1  2  3  4" || len(oct) != 2+5 {
		t.Errorf("first week = %q (lines %d)", oct[2], len(oct))
	}
	if !strings.Contains(oct[3], " 8•") || !strings.Contains(oct[5], "20•") {
		t.Errorf("event days carry a dot: %q / %q", oct[3], oct[5])
	}
	if strings.Contains(oct[2], "•") {
		t.Errorf("days without events have no dot: %q", oct[2])
	}

	feb24 := grid(time.Date(2024, 2, 1, 0, 0, 0, 0, time.Local), far, far, nil)
	if !strings.Contains(strings.Join(feb24, "\n"), "29") {
		t.Errorf("2024 is a leap year, Feb has 29 days:\n%s", strings.Join(feb24, "\n"))
	}
	feb26 := grid(time.Date(2026, 2, 1, 0, 0, 0, 0, time.Local), far, far, nil)
	if strings.Contains(strings.Join(feb26, "\n"), "29") {
		t.Error("Feb 2026 has 28 days")
	}
	if feb26[2] != "                   1" { // 1 Feb 2026 is a Sunday: six blanks
		t.Errorf("Sunday-start month: %q", feb26[2])
	}
	for _, l := range append(oct, feb24...) {
		if lipgloss.Width(l) > 21 {
			t.Errorf("grid line wider than 7 cells * 3: %q", l)
		}
	}
	// today and the selected day are styled (extra escape codes), others are not
	today := time.Date(2026, 10, 8, 0, 0, 0, 0, time.Local)
	styled := monthGrid(time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local), today, today, nil)[3]
	plain := monthGrid(time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local), far, far, nil)[3]
	if styled == plain {
		t.Error("today must be highlighted")
	}
}

func TestWeekStripCountsSelectedPillAndClickSpans(t *testing.T) {
	ws := time.Date(2026, 10, 5, 0, 0, 0, 0, time.Local) // a Monday
	today, sel := ws.AddDate(0, 0, 1), ws.AddDate(0, 0, 2)
	counts := map[string]int{"2026-10-06": 2, "2026-10-07": 1, "2026-10-09": 3}

	l := weekStrip(120, ws, today, sel, counts)
	text := strip(l.text)
	for _, want := range []string{"◀", "KW41", "Mon 05", "Tue 06 ·2", "Wed 07 ·1", "Fri 09 ·3", "▶"} {
		if !strings.Contains(text, want) {
			t.Errorf("strip lacks %q: %q", want, text)
		}
	}
	if strings.Contains(text, "Mon 05 ·") || strings.Contains(text, "Thu 08 ·") {
		t.Errorf("days without events show no count: %q", text)
	}
	if len(l.days) != 7 {
		t.Fatalf("7 clickable days, got %d", len(l.days))
	}
	prev := l.prev1
	for i, d := range l.days {
		if d.x0 < prev || d.x1 <= d.x0 {
			t.Errorf("day %d span [%d,%d) overlaps/empties (previous end %d)", i, d.x0, d.x1, prev)
		}
		prev = d.x1
		if got := []rune(text)[d.x0:d.x1]; !strings.Contains(string(got), ws.AddDate(0, 0, i).Format("02")) {
			t.Errorf("span of day %d covers %q, want its number", i, string(got))
		}
	}
	if l.prev1 > l.days[0].x0 || l.next0 < l.days[6].x1 {
		t.Error("arrows sit left and right of the days")
	}
	// the selected day is the only pill (differs from an unselected render)
	other := weekStrip(120, ws, today, ws, counts)
	if other.text == l.text {
		t.Error("moving the selection must change the strip")
	}
}

func TestWeekStripDegradesToFitEveryWidth(t *testing.T) {
	ws := time.Date(2026, 10, 5, 0, 0, 0, 0, time.Local)
	counts := map[string]int{"2026-10-06": 12, "2026-10-08": 3}
	prevWidth := 0
	for _, w := range []int{120, 100, 80, 70, 60, 50, 40, 32, 28} {
		l := weekStrip(w, ws, ws, ws.AddDate(0, 0, 1), counts)
		if got := lipgloss.Width(l.text); got > w {
			t.Errorf("width %d: strip is %d cells: %q", w, got, strip(l.text))
		}
		if got := lipgloss.Width(l.text); prevWidth != 0 && got > prevWidth {
			t.Errorf("a narrower terminal must not get a wider strip (%d → %d)", prevWidth, got)
		}
		prevWidth = lipgloss.Width(l.text)
		if len(l.days) != 7 {
			t.Errorf("width %d: %d days", w, len(l.days))
		}
	}
	if strings.Contains(strip(weekStrip(100, ws, ws, ws, counts).text), "·12") == false {
		t.Error("counts shown while there is room")
	}
}

func TestComposeSideExactHeightAndPinsCalendar(t *testing.T) {
	detail := []string{"title", "", "Time  09:00"}
	cal := []string{"October 2026", "Mo Tu", "1 2", "3 4"}
	out := composeSide(detail, cal, 14)
	if len(out) != 14 || out[0] != "title" || out[len(out)-1] != "3 4" || out[len(out)-4] != "October 2026" {
		t.Errorf("details on top, calendar pinned to the bottom, exact height:\n%q", out)
	}
	long := make([]string, 40)
	for i := range long {
		long[i] = fmt.Sprintf("line %d", i)
	}
	out = composeSide(long, cal, 14)
	if len(out) != 14 || out[len(out)-1] != "3 4" || strings.Contains(strings.Join(out, "|"), "line 20") {
		t.Errorf("long details are cut before the calendar: %q", out)
	}
	small := composeSide(detail, cal, 6)
	if len(small) != 6 || strings.Contains(strings.Join(small, "|"), "October") {
		t.Errorf("no room for both → details only: %q", small)
	}
	if composeSide(detail, cal, 0) != nil {
		t.Error("zero height")
	}
}

// ── Model level ───────────────────────────────────────────────────────────────

// every view/mode of the list screen, entered from a loaded model
var layoutStates = []struct {
	name string
	keys []string
}{
	{"list", nil},
	{"detail", []string{"1", "enter"}},
	{"form", []string{"n"}},
	{"free", []string{"f"}},
	{"help", []string{"?"}},
	{"search", []string{"/", "s"}},
	{"palette", []string{":"}},
	{"delete confirm", []string{"1", "d"}},
	{"filter active", []string{"/", "s", "enter"}},
	{"week +1", []string{"l"}},
}

func TestEveryStateFitsTheTerminalAtEveryWidth(t *testing.T) {
	for _, w := range []int{40, 60, 80, 100, 119, 120, 140, 170} {
		for _, h := range []int{15, 30, 40} {
			for _, st := range layoutStates {
				m, _ := tuitest.Send(loaded(t), tuitest.Resize(w, h))
				m, _ = tuitest.Keys(m, st.keys...)
				out := tuitest.Text(m)
				lines := strings.Split(out, "\n")
				if len(lines) != h-1 { // Update keeps one row of slack (bubbletea#304)
					t.Errorf("%dx%d %s: %d lines, want terminal height-1", w, h, st.name, len(lines))
				}
				for i, l := range lines {
					if lw := lipgloss.Width(l); lw > w {
						t.Fatalf("%dx%d %s: line %d is %d cells wide: %q", w, h, st.name, i, lw, l)
					}
				}
			}
		}
	}
}

func TestHeaderBlockShape(t *testing.T) {
	m, _ := tuitest.Send(loaded(t), tuitest.Resize(100, 30))
	lines := strings.Split(tuitest.Text(m), "\n")
	if !strings.HasPrefix(lines[0], "calctl · Events") || !strings.Contains(lines[0], "all calendars · KW") ||
		!strings.HasSuffix(strings.TrimRight(lines[0], " "), time.Now().Format("Mon 02 Jan")) {
		t.Errorf("header row: %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "────") || lines[2] != "" || lines[4] != "" {
		t.Errorf("rule, blank, strip, blank expected: %q %q %q", lines[1], lines[2], lines[4])
	}
	if !strings.Contains(lines[stripRow], "KW") || !strings.Contains(lines[stripRow], "◀") {
		t.Errorf("week strip row: %q", lines[stripRow])
	}
	m, _ = tuitest.Keys(m, "/", "s", "enter")
	if first := strings.Split(tuitest.Text(m), "\n")[0]; !strings.Contains(first, "filter: /s · KW") {
		t.Errorf("an active filter shows in the header scope: %q", first)
	}
}

func TestPanelsOnlyFromWideBreakpoint(t *testing.T) {
	for _, c := range []struct {
		w    int
		want bool
	}{{100, false}, {119, false}, {120, true}, {150, true}} {
		m, _ := tuitest.Send(loaded(t), tuitest.Resize(c.w, 32))
		out := tuitest.Text(m)
		if got := strings.Contains(out, "╭─ Agenda"); got != c.want {
			t.Errorf("width %d: Agenda panel = %v, want %v", c.w, got, c.want)
		}
		if got := strings.Contains(out, "October 20") || strings.Contains(out, "Mo Tu We Th Fr Sa Su"); got != c.want {
			t.Errorf("width %d: month mini-calendar = %v, want %v", c.w, got, c.want)
		}
	}
}

func TestSidePanelFollowsTheSelectedEventAndFallsBackToDaySummary(t *testing.T) {
	m, _ := tuitest.Send(loaded(t), tuitest.Resize(150, 36))
	m, _ = tuitest.Keys(m, "2") // Review: 14:00–15:00, Büro, Arbeit
	out := tuitest.Text(m)
	for _, want := range []string{"Review", "14:00–15:00 · 1h", "Arbeit", "Büro", "Location"} {
		if !strings.Contains(out, want) {
			t.Errorf("side panel lacks %q:\n%s", want, out)
		}
	}
	m, _ = tuitest.Keys(m, "3") // Offsite: all day, notes, attendees
	out = tuitest.Text(m)
	for _, want := range []string{"Offsite", "All day", "x@y.test", "Tickets mitbringen"} {
		if !strings.Contains(out, want) {
			t.Errorf("all-day event details lack %q:\n%s", want, out)
		}
	}
	empty, _ := tuitest.Send(loaded(t), tuitest.Resize(150, 36))
	e := empty.(Model)
	e.events, e.rows, e.cursor = nil, buildRows(nil, 0, 7, ""), 0
	if text := tuitest.Text(e); !strings.Contains(text, "Nothing planned") {
		t.Errorf("without a selected event the panel summarises the day:\n%s", text)
	}
}

func TestWideClickMappingAndRightPanelClicksIgnored(t *testing.T) {
	mi, _ := tuitest.Send(loaded(t), tuitest.Resize(150, 36))
	m := mi.(Model)
	x0, _, w, _ := m.listArea()
	lines := screenLines(m)
	for i, r := range m.rows {
		if r.event == nil {
			continue
		}
		y := -1
		for ly, l := range lines {
			if strings.Contains(l[:min(len(l), 80)], r.event.Title) {
				y = ly
				break
			}
		}
		if y < 0 {
			t.Fatalf("event %q not found in the agenda panel", r.event.Title)
		}
		if got := m.listHit(x0+1, y); got != i {
			t.Errorf("wide: click on %q (line %d) hit %d, want %d", r.event.Title, y, got, i)
		}
		if got := m.listHit(x0+w+5, y); got != -1 {
			t.Errorf("a click right of the agenda panel (x=%d) must hit nothing, got %d", x0+w+5, got)
		}
		if got := m.listHit(0, y); got != -1 {
			t.Errorf("a click on the panel border must hit nothing, got %d", got)
		}
	}
	start := m.cursor
	mi, _ = m.Update(tuitestClick(m.width-5, headerLines+3))
	if mi.(Model).cursor != start {
		t.Error("clicking inside the side panel must not move the cursor")
	}
}

func tuitestClick(x, y int) any { return tuitest.Click(x, y) }

func TestStripClicksJumpToDayAndChangeWeek(t *testing.T) {
	mi, _ := tuitest.Send(loaded(t), tuitest.Resize(110, 30))
	m := mi.(Model)
	l := m.stripLayout()
	thursday := l.days[3] // the all-day "Offsite"
	mi, cmd := m.Update(tuitest.Click(thursday.x0+1, stripRow))
	m = mi.(Model)
	if cmd != nil || curTitle(m) != "Offsite" {
		t.Errorf("clicking Thu jumps to its first event: cursor on %q", curTitle(m))
	}
	tuesday := l.days[1]
	mi, _ = m.Update(tuitest.Click(tuesday.x0+1, stripRow))
	if got := curTitle(mi.(Model)); got != "Standup" {
		t.Errorf("clicking Tue → %q, want Standup", got)
	}
	// an empty day leaves the cursor alone
	before := curTitle(mi.(Model))
	mi, _ = mi.Update(tuitest.Click(l.days[5].x0+1, stripRow))
	if got := curTitle(mi.(Model)); got != before {
		t.Errorf("an empty day must not move the cursor: %q → %q", before, got)
	}
	// the arrows flip the week and reload
	mi, cmd = mi.Update(tuitest.Click(l.next0, stripRow))
	if mi.(Model).weekOffset != 1 || cmd == nil {
		t.Errorf("▶ must go to the next week and reload: offset %d cmd %v", mi.(Model).weekOffset, cmd != nil)
	}
	mi, _ = mi.Update(tuitest.Click(l.prev0, stripRow))
	mi, _ = mi.Update(tuitest.Click(l.prev0, stripRow))
	if mi.(Model).weekOffset != -1 {
		t.Errorf("◀ twice from week 1 → %d, want -1", mi.(Model).weekOffset)
	}
}

func TestFooterRightShowsCountAndAmberSyncAgeOnlyWhenOld(t *testing.T) {
	mi, _ := tuitest.Send(loaded(t), tuitest.Resize(110, 30))
	m := mi.(Model)
	m.lastSynced = time.Now().Add(-2 * time.Hour)
	fresh := m.footerRight()
	m.lastSynced = time.Now().Add(-49 * time.Hour)
	old := m.footerRight()
	if !strings.Contains(strip(fresh), "synced 2h ago") || !strings.Contains(strip(old), "synced 2d ago") {
		t.Fatalf("sync age: %q / %q", strip(fresh), strip(old))
	}
	if !strings.Contains(strip(fresh), " on ") {
		t.Errorf("selected-day count missing: %q", strip(fresh))
	}
	amber := lipgloss.NewStyle().Foreground(colorAmber).Render("x")
	amberSeq := amber[:strings.Index(amber, "x")]
	if strings.Contains(fresh, amberSeq) || !strings.Contains(old, amberSeq) {
		t.Errorf("sync age is amber only when older than a day:\nfresh %q\nold %q", fresh, old)
	}
	m.err = fmt.Errorf("boom")
	if got := strip(m.footerRight()); !strings.Contains(got, "boom") || strings.Contains(got, "synced") {
		t.Errorf("an error replaces the counters: %q", got)
	}
	narrow, _ := tuitest.Send(loaded(t), tuitest.Resize(60, 30))
	if r := narrow.(Model).footerRight(); r != "" {
		t.Errorf("below 80 columns the hints get the whole line, right side = %q", strip(r))
	}
}

func TestEventRowsKeepColorsOnSelectionAndDimPastEvents(t *testing.T) {
	m := loaded(t)
	now := time.Now()
	day := startOfDay(now)
	pastEv := evAt(day, 0, 5, 10, "Early")
	pastEv.Calendar = "Arbeit"
	futureEv := evAt(day, 23, 40, 10, "Late")
	futureEv.Calendar = "Arbeit"
	if now.Hour() == 23 || now.Hour() == 0 && now.Minute() < 20 {
		t.Skip("too close to midnight for a past/future pair")
	}
	past := m.eventRow(80, &pastEv, false, false, now)
	fut := m.eventRow(80, &futureEv, false, false, now)
	if strings.ReplaceAll(past, "Early", "Late") == fut {
		t.Error("past events are drawn muted, upcoming ones are not")
	}
	sel := m.eventRow(80, &futureEv, true, false, now)
	if !strings.Contains(strip(sel), "▌") || lipgloss.Width(sel) != 80 {
		t.Errorf("selected row: accent bar and exact width: %q (%d)", strip(sel), lipgloss.Width(sel))
	}
	dot := calDot("Arbeit")
	if !strings.Contains(sel, dot) {
		t.Error("the calendar's dot keeps its own color inside the selection bar")
	}
	pill := m.eventRow(80, &models.Event{Title: "Offsite", AllDay: true, StartTime: day, EndTime: day.AddDate(0, 0, 1)}, false, false, now)
	if !strings.Contains(strip(pill), " all day ") {
		t.Errorf("all-day events show an 'all day' pill: %q", strip(pill))
	}
	if !strings.HasSuffix(strings.TrimRight(strip(m.eventRow(80, &futureEv, false, false, now)), " "), "Arbeit") {
		t.Error("the calendar name sits dimmed at the right end")
	}
}
