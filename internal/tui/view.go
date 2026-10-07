package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/aeon022/calctl/internal/calendar"
	"github.com/aeon022/calctl/internal/config"
	"github.com/aeon022/calctl/internal/models"
	"github.com/aeon022/calctl/internal/store"
	"github.com/aeon022/missionctl-core/emptystate"
	"github.com/aeon022/missionctl-core/humanize"
	"github.com/aeon022/missionctl-core/keymap"
	"github.com/aeon022/missionctl-core/overlay"
	"github.com/aeon022/missionctl-core/palette"
	"github.com/aeon022/missionctl-core/statusbar"
	"github.com/aeon022/missionctl-core/theme"
	"github.com/aeon022/missionctl-core/ui"
	"github.com/charmbracelet/x/ansi"
)

// ── View ──────────────────────────────────────────────────────────────────────

// assembleFrame wraps content with the header block (header, rule, week strip)
// and the one-line footer every view shares, always exactly m.height lines
// (ui.Frame pads or cuts the body) — factored out so the help overlay's
// background can be built from the list content specifically
// (m.renderList()), regardless of which m.view is actually active.
func (m Model) assembleFrame(content string) string {
	return ui.Frame(m.height, m.headerBlock(), strings.TrimRight(content, "\n"), m.renderStatusBar())
}

func (m Model) View() tea.View {
	v := tea.NewView(m.viewContent())
	// v2: AltScreen/MouseMode are per-View fields, not Program options.
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	v.ReportFocus = true // FocusMsg → reload stale events when the window regains focus
	return v
}

func (m Model) viewContent() string {
	if m.width == 0 {
		return "Loading..."
	}

	switch m.view {
	case viewCreate:
		return m.assembleFrame(m.renderCreate())
	case viewDetail:
		return m.assembleFrame(m.renderDetail())
	case viewFree:
		return m.assembleFrame(m.renderFree())
	case viewHelp:
		// "?" is only reachable from the main list, so the list is always
		// the correct background to keep visible behind the popup. No
		// enclosing border around the whole frame, so inset 0 is safe.
		bg := m.assembleFrame(m.renderList())
		return overlay.CenterDim(bg, m.renderHelpPopup(), m.width, m.height, 0)
	case viewCalendarPicker:
		bg := m.assembleFrame(m.renderList())
		return overlay.CenterDim(bg, m.renderCalendarPicker(), m.width, m.height, 0)
	default:
		return m.assembleFrame(m.renderList())
	}
}

// sectionLabel names the currently active view, shown in the header so
// it's always clear which screen is on-screen — the header itself stays
// in the same place and style across every view.
func (m Model) sectionLabel() string {
	switch m.view {
	case viewCreate:
		if m.editTarget != nil {
			return "Edit Event"
		}
		return "New Event"
	case viewDetail:
		return "Event Detail"
	case viewFree:
		return "Free Slots"
	case viewHelp:
		return "Help"
	default:
		return "Events"
	}
}

// dateFmt is the one date format used in the header, day banners and
// details: "Tue 06 Oct".
const dateFmt = "Mon 02 Jan"

// ── Geometry ──────────────────────────────────────────────────────────────────

const (
	headerLines = 5   // header, rule, blank, week strip, blank
	stripRow    = 3   // screen row of the week strip
	footerLines = 1   // the one-line status bar
	wideMin     = 120 // from this width the agenda gets a framed side panel
	paletteBar  = 8   // lines the ":" palette block always takes (padded)
)

func (m Model) wide() bool { return m.width >= wideMin }

// bodyHeight is the number of lines between the header block and the footer.
func (m Model) bodyHeight() int { return max(m.height-headerLines-footerLines, 1) }

// listPanelWidth is the outer width of the framed agenda panel (wide layout).
func (m Model) listPanelWidth() int { return max(m.width*11/20, 60) }

// listArea is where list lines are drawn: the first column and screen row,
// the width and the number of lines available. In the wide layout that is the
// inside of the framed Agenda panel (border + one cell of padding).
func (m Model) listArea() (x0, y0, w, h int) {
	if !m.wide() {
		return 0, headerLines, m.width, m.bodyHeight()
	}
	return 2, headerLines + 1, m.listPanelWidth() - 3, max(m.bodyHeight()-2, 1)
}

