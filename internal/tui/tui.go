package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aeon022/calctl/internal/calendar"
	"github.com/aeon022/calctl/internal/config"
	"github.com/aeon022/calctl/internal/models"
	"github.com/aeon022/calctl/internal/store"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── Styles ────────────────────────────────────────────────────────────────────

var (
	styleApp = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15"))

	styleWeekNav = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	styleWeekNavArrow = lipgloss.NewStyle().
				Foreground(lipgloss.Color("12")).
				Bold(true)

	styleWeekDay = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7")).
			Padding(0, 1)

	styleWeekDayToday = lipgloss.NewStyle().
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("12")).
				Bold(true).
				Padding(0, 1)

	styleWeekDayHasEvent = lipgloss.NewStyle().
				Foreground(lipgloss.Color("14")).
				Padding(0, 1)

	styleDivider = lipgloss.NewStyle().
			Foreground(lipgloss.Color("236"))

	styleDateBanner = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("6"))

	styleDateBannerToday = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("12"))

	styleEventTime = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Width(14)

	styleEventTitle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15"))

	styleEventTitleSelected = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("12"))

	styleEventCal = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	styleEmpty = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true)

	styleStatusBar = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(0, 1)

	styleKey = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")).
			Bold(true)

	styleError = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9"))

	styleLoading = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11"))
)

// ── Messages ──────────────────────────────────────────────────────────────────

type eventsLoadedMsg struct{ events []models.Event }
type syncDoneMsg struct {
	events []models.Event
	err    error
}

// ── Model ─────────────────────────────────────────────────────────────────────

type view int

const (
	viewList view = iota
	viewDetail
	viewFree
)

type row struct {
	isHeader bool
	label    string
	date     time.Time
	event    *models.Event
}

type Model struct {
	events     []models.Event
	rows       []row
	cursor     int
	view       view
	weekOffset int
	syncing    bool
	err        error
	width      int
	height     int
}

func New() Model {
	return Model{}
}

func (m Model) Init() tea.Cmd {
	return loadWeekCmd(0)
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case eventsLoadedMsg:
		m.events = msg.events
		m.rows = buildRows(m.events, weekStart(m.weekOffset))
		if m.cursor >= eventRowCount(m.rows) {
			m.cursor = 0
		}

	case syncDoneMsg:
		m.syncing = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			m.events = msg.events
			m.rows = buildRows(m.events, weekStart(m.weekOffset))
		}

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// detail / free view: only back and quit
	if m.view == viewDetail || m.view == viewFree {
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc", "backspace", "shift+tab", "tab":
			m.view = viewList
		}
		return m, nil
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "up", "k", "shift+tab":
		m.moveCursor(-1)

	case "down", "j", "tab":
		m.moveCursor(+1)

	case "enter":
		if m.cursorEvent() != nil {
			m.view = viewDetail
		}

	case "left", "h":
		m.weekOffset--
		m.cursor = 0
		return m, loadWeekCmd(m.weekOffset)

	case "right", "l":
		m.weekOffset++
		m.cursor = 0
		return m, loadWeekCmd(m.weekOffset)

	case "s":
		if !m.syncing {
			m.syncing = true
			m.err = nil
			return m, syncWeekCmd(m.weekOffset)
		}

	case "f":
		m.view = viewFree
	}

	return m, nil
}

func (m *Model) moveCursor(delta int) {
	eventRows := eventRowIndices(m.rows)
	if len(eventRows) == 0 {
		return
	}
	// find current position in eventRows
	pos := 0
	for i, idx := range eventRows {
		if idx == m.cursor {
			pos = i
			break
		}
	}
	pos += delta
	if pos < 0 {
		pos = 0
	}
	if pos >= len(eventRows) {
		pos = len(eventRows) - 1
	}
	m.cursor = eventRows[pos]
}

func (m Model) cursorEvent() *models.Event {
	if m.cursor < len(m.rows) && !m.rows[m.cursor].isHeader {
		return m.rows[m.cursor].event
	}
	return nil
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}
	var b strings.Builder
	b.WriteString(m.renderWeekNav())
	b.WriteString("\n")
	b.WriteString(styleDivider.Render(strings.Repeat("─", m.width)))
	b.WriteString("\n")

	switch m.view {
	case viewDetail:
		b.WriteString(m.renderDetail())
	case viewFree:
		b.WriteString(m.renderFree())
	default:
		b.WriteString(m.renderList())
	}

	b.WriteString(styleDivider.Render(strings.Repeat("─", m.width)))
	b.WriteString("\n")
	b.WriteString(m.renderStatusBar())
	return b.String()
}

