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
)

// ── View ──────────────────────────────────────────────────────────────────────

// assembleFrame wraps content with the header/week-nav/divider/status-bar
// frame every view shares — factored out so the help overlay's background
// can be built from the list content specifically (m.renderList()),
// regardless of which m.view is actually active.
func (m Model) assembleFrame(content string) string {
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n")
	b.WriteString(m.renderWeekNav())
	b.WriteString("\n")
	b.WriteString(styleDivider.Render(strings.Repeat("─", m.width)))
	b.WriteString("\n")
	b.WriteString(content)
	b.WriteString(m.renderStatusBar())
	return b.String()
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

func (m Model) renderWeekNav() string {
	ws := weekStart(m.weekOffset)
	today := startOfDay(time.Now())
	_, kw := ws.ISOWeek()
	eventDays := daysWithEvents(m.events)

	var days []string
	for i := 0; i < 7; i++ {
		d := ws.AddDate(0, 0, i)
		label := shortWeekday(d) + " " + fmt.Sprintf("%02d", d.Day())
		switch {
		case sameDay(d, today):
			days = append(days, styleKWDayToday.Render(label))
		case eventDays[d.Format("2006-01-02")]:
			days = append(days, styleKWDayEvent.Render(label))
		default:
			days = append(days, styleKWDay.Render(label))
		}
	}

	return " " + styleKWArrow.Render("◀") + "  " +
		styleKWLabel.Render(fmt.Sprintf("KW%02d", kw)) + "  " +
		strings.Join(days, "  ") + "  " +
		styleKWArrow.Render("▶")
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

func (m Model) renderHeader() string {
	left := styleHeader.Render("calctl") + styleStatusBar.Render(" · "+m.sectionLabel()) + "  " + time.Now().Format("Mon, Jan 02 2006")
	right := ""
	if m.submitting {
		right = m.sp.View() + styleLoading.Render(" saving…")
	} else if m.syncing {
		right = m.sp.View() + styleLoading.Render(" syncing…")
	} else if m.err != nil {
		right = styleError.Render("⚠ " + m.err.Error())
	} else if m.status != "" {
		right = styleOK.Render("✓ " + m.status)
	} else if !m.lastSynced.IsZero() {
		right = styleCal.Render("synced " + humanize.TimeAgo(m.lastSynced))
	}
	return statusbar.Line(m.width, left, right)
}

func (m Model) renderList() string {
	if m.loading {
		return emptystate.Loading(m.width, max(m.height-6, 1), m.sp.View(), "Loading events…")
	}

	var b strings.Builder
	b.WriteString("\n")

	contentHeight := m.height - 6
	if m.searching {
		b.WriteString("  / " + m.searchInput.View() + "\n\n")
		contentHeight -= 2
	} else if m.searchQ != "" {
		b.WriteString("  " + styleCal.Render("filter: /"+m.searchQ+"  (esc to clear)") + "\n\n")
		contentHeight -= 2
	}
	if m.inPalette {
		b.WriteString("  " + m.paletteInput.View() + "\n")
		matches := palette.Match(paletteCommands, m.paletteInput.Value())
		if len(matches) > 6 {
			matches = matches[:6]
		}
		if len(matches) == 0 {
			b.WriteString("    " + styleCal.Render("no matching command") + "\n")
		}
		for i, c := range matches {
			row := fmt.Sprintf("%-9s %s", c.Name, c.Desc)
			if i == m.paletteCursor {
				b.WriteString("    " + styleTitleSelected.Render("▶ "+row) + "\n")
			} else {
				b.WriteString("      " + styleCal.Render(row) + "\n")
			}
		}
		b.WriteString("\n")
		contentHeight -= 8
	}
	visibleRows := m.visibleRows(contentHeight)

	// visibleRows windows by row COUNT, but a header row costs 2 physical
	// lines (banner + divider) against a budget sized in row units — with
	// many single-event days (many headers packed close together), that
	// mismatch can render more physical lines than contentHeight actually
	// allows. Stop hard at the real line budget rather than trusting the
	// row-count window alone, so the palette/search bar above it can never
	// get pushed off screen by an under-budgeted tail of the list.
	linesUsed := 0
	for _, r := range visibleRows {
		if linesUsed >= contentHeight {
			break
		}
		if r.isHeader {
			b.WriteString("  " + styleDateBanner.Render(r.label) + "\n")
			b.WriteString("  " + styleDivider.Render(strings.Repeat("─", m.width-4)) + "\n")
			linesUsed += 2
			continue
		}
		linesUsed++

		e := r.event
		selected := m.rows[m.cursor] == r
		hovered := !selected && m.hoverRow >= 0 && m.hoverRow < len(m.rows) && m.rows[m.hoverRow] == r

		timeStr := styleTime.Render(e.StartTime.Format("15:04") + "–" + e.EndTime.Format("15:04"))
		if e.AllDay {
			timeStr = styleAllDay.Render("all day    ")
		}

		titleStyle := styleTitle
		switch {
		case selected:
			titleStyle = styleTitleSelected
		case hovered:
			titleStyle = styleTitleHover
		}

		calLabel := ""
		if e.Calendar != "" {
			calLabel = styleCal.Render("  [" + e.Calendar + "]")
		}

		// Independently-rendered segments concatenated side by side, not
		// nested inside one another — safe even though titleStyle carries a
		// background when selected: each segment (leading space, per-
		// character-highlighted title, trailing space) is self-contained.
		matchIdx := fuzzyMatchIndexes(m.searchQ, e.Title)
		titleRendered := titleStyle.Render(" ") + highlightMatches(humanize.Truncate(e.Title, m.width-30), matchIdx, titleStyle) + titleStyle.Render(" ")
		b.WriteString("  " + timeStr + " " + titleRendered + calLabel + "\n")
	}

	if len(visibleRows) == 0 {
		var title, hint string
		switch {
		case m.searchQ != "":
			title = "No events match your search"
		case len(m.rows) == 0:
			title, hint = "No events yet", "press s to sync from Apple Calendar, or calctl import to add one"
		}
		if title != "" {
			b.WriteString(emptystate.Render(m.width, max(contentHeight-1, 1), "", title, hint) + "\n")
		}
	}

	used := strings.Count(b.String(), "\n")
	for i := used; i < contentHeight; i++ {
		b.WriteString("\n")
	}
	return b.String()
}

// rowHitTest returns the m.rows index at screen row y, or -1 if the click
// landed on a section header, blank line, or outside the list. Mirrors the
// exact layout assembleFrame + renderList produce (header line, week nav
// line, divider line, renderList's own leading blank, an optional 2-line
// search/filter bar) plus visibleRows' scroll window, so a click lands on
// the event it visually appears to be over.
func (m Model) rowHitTest(y int) int {
	row := 4
	if m.searching || m.searchQ != "" {
		row += 2
	}
	if m.inPalette {
		row += 8
	}
	contentHeight := m.height - 6
	if m.searching || m.searchQ != "" {
		contentHeight -= 2
	}
	if m.inPalette {
		contentHeight -= 8
	}
	visible, start := m.visibleRowsWithStart(contentHeight)
	for i, r := range visible {
		if r.isHeader {
			if y >= row && y < row+2 {
				return -1
			}
			row += 2
			continue
		}
		if y == row {
			return start + i
		}
		row++
	}
	return -1
}

func (m Model) renderCreate() string {
	var b strings.Builder
	b.WriteString("\n")

	heading := "New Event"
	if m.editTarget != nil {
		heading = "Edit Event"
	}
	inner := strings.Builder{}
	inner.WriteString(styleHeader.Render(heading) + "\n\n")
	for i, inp := range m.inputs {
		label := formLabels[i]
		labelStyle := styleFormLabel
		if i == m.inputIdx {
			labelStyle = styleFormLabelActive
		}
		inner.WriteString(labelStyle.Render(label) + "  " + inp.View() + "\n")
	}

	b.WriteString(styleFormBox.Render(inner.String()))
	m.padToStatusBar(&b)
	return b.String()
}

func (m Model) renderDetail() string {
	if m.cursor >= len(m.rows) || m.rows[m.cursor].event == nil {
		return ""
	}
	e := m.rows[m.cursor].event

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(styleHeader.Render(e.Title) + "\n\n")
	b.WriteString(fmt.Sprintf("  Date      %s\n", e.StartTime.Format("Mon, Jan 02 2006")))
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
		b.WriteString("\n" + wordWrap(e.Notes, m.width-4) + "\n")
	}
	rendered := styleDetail.Render(b.String())
	var out strings.Builder
	out.WriteString(rendered)
	m.padToStatusBar(&out)
	return out.String()
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
	b.WriteString("\n  " + styleHeader.Render("Free Slots") + "\n\n")
	if len(slots) == 0 {
		b.WriteString(styleEmpty.Render("  No free slots found.") + "\n")
		m.padToStatusBar(&b)
		return b.String()
	}
	var lastDate string
	for _, sl := range slots {
		if sl.Date != lastDate {
			b.WriteString("  " + styleDateBanner.Render(sl.Start.Format("Mon, Jan 02")) + "\n")
			lastDate = sl.Date
		}
		b.WriteString(fmt.Sprintf("    %s – %s  (%s)\n",
			sl.Start.Format("15:04"),
			sl.End.Format("15:04"),
			models.FormatDuration(sl.Duration),
		))
	}
	m.padToStatusBar(&b)
	return b.String()
}

// padToStatusBar pins assembleFrame's trailing status bar to the bottom of
// the terminal instead of letting it glue itself right under a short view —
// pads with blank lines up to the same content budget renderList already
// uses (m.height - 6: header + week-nav + divider above, status bar below).
func (m Model) padToStatusBar(b *strings.Builder) {
	if m.height <= 0 {
		return
	}
	contentHeight := m.height - 6
	for used := strings.Count(b.String(), "\n"); used < contentHeight; used++ {
		b.WriteString("\n")
	}
}

func (m Model) renderStatusBar() string {
	hints := func(pairs ...[2]string) string { return statusbar.Hints(m.width, pairs...) }
	if m.view == viewCreate {
		return hints([2]string{"tab", "next field"}, [2]string{"enter", "next / save"},
			[2]string{"ctrl+s", "save"}, [2]string{"esc", "cancel"})
	}
	if m.view == viewDetail || m.view == viewFree {
		return hints([2]string{"esc", "back"}, [2]string{"q", "quit"})
	}
	if m.deleteTarget != nil {
		return styleDeleteConfirm.Render(
			fmt.Sprintf("  Delete %q?  ", m.deleteTarget.Title),
		) + statusbar.Hint("y", "confirm") + "  " + statusbar.Hint("any", "cancel")
	}
	return hints([2]string{"↑↓", "navigate"}, [2]string{"enter", "detail"}, [2]string{"n", "new"},
		[2]string{"e", "edit"}, [2]string{"d", "delete"}, [2]string{"s", "sync"}, [2]string{"/", "filter"},
		[2]string{"←→", "week"}, [2]string{"u", "undo"}, [2]string{"y", "copy"}, [2]string{"f", "free"},
		[2]string{"+/-", fmt.Sprintf("%dd", m.daysAhead)}, [2]string{"?", "help"}, [2]string{"q", "quit"})
}

func (m Model) visibleRows(height int) []row {
	rows, _ := m.visibleRowsWithStart(height)
	return rows
}

// visibleRowsWithStart is visibleRows plus the scroll-window start index,
// so callers that need to map a visible row back to its m.rows index
// (rowHitTest) don't have to duplicate the windowing math.
func (m Model) visibleRowsWithStart(height int) ([]row, int) {
	if len(m.rows) == 0 {
		return nil, 0
	}
	start := 0
	end := len(m.rows)
	if end-start > height {
		mid := m.cursor - height/2
		if mid < 0 {
			mid = 0
		}
		if mid+height > end {
			mid = end - height
		}
		start = mid
		end = start + height
	}
	return m.rows[start:end], start
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
	popW := min(70, m.width)
	if popW < 40 {
		popW = 40
	}

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
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBlue).
		Padding(1, 2).
		Width(m.helpPopW).
		Render(body)
}

func (m Model) renderCalendarPicker() string {
	var b strings.Builder
	b.WriteString(styleHeader.Render("Default Calendar") + "\n")
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
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBlue).
		Padding(1, 2).
		Width(min(50, m.width-4)).
		Render(b.String())
}