// barLines is how many lines the search/filter bar or palette take above the
// rows; renderers and hit-tests both go through it.
func (m Model) barLines() int {
	n := 0
	if m.searching || m.searchQ != "" {
		n += 2
	}
	if m.inPalette {
		n += paletteBar
	}
	return n
}

// ── Header block ──────────────────────────────────────────────────────────────

// selectedDay is the day the cursor is on (its event's day, or its header's);
// without rows it is today when in view, else the week's Monday.
func (m Model) selectedDay() time.Time {
	if m.cursor >= 0 && m.cursor < len(m.rows) {
		r := m.rows[m.cursor]
		if r.event != nil {
			return startOfDay(r.event.StartTime)
		}
		if !r.day.IsZero() {
			return r.day
		}
	}
	today, ws := startOfDay(time.Now()), weekStart(m.weekOffset)
	if !today.Before(ws) && today.Before(ws.AddDate(0, 0, 7)) {
		return today
	}
	return ws
}

func (m Model) stripLayout() stripLayout {
	return weekStrip(m.width, weekStart(m.weekOffset), startOfDay(time.Now()), m.selectedDay(), dayCounts(m.events))
}

// headerBlock is the 5-line top of every view: title row, rule, blank, the
// pill week strip, blank.
func (m Model) headerBlock() string {
	_, kw := weekStart(m.weekOffset).ISOWeek()
	scope := "all calendars"
	if m.searchQ != "" {
		scope = "filter: /" + m.searchQ
	}
	left := styleHeader.Render("calctl") + styleStatusBar.Render(" · "+m.sectionLabel())
	mid := styleStatusBar.Render(fmt.Sprintf("%s · KW%02d", scope, kw))
	head := ui.Header(m.width, left, mid, time.Now().Format(dateFmt))
	if m.view == viewCreate || m.view == viewDetail || m.view == viewFree {
		return strings.Join([]string{head, ui.Divider(m.width, ""), ""}, "\n")
	}
	return strings.Join([]string{head, ui.Divider(m.width, ""), "", m.stripLayout().text, ""}, "\n")
}

// bodyPanel frames a secondary view's content in a full-width titled panel
// that fills the body budget under the (strip-less) header.
func (m Model) bodyPanel(title, body string) string {
	return ui.Panel(m.width, max(m.height-4, 3), title, body, true)
}

// popup frames dialog content (help, pickers) in a titled panel sized to fit.
func popup(width int, title, body string) string {
	body = lipgloss.NewStyle().Width(max(width-4, 1)).Render(body)
	return ui.Panel(width, strings.Count(body, "\n")+3, title, body, true)
}

// ── List ──────────────────────────────────────────────────────────────────────

func (m Model) renderList() string {
	_, _, w, h := m.listArea()
	var content string
	if m.loading {
		content = emptystate.Loading(w, h, m.sp.View(), "Loading events…")
	} else {
		content = strings.Join(m.listLines(w, h), "\n")
	}
	if !m.wide() {
		return content
	}
	lw := m.listPanelWidth()
	rw := m.width - lw - 1
	left := ui.Panel(lw, m.bodyHeight(), "Agenda", content, true)
	right := ui.Panel(rw, m.bodyHeight(), m.sideTitle(), strings.Join(m.sideLines(rw-3, m.bodyHeight()-2), "\n"), false)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
}

// rowSpan is how many screen lines visible row i (index within the visible
// window) takes: events 1, the now line 1, other headers 1 plus a blank
// above unless first on screen.
func rowSpan(i int, r row) int {
	if r.isHeader && r.kind != rowNow && i > 0 {
		return 2
	}
	return 1
}