func (m Model) renderWeekNav() string {
	ws := weekStart(m.weekOffset)
	today := startOfDay(time.Now())

	// KW number
	_, kw := ws.ISOWeek()
	kwLabel := fmt.Sprintf("KW%02d", kw)

	// day pills
	var days []string
	eventDays := daysWithEvents(m.events)
	for i := 0; i < 7; i++ {
		d := ws.AddDate(0, 0, i)
		label := shortWeekday(d) + " " + fmt.Sprintf("%02d", d.Day())
		var s lipgloss.Style
		switch {
		case sameDay(d, today):
			s = styleWeekDayToday
		case eventDays[d.Format("2006-01-02")]:
			s = styleWeekDayHasEvent
		default:
			s = styleWeekDay
		}
		days = append(days, s.Render(label))
	}

	status := ""
	if m.syncing {
		status = "  " + styleLoading.Render("syncing…")
	} else if m.err != nil {
		status = "  " + styleError.Render("⚠ "+truncate(m.err.Error(), 30))
	}

	left := styleWeekNavArrow.Render("◀") + "  " +
		styleWeekNav.Render(kwLabel) + "  " +
		strings.Join(days, " ") + "  " +
		styleWeekNavArrow.Render("▶") +
		status

	return " " + left
}

func (m Model) renderList() string {
	var b strings.Builder
	b.WriteString("\n")

	listHeight := m.height - 6 // week nav + dividers + status + margins
	visible := m.visibleRows(listHeight)

	if len(visible) == 0 {
		b.WriteString(styleEmpty.Render("  No events — press s to sync") + "\n")
		return b.String()
	}

	for _, r := range visible {
		if r.isHeader {
			banner := r.label
			if sameDay(r.date, startOfDay(time.Now())) {
				b.WriteString("  " + styleDateBannerToday.Render(banner) + "\n")
			} else {
				b.WriteString("  " + styleDateBanner.Render(banner) + "\n")
			}
			continue
		}

		e := r.event
		if e == nil {
			continue
		}

		// placeholder for "(no events)"
		if e.Title == "" {
			b.WriteString(styleEmpty.Render("    (no events)") + "\n")
			continue
		}

		selected := m.rows[m.cursor].event != nil && m.rows[m.cursor].event == e

		timeStr := e.StartTime.Format("15:04") + "–" + e.EndTime.Format("15:04")
		if e.AllDay {
			timeStr = "all day    "
		}
		tStyle := styleEventTime.Render(timeStr)

		title := truncate(e.Title, m.width-32)
		titleRender := styleEventTitle.Render(" " + title + " ")
		if selected {
			titleRender = styleEventTitleSelected.Render(" " + title + " ")
		}

		calLabel := ""
		if e.Calendar != "" {
			calLabel = styleEventCal.Render(" [" + truncate(e.Calendar, 20) + "]")
		}

		b.WriteString("  " + tStyle + " " + titleRender + calLabel + "\n")
	}

	// fill remaining height
	used := strings.Count(b.String(), "\n")
	for used < listHeight {
		b.WriteString("\n")
		used++
	}
	return b.String()
}

func (m Model) renderDetail() string {
	e := m.cursorEvent()
	if e == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  " + lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Render(e.Title) + "\n\n")
	b.WriteString(fmt.Sprintf("  %-10s %s\n", "Date", e.StartTime.Format("Mon, Jan 02 2006")))
	if e.AllDay {
		b.WriteString(fmt.Sprintf("  %-10s All day\n", "Time"))
	} else {
		b.WriteString(fmt.Sprintf("  %-10s %s – %s  (%s)\n", "Time",
			e.StartTime.Format("15:04"), e.EndTime.Format("15:04"),
			fmtDur(e.EndTime.Sub(e.StartTime))))
	}
	if e.Calendar != "" {
		b.WriteString(fmt.Sprintf("  %-10s %s\n", "Calendar", e.Calendar))
	}
	if e.Location != "" {
		b.WriteString(fmt.Sprintf("  %-10s %s\n", "Location", e.Location))
	}
	if len(e.Attendees) > 0 {
		b.WriteString(fmt.Sprintf("  %-10s %s\n", "Attendees", strings.Join(e.Attendees, ", ")))
	}
	if e.Notes != "" {
		b.WriteString("\n" + wordWrap("  "+e.Notes, m.width-4) + "\n")
	}
	return b.String()
}

func (m Model) renderFree() string {
	s, err := store.New(config.DBPath())
	if err != nil {
		return styleError.Render("  Cannot open store: " + err.Error())
	}
	defer s.Close()

	ws := weekStart(m.weekOffset)
	we := ws.AddDate(0, 0, 7)
	events, _ := s.ListEvents(context.Background(), ws, we)

	cfg := config.Active
	slots := calendar.FindFreeSlots(events, ws, we, calendar.WorkingHours{
		From: cfg.WorkingHoursFrom,
		To:   cfg.WorkingHoursTo,
	}, cfg.MinFreeSlot)

	var b strings.Builder
	b.WriteString("\n  " + lipgloss.NewStyle().Bold(true).Render("Free Slots") + "\n\n")
	if len(slots) == 0 {
		b.WriteString(styleEmpty.Render("  No free slots found.") + "\n")
		return b.String()
	}
	var lastDate string
	for _, sl := range slots {
		if sl.Date != lastDate {
			b.WriteString("  " + styleDateBanner.Render(sl.Start.Format("Mon, Jan 02")) + "\n")
			lastDate = sl.Date
		}
		b.WriteString(fmt.Sprintf("    %s – %s  (%s)\n",
			sl.Start.Format("15:04"), sl.End.Format("15:04"), fmtDur(sl.Duration)))
	}
	return b.String()
}