// listLines draws the search/filter bar or palette and the visible rows, at
// most h lines of width w.
func (m Model) listLines(w, h int) []string {
	var out []string
	cut := func(s string) string { return ansi.Truncate(s, w, "…") }
	if m.searching {
		out = append(out, cut("  / "+m.searchInput.View()), "")
	} else if m.searchQ != "" {
		out = append(out, cut("  "+styleCal.Render("filter: /"+m.searchQ+"  (esc to clear)")), "")
	}
	if m.inPalette {
		pal := []string{cut("  " + m.paletteInput.View())}
		matches := palette.Match(paletteCommands, m.paletteInput.Value())
		if len(matches) > 6 {
			matches = matches[:6]
		}
		if len(matches) == 0 {
			pal = append(pal, cut("    "+styleCal.Render("no matching command")))
		}
		for i, c := range matches {
			line := fmt.Sprintf("%-9s %s", c.Name, c.Desc)
			if i == m.paletteCursor {
				pal = append(pal, cut("    "+styleTitleSelected.Render("▶ "+line)))
			} else {
				pal = append(pal, cut("      "+styleCal.Render(line)))
			}
		}
		for len(pal) < paletteBar {
			pal = append(pal, "")
		}
		out = append(out, pal...)
	}
	room := h - len(out)
	if room <= 0 {
		return out
	}

	hasEvent := false
	for _, r := range m.rows {
		if r.event != nil {
			hasEvent = true
			break
		}
	}
	if !hasEvent && (m.searchQ != "" || len(m.events) == 0) {
		title, hint := "No events match your search", ""
		if m.searchQ == "" {
			title, hint = "No events yet", "press s to sync from Apple Calendar, or calctl import to add one"
		}
		return append(out, strings.Split(emptystate.Render(w, room, "", title, hint), "\n")...)
	}

	now := time.Now()
	visible, start := m.visibleRowsWithStart(room)
	used := 0
	for li, r := range visible {
		if used >= room {
			break
		}
		idx := start + li
		if r.isHeader {
			if r.kind != rowNow && li > 0 { // breathing room between days
				out = append(out, "")
				used++
			}
			out = append(out, cut("  "+m.headerRowText(r, w-4)))
			used++
			continue
		}
		out = append(out, m.eventRow(w, r.event, idx == m.cursor, idx != m.cursor && idx == m.hoverRow, now))
		used++
	}
	return out
}

// headerRowText is a non-event row: a day header with its count, a collapsed
// empty day, or the now line. Today's name is accented.
func (m Model) headerRowText(r row, w int) string {
	dim := lipgloss.NewStyle().Foreground(colorMuted)
	faint := lipgloss.NewStyle().Foreground(colorSubtle)
	switch r.kind {
	case rowNow:
		label := "── now " + r.label + " "
		return lipgloss.NewStyle().Foreground(colorBlue).Render(label + strings.Repeat("─", max(w-lipgloss.Width(label), 0)))
	case rowEmptyDay:
		name := dim.Render(r.day.Format("Mon 02"))
		if strings.HasPrefix(r.label, "TODAY — ") {
			name = lipgloss.NewStyle().Bold(true).Foreground(colorBlue).Render("TODAY") + " " + name
		}
		return name + faint.Render(" · nothing planned")
	}
	count := "1 event"
	if r.count != 1 {
		count = fmt.Sprintf("%d events", r.count)
	}
	if strings.HasPrefix(r.label, "TODAY — ") {
		return lipgloss.NewStyle().Bold(true).Foreground(colorBlue).Render("TODAY") + "  " +
			lipgloss.NewStyle().Bold(true).Render(strings.TrimPrefix(r.label, "TODAY — ")) + faint.Render("  · "+count)
	}
	return lipgloss.NewStyle().Bold(true).Foreground(colorMuted).Render(r.label) + faint.Render("  · "+count)
}

// eventRow draws one event on exactly w cells: dim time column (an "all day"
// pill for all-day events), the calendar's colored dot, the title (matched
// filter letters highlighted; muted once the event is over) and the calendar
// name dimmed on the right. Selection is a full-width bar, hover a gray one.
func (m Model) eventRow(w int, e *models.Event, selected, hovered bool, now time.Time) string {
	past := isPast(e, now)
	var timeCol string
	switch {
	case e.AllDay:
		timeCol = ui.Pill("all day", ui.Info) + "  "
	default:
		ts := e.StartTime.Format("15:04") + "–" + e.EndTime.Format("15:04")
		if past {
			timeCol = lipgloss.NewStyle().Foreground(colorSubtle).Render(ts)
		} else {
			timeCol = lipgloss.NewStyle().Foreground(colorAmber).Render(ts)
		}
	}
	titleStyle := styleTitle
	if past {
		titleStyle = styleCal
	}
	cal := ""
	if e.Calendar != "" {
		cal = styleCal.Render(ui.MidEllipsis(e.Calendar, 18))
	}
	title := highlightMatches(e.Title, fuzzyMatchIndexes(m.searchQ, e.Title), titleStyle)
	left := timeCol + " " + calDot(e.Calendar) + " " + title
	line := statusbar.Line(max(w-2, 1), left, cal)
	switch {
	case selected:
		return ui.Row(w, true, line)
	case hovered:
		return theme.HoverV2.Render(ansi.Strip(ui.Row(w, false, line)))
	}
	return ui.Row(w, false, line)
}

// rowHitTest returns the m.rows index at screen row y, or -1 if the click
// landed on a day header, blank gap, the now line or outside the list. It
// walks exactly the lines listLines draws (bar/palette, then the visible
// window with a blank above every header but the first on screen).
func (m Model) rowHitTest(y int) int {
	_, y0, _, h := m.listArea()
	row := y0 + m.barLines()
	visible, start := m.visibleRowsWithStart(h - m.barLines())
	used := 0
	for i, r := range visible {
		if used >= h-m.barLines() {
			break
		}
		span := rowSpan(i, r)
		if !r.isHeader && y == row {
			return start + i
		}
		row += span
		used += span
	}
	return -1
}

// listHit is rowHitTest for a click at column x: in the wide layout only the
// Agenda panel's inside counts.
func (m Model) listHit(x, y int) int {
	if m.wide() {
		x0, _, w, _ := m.listArea()
		if x < x0 || x >= x0+w {
			return -1
		}
	}
	return m.rowHitTest(y)
}

// ── Side panel (wide layout) ──────────────────────────────────────────────────

func (m Model) sideTitle() string {
	d := m.selectedDay()
	n := dayCounts(m.events)[d.Format("2006-01-02")]
	cnt := "no events"
	if n == 1 {
		cnt = "1 event"
	} else if n > 1 {
		cnt = fmt.Sprintf("%d events", n)
	}
	return d.Format(dateFmt) + " · " + cnt
}

// sideLines: the selected event's details (or the day's summary) on top and
// the month mini-calendar pinned to the bottom, in exactly h lines of width w.
func (m Model) sideLines(w, h int) []string {
	d := m.selectedDay()
	counts := dayCounts(m.events)
	var detail []string
	if e := m.selectedEvent(); e != nil {
		detail = m.eventDetailLines(e, w)
	} else {
		detail = m.daySummaryLines(d, counts)
	}
	cal := monthGrid(d, startOfDay(time.Now()), d, counts)
	return composeSide(detail, cal, h)
}

func (m Model) selectedEvent() *models.Event {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return m.rows[m.cursor].event
}

func (m Model) daySummaryLines(d time.Time, counts map[string]int) []string {
	dim := lipgloss.NewStyle().Foreground(colorMuted)
	n := counts[d.Format("2006-01-02")]
	if n == 0 {
		return []string{dim.Render("Nothing planned."), "", dim.Render("press n to add an event,"), dim.Render("f to find free slots")}
	}
	var busy time.Duration
	for _, e := range m.events {
		if sameDay(e.StartTime, d) && !e.AllDay {
			busy += e.EndTime.Sub(e.StartTime)
		}
	}
	lines := []string{dim.Render(fmt.Sprintf("%d events", n))}
	if busy > 0 {
		lines = append(lines, dim.Render("busy "+ui.Duration(busy)))
	}
	return lines
}

// eventDetailLines is the compact detail of one event for the side panel.
func (m Model) eventDetailLines(e *models.Event, w int) []string {
	dim := lipgloss.NewStyle().Foreground(colorMuted)
	var out []string
	for _, l := range strings.Split(ansi.Wrap(e.Title, w, ""), "\n") {
		out = append(out, styleHeader.Render(l))
	}
	out = append(out, "")
	kv := func(k, v string) {
		for i, l := range strings.Split(ansi.Wrap(v, max(w-11, 8), ""), "\n") {
			key := strings.Repeat(" ", 11)
			if i == 0 {
				key = dim.Render(fmt.Sprintf("%-11s", k))
			}
			out = append(out, key+l)
		}
	}
	if e.AllDay {
		kv("Time", "All day")
	} else {
		kv("Time", e.StartTime.Format("15:04")+"–"+e.EndTime.Format("15:04")+" · "+ui.Duration(e.EndTime.Sub(e.StartTime)))
	}
	if e.Timezone != "" {
		if loc, err := time.LoadLocation(e.Timezone); err == nil {
			orig, local := e.StartTime.In(loc).Format("15:04"), e.StartTime.Local().Format("15:04")
			if orig != local {
				kv("Timezone", fmt.Sprintf("%s (%s there, %s here)", e.Timezone, orig, local))
			}
		}
	}
	if e.Calendar != "" {
		kv("Calendar", calDot(e.Calendar)+" "+e.Calendar)
	}
	if e.Location != "" {
		kv("Location", e.Location)
	}
	if len(e.Attendees) > 0 {
		kv("Attendees", strings.Join(e.Attendees, ", "))
	}
	if e.Recurrence != "" {
		kv("Repeats", e.Recurrence)
	}
	if e.Notes != "" {
		out = append(out, "")
		out = append(out, strings.Split(ansi.Wrap(e.Notes, w, ""), "\n")...)
	}
	return out
}