func (m Model) renderStatusBar() string {
	if m.view == viewDetail || m.view == viewFree {
		return styleStatusBar.Render(
			styleKey.Render("esc") + " back  " +
				styleKey.Render("q") + " quit")
	}
	return styleStatusBar.Render(
		styleKey.Render("←→") + " week  " +
			styleKey.Render("tab") + "/" + styleKey.Render("shift+tab") + " navigate  " +
			styleKey.Render("enter") + " detail  " +
			styleKey.Render("s") + " sync  " +
			styleKey.Render("f") + " free  " +
			styleKey.Render("q") + " quit")
}

// visibleRows returns a window of rows fitting within height.
func (m Model) visibleRows(height int) []row {
	if len(m.rows) == 0 {
		return nil
	}
	start, end := 0, len(m.rows)
	if end > height {
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
	return m.rows[start:end]
}

// ── Commands ──────────────────────────────────────────────────────────────────

func loadWeekCmd(offset int) tea.Cmd {
	return func() tea.Msg {
		ws := weekStart(offset)
		we := ws.AddDate(0, 0, 7)
		s, err := store.New(config.DBPath())
		if err != nil {
			return eventsLoadedMsg{}
		}
		defer s.Close()
		events, _ := s.ListEvents(context.Background(), ws, we)
		return eventsLoadedMsg{events}
	}
}

func syncWeekCmd(offset int) tea.Cmd {
	return func() tea.Msg {
		ws := weekStart(offset)
		we := ws.AddDate(0, 0, 7)

		events, err := calendar.FetchEvents(ws, we)
		if err != nil {
			return syncDoneMsg{err: err}
		}

		s, err := store.New(config.DBPath())
		if err != nil {
			return syncDoneMsg{err: err}
		}
		defer s.Close()

		ctx := context.Background()
		_ = s.DeleteBySource(ctx, "apple", ws, we)
		for i := range events {
			_ = s.UpsertEvent(ctx, &events[i])
		}
		stored, err := s.ListEvents(ctx, ws, we)
		return syncDoneMsg{events: stored, err: err}
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// weekStart returns the Monday of the week at `offset` weeks from now.
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

func buildRows(events []models.Event, ws time.Time) []row {
	var rows []row
	today := startOfDay(time.Now())

	for d := 0; d < 7; d++ {
		day := ws.AddDate(0, 0, d)
		dayEnd := day.Add(24*time.Hour - time.Second)

		label := day.Format("Mon, Jan 02")
		if sameDay(day, today) {
			label = "TODAY — " + label
		}
		rows = append(rows, row{isHeader: true, label: label, date: day})

		var dayEvents []models.Event
		for _, e := range events {
			if !e.StartTime.Before(day) && !e.StartTime.After(dayEnd) {
				dayEvents = append(dayEvents, e)
			}
		}
		if len(dayEvents) == 0 {
			rows = append(rows, row{event: &models.Event{}}) // empty placeholder
		} else {
			for i := range dayEvents {
				e := dayEvents[i]
				rows = append(rows, row{event: &e, date: day})
			}
		}
	}
	return rows
}

// eventRowIndices returns indices of rows that are real events (not headers, not placeholders).
func eventRowIndices(rows []row) []int {
	var out []int
	for i, r := range rows {
		if !r.isHeader && r.event != nil && r.event.Title != "" {
			out = append(out, i)
		}
	}
	return out
}

func eventRowCount(rows []row) int {
	return len(eventRowIndices(rows))
}

func daysWithEvents(events []models.Event) map[string]bool {
	m := make(map[string]bool)
	for _, e := range events {
		m[e.StartTime.Format("2006-01-02")] = true
	}
	return m
}

var shortDays = map[time.Weekday]string{
	time.Monday:    "Mo",
	time.Tuesday:   "Di",
	time.Wednesday: "Mi",
	time.Thursday:  "Do",
	time.Friday:    "Fr",
	time.Saturday:  "Sa",
	time.Sunday:    "So",
}

func shortWeekday(t time.Time) string {
	if s, ok := shortDays[t.Weekday()]; ok {
		return s
	}
	return t.Format("Mo")
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func wordWrap(s string, width int) string {
	words := strings.Fields(s)
	var lines []string
	line := ""
	for _, w := range words {
		if len(line)+len(w)+1 > width {
			lines = append(lines, line)
			line = w
		} else {
			if line == "" {
				line = w
			} else {
				line += " " + w
			}
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func fmtDur(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 && m > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	if h > 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dm", m)
}

// Run starts the TUI.
func Run() error {
	p := tea.NewProgram(New(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