func (m Model) renderCreate() string {
	title := "New Event"
	if m.editTarget != nil {
		title = "Edit Event"
	}
	var inner strings.Builder
	for i, inp := range m.inputs {
		inp.SetWidth(min(60, max(m.width-24, 8))) // border 2 + padding 2 + label 12 + gap 2 + slack
		labelStyle := styleFormLabel
		if i == m.inputIdx {
			labelStyle = styleFormLabelActive
		}
		inner.WriteString(" " + labelStyle.Render(formLabels[i]) + "  " + inp.View() + "\n")
	}
	return m.bodyPanel(title, "\n"+inner.String())
}

func (m Model) renderDetail() string {
	if m.cursor >= len(m.rows) || m.rows[m.cursor].event == nil {
		return ""
	}
	e := m.rows[m.cursor].event

	var b strings.Builder
	b.WriteString(styleHeader.Render(humanize.Truncate(e.Title, max(m.width-8, 4))) + "\n\n")
	b.WriteString(fmt.Sprintf("  Date      %s\n", e.StartTime.Format(dateFmt+" 2006")))
	if e.AllDay {
		b.WriteString("  Time      All day\n")
	} else {
		b.WriteString(fmt.Sprintf("  Time      %s – %s  (%s)\n",
			e.StartTime.Format("15:04"),
			e.EndTime.Format("15:04"),
			models.FormatDuration(e.EndTime.Sub(e.StartTime)),
		))
	}
	if e.Timezone != "" {
		if loc, err := time.LoadLocation(e.Timezone); err == nil {
			orig, local := e.StartTime.In(loc).Format("15:04"), e.StartTime.Local().Format("15:04")
			if orig != local {
				b.WriteString(fmt.Sprintf("  Timezone  %s (%s there, %s here)\n", e.Timezone, orig, local))
			}
		}
	}
	if e.Calendar != "" {
		b.WriteString(fmt.Sprintf("  Calendar  %s\n", e.Calendar))
	}
	if e.Location != "" {
		b.WriteString(fmt.Sprintf("  Location  %s\n", e.Location))
	}
	if len(e.Attendees) > 0 {
		b.WriteString(fmt.Sprintf("  Attendees %s\n", strings.Join(e.Attendees, ", ")))
	}
	if e.Notes != "" {
		b.WriteString("\n" + wordWrap(e.Notes, max(m.width-12, 10)) + "\n")
	}
	body := lipgloss.NewStyle().Width(max(m.width-4, 10)).Render(b.String())
	return m.bodyPanel("Event", body)
}

func (m Model) renderFree() string {
	s, err := store.New(config.DBPath(), config.Shared())
	if err != nil {
		return styleError.Render("Cannot open store: " + err.Error())
	}
	defer s.Close()

	ws := weekStart(m.weekOffset)
	to := ws.AddDate(0, 0, m.daysAhead)
	events, _ := s.ListEvents(context.Background(), ws, to)

	cfg := config.Active
	slots := calendar.FindFreeSlots(events, ws, to, calendar.WorkingHours{
		From: cfg.WorkingHoursFrom,
		To:   cfg.WorkingHoursTo,
	}, cfg.MinFreeSlot)

	var b strings.Builder
	b.WriteString("\n")
	if len(slots) == 0 {
		b.WriteString(styleEmpty.Render("  No free slots found.") + "\n")
		return m.bodyPanel("Free Slots", b.String())
	}
	var lastDate string
	for _, sl := range slots {
		if sl.Date != lastDate {
			b.WriteString("  " + styleDateBanner.Render(sl.Start.Format(dateFmt)) + "\n")
			lastDate = sl.Date
		}
		b.WriteString(fmt.Sprintf("    %s – %s  (%s)\n",
			sl.Start.Format("15:04"),
			sl.End.Format("15:04"),
			models.FormatDuration(sl.Duration),
		))
	}
	return m.bodyPanel("Free Slots", b.String())
}

// footerRight is the right side of the status bar: transient state (saving,
// syncing, error, confirmation) wins; otherwise the selected day's event count
// and the sync age, amber once the data is more than a day old.
func (m Model) footerRight() string {
	switch {
	case m.submitting:
		return m.sp.View() + styleLoading.Render(" saving…")
	case m.syncing:
		return m.sp.View() + styleLoading.Render(" syncing…")
	case m.err != nil:
		return styleError.Render("⚠ " + m.err.Error())
	case m.status != "":
		return styleOK.Render("✓ " + m.status)
	}
	if m.width < 80 { // narrow: the hints matter more than the counters
		return ""
	}
	var parts []string
	if m.view == viewList && len(m.rows) > 0 {
		n := dayCounts(m.events)[m.selectedDay().Format("2006-01-02")]
		parts = append(parts, styleCal.Render(fmt.Sprintf("%d on %s", n, m.selectedDay().Format("Mon 02"))))
	}
	if !m.lastSynced.IsZero() {
		age := lipgloss.NewStyle().Foreground(colorMuted)
		if time.Since(m.lastSynced) > 24*time.Hour {
			age = lipgloss.NewStyle().Foreground(colorAmber)
		}
		parts = append(parts, age.Render("synced "+humanize.TimeAgo(m.lastSynced)))
	}
	return strings.Join(parts, "  ")
}

func (m Model) renderStatusBar() string {
	right := m.footerRight()
	hints := func(pairs ...[2]string) string {
		return statusbar.Hints(max(m.width-lipgloss.Width(right)-2, 1), pairs...)
	}
	var left string
	switch {
	case m.view == viewCreate:
		left = hints([2]string{"tab", "next field"}, [2]string{"enter", "next / save"},
			[2]string{"ctrl+s", "save"}, [2]string{"esc", "cancel"})
	case m.view == viewDetail || m.view == viewFree:
		left = hints([2]string{"esc", "back"}, [2]string{"q", "quit"})
	case m.deleteTarget != nil:
		tail := statusbar.Hint("y", "confirm") + "  " + statusbar.Hint("any", "cancel")
		q := humanize.Truncate(fmt.Sprintf("  Delete %q?  ", m.deleteTarget.Title), max(m.width-lipgloss.Width(tail), 4))
		return ansi.Truncate(styleDeleteConfirm.Render(q)+tail, m.width, "")
	default:
		// "? help" and "q quit" sit early so they are dropped last on narrow terminals
		left = hints([2]string{"enter", "detail"}, [2]string{"n", "new"}, [2]string{"?", "help"}, [2]string{"q", "quit"},
			[2]string{"↑↓", "navigate"}, [2]string{"←→", "week"}, [2]string{"s", "sync"}, [2]string{"/", "filter"},
			[2]string{"e", "edit"}, [2]string{"d", "delete"}, [2]string{"u", "undo"}, [2]string{"y", "copy"},
			[2]string{"f", "free"}, [2]string{"+/-", fmt.Sprintf("%dd", m.daysAhead)})
	}
	return statusbar.Line(m.width, left, right)
}

func (m Model) visibleRows(height int) []row {
	rows, _ := m.visibleRowsWithStart(height)
	return rows
}

// visibleRowsWithStart is visibleRows plus the scroll-window start index,
// so callers that need to map a visible row back to its m.rows index
// (rowHitTest) don't have to duplicate the windowing math.
func (m Model) visibleRowsWithStart(height int) ([]row, int) {
	n := len(m.rows)
	if n == 0 {
		return nil, 0
	}
	height = max(height, 1)
	total := 0
	for i := range m.rows {
		total += m.rowLines(i)
	}
	if total <= height {
		return m.rows, 0
	}
	// grow a window around the cursor (below first, then above) within the
	// LINE budget, so the cursor row is always on screen
	cur := min(max(m.cursor, 0), n-1)
	start, end, used := cur, cur+1, m.rowLines(cur)
	for {
		grew := false
		if end < n && used+m.rowLines(end) <= height {
			used += m.rowLines(end)
			end++
			grew = true
		}
		if start > 0 && used+m.rowLines(start-1) <= height {
			start--
			used += m.rowLines(start)
			grew = true
		}
		if !grew {
			break
		}
	}
	return m.rows[start:end], start
}

// rowLines is the most screen lines row i can take: an event or the now line
// 1, any other header 1 plus the blank above it (budgeted for every row but
// the very first, which is conservative for the first row on screen).
func (m Model) rowLines(i int) int {
	r := m.rows[i]
	if r.isHeader && r.kind != rowNow && i > 0 {
		return 2
	}
	return 1
}

// advanceCursorPastHeader moves the cursor onto the first event row.
func (m *Model) advanceCursorPastHeader() {
	if m.cursor < len(m.rows) && !m.rows[m.cursor].isHeader {
		return
	}
	for i := m.cursor + 1; i < len(m.rows); i++ {
		if !m.rows[i].isHeader {
			m.cursor = i
			return
		}
	}
}

func (m Model) helpContent() string {
	return keymap.New("calctl", "calendar from the terminal").
		Section("Navigation").
		Row("j / ↓", "next event").
		Row("k / ↑", "previous event").
		Row("h / ←", "previous week").
		Row("l / →", "next week").
		Row("+ / -", "show more / fewer days (7–90)").
		Row("1-9", "jump to nth visible event").
		Section("Events").
		Row("enter", "event detail").
		Row("n", "new event").
		Row("e", "edit event").
		Row("d", "delete event (asks to confirm)").
		Row("u", "undo last delete").
		Row("y", "copy event title").
		Section("Views & Data").
		Row("/", "filter events (title, location, calendar, notes)").
		Row(":", "command palette — type an action by name").
		Row("esc", "clear active filter").
		Row("f", "free slots").
		Row("c", "set default calendar for new events").
		Row("s", "sync from Apple Calendar").
		Section("Other").
		Row("?", "toggle this help").
		Row("q", "quit").
		String()
}

// openHelp sizes and populates the transient help popup (see
// renderHelpPopup/overlay.Center) from the ACTUAL rendered background
// height, not the terminal size.
func (m Model) openHelp() Model {
	bg := m.assembleFrame(m.renderList())
	bgLines := strings.Split(bg, "\n")

	safeH := max(6, len(bgLines))
	popH := min(safeH, 22)
	popW := max(min(70, m.width), 12)

	vp := viewport.New(viewport.WithWidth(popW-6), viewport.WithHeight(popH-5)) // border 1+1, padding(1,2) → 2 rows/4 cols; -1 row for footer
	vp.SetContent(m.helpContent())

	m.helpVP = vp
	m.helpPopW = popW
	m.helpPopH = popH
	m.view = viewHelp
	return m
}

// renderHelpPopup renders the help viewport in a bordered box, meant to be
// composited over the list view via overlay.Center rather than replacing
// the whole screen — the list stays visible around it.
func (m Model) renderHelpPopup() string {
	footer := "esc / ?  close"
	if m.helpVP.TotalLineCount() > m.helpVP.Height() {
		footer = fmt.Sprintf("j/k scroll (%d%%)  ·  %s", int(m.helpVP.ScrollPercent()*100), footer)
	}
	body := m.helpVP.View() + "\n" + styleStatusBar.Render(footer)
	return ui.Panel(m.helpPopW, m.helpPopH, "Help", body, true)
}

func (m Model) renderCalendarPicker() string {
	var b strings.Builder
	b.WriteString(styleStatusBar.Render("New events use this calendar when --cal isn't given.") + "\n\n")

	if m.availableCalendars == nil {
		b.WriteString(styleStatusBar.Render("Loading…") + "\n")
	} else if len(m.availableCalendars) == 0 {
		b.WriteString(styleStatusBar.Render("No calendars found.") + "\n")
	}
	for i, name := range m.availableCalendars {
		line := name
		if name == config.Active.DefaultCalendar {
			line += "  (current)"
		}
		if i == m.calPickerCursor {
			b.WriteString(styleTitleSelected.Render("› "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	b.WriteString("\n" + styleStatusBar.Render("j/k move  enter set default  esc cancel"))
	return popup(max(min(50, m.width), 12), "Default Calendar", b.String())
}
