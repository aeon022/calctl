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
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
)

// ── Styles ────────────────────────────────────────────────────────────────────

var (
	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12")).
			Padding(0, 1)

	styleDateBanner = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("6"))

	styleDivider = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	styleTime = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Width(16)

	styleTitle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15"))

	styleTitleSelected = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("12"))

	styleCal = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	styleAllDay = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Width(16)

	styleEmpty = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true)

	styleStatusBar = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(0, 1)

	styleStatusKey = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")).
			Bold(true)

	styleError = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")).
			Padding(0, 1)

	styleLoading = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Padding(0, 1)

	styleDetail = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("12")).
			Padding(1, 2).
			Margin(1, 2)

	styleFormLabel = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Width(12)

	styleFormLabelActive = lipgloss.NewStyle().
				Foreground(lipgloss.Color("12")).
				Bold(true).
				Width(12)

	styleFormBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("12")).
			Padding(1, 2).
			Margin(1, 2)

	styleDeleteConfirm = lipgloss.NewStyle().
				Foreground(lipgloss.Color("9")).
				Bold(true)

	styleKWArrow = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")).
			Bold(true)

	styleKWLabel = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Bold(true)

	styleKWDay = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	styleKWDayToday = lipgloss.NewStyle().
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("12")).
			Bold(true).
			Padding(0, 1)

	styleKWDayEvent = lipgloss.NewStyle().
			Foreground(lipgloss.Color("14"))
)

// ── Messages ──────────────────────────────────────────────────────────────────

type eventsLoadedMsg struct{ events []models.Event }
type syncDoneMsg struct {
	events []models.Event
	err    error
}
type eventCreatedMsg struct{ err error }
type eventDeletedMsg struct {
	id  string
	err error
}
type errMsg struct{ err error }

// ── Model ─────────────────────────────────────────────────────────────────────

type view int

const (
	viewList view = iota
	viewDetail
	viewFree
	viewCreate
)

// form field indices
const (
	fTitle = iota
	fDate
	fTime
	fDuration
	fCalendar
	fLocation
	fCount
)

var formLabels = [fCount]string{"Title", "Date", "Time", "Duration", "Calendar", "Location"}
var formPlaceholders = [fCount]string{"Meeting mit Team", time.Now().Format("2006-01-02"), "09:00", "1h", config.Active.DefaultCalendar, "optional"}

type Model struct {
	events       []models.Event
	rows         []row
	cursor       int
	view         view
	loading      bool
	syncing      bool
	err          error
	width        int
	height       int
	daysAhead    int
	weekOffset   int
	// create form
	inputs       [fCount]textinput.Model
	inputIdx     int
	submitting   bool
	// delete
	deleteTarget *models.Event
}

type row struct {
	isHeader bool
	label    string
	event    *models.Event
}

func New() Model {
	return Model{
		daysAhead: 7,
		loading:   true,
	}
}

// newFormInputs returns fresh text inputs. Call focusInput(0) separately to get the blink cmd.
func newFormInputs() [fCount]textinput.Model {
	var inputs [fCount]textinput.Model
	for i := range inputs {
		t := textinput.New()
		t.Placeholder = formPlaceholders[i]
		t.CharLimit = 120
		inputs[i] = t
	}
	inputs[fDate].SetValue(time.Now().Format("2006-01-02"))
	return inputs
}

func (m Model) Init() tea.Cmd {
	return loadEvents(0, 7)
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case eventsLoadedMsg:
		m.loading = false
		m.events = msg.events
		m.rows = buildRows(msg.events, m.weekOffset, m.daysAhead)
		if m.cursor >= len(m.rows) {
			m.cursor = 0
		}

	case syncDoneMsg:
		m.syncing = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			m.events = msg.events
			m.rows = buildRows(msg.events, m.weekOffset, m.daysAhead)
		}

	case eventCreatedMsg:
		m.submitting = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			m.view = viewList
			m.inputs = newFormInputs()
			return m, loadEvents(m.weekOffset, m.daysAhead)
		}

	case eventDeletedMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			m.events = removeByID(m.events, msg.id)
			m.rows = buildRows(m.events, m.weekOffset, m.daysAhead)
			if m.cursor >= len(m.rows) {
				m.cursor = max(0, len(m.rows)-1)
			}
		}
		m.deleteTarget = nil

	case errMsg:
		m.loading = false
		m.syncing = false
		m.err = msg.err

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// forward key events to active text input in create view
	if m.view == viewCreate {
		var cmd tea.Cmd
		m.inputs[m.inputIdx], cmd = m.inputs[m.inputIdx].Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// ── delete confirmation ───────────────────────────────────────────────────
	if m.deleteTarget != nil {
		switch msg.String() {
		case "y", "Y":
			target := m.deleteTarget
			m.deleteTarget = nil
			return m, deleteEventCmd(target)
		default:
			m.deleteTarget = nil
		}
		return m, nil
	}

	// ── create form ───────────────────────────────────────────────────────────
	if m.view == viewCreate {
		switch msg.String() {
		case "esc":
			m.view = viewList
			m.inputIdx = 0
			return m, nil
		case "tab", "down":
			m.inputs[m.inputIdx].Blur()
			m.inputIdx = (m.inputIdx + 1) % fCount
			return m, m.inputs[m.inputIdx].Focus()
		case "shift+tab", "up":
			m.inputs[m.inputIdx].Blur()
			m.inputIdx = (m.inputIdx - 1 + fCount) % fCount
			return m, m.inputs[m.inputIdx].Focus()
		case "enter":
			if m.inputIdx < fCount-1 {
				m.inputs[m.inputIdx].Blur()
				m.inputIdx++
				return m, m.inputs[m.inputIdx].Focus()
			}
			return m.submitCreate()
		case "ctrl+s":
			return m.submitCreate()
		}
		var cmd tea.Cmd
		m.inputs[m.inputIdx], cmd = m.inputs[m.inputIdx].Update(msg)
		return m, cmd
	}

	// ── detail / free view ───────────────────────────────────────────────────
	if m.view == viewDetail || m.view == viewFree {
		switch msg.String() {
		case "q", "esc", "backspace":
			m.view = viewList
		}
		return m, nil
	}

	// ── list view ─────────────────────────────────────────────────────────────
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		m.cursor = max(0, m.cursor-1)
		for m.cursor > 0 && m.rows[m.cursor].isHeader {
			m.cursor--
		}

	case "down", "j":
		m.cursor = min(len(m.rows)-1, m.cursor+1)
		for m.cursor < len(m.rows)-1 && m.rows[m.cursor].isHeader {
			m.cursor++
		}

	case "enter":
		if m.cursor < len(m.rows) && !m.rows[m.cursor].isHeader {
			m.view = viewDetail
		}

	case "n":
		m.view = viewCreate
		m.inputs = newFormInputs()
		m.inputIdx = 0
		return m, m.inputs[fTitle].Focus()

	case "d":
		if m.cursor < len(m.rows) && !m.rows[m.cursor].isHeader {
			e := m.rows[m.cursor].event
			if e != nil && e.Title != "" && e.Title != "(no events)" {
				m.deleteTarget = e
			}
		}

	case "s":
		if !m.syncing {
			m.syncing = true
			m.err = nil
			return m, syncCmd(m.weekOffset, m.daysAhead)
		}

	case "f":
		m.view = viewFree

	case "+", "]":
		m.daysAhead = min(m.daysAhead+7, 90)
		m.rows = buildRows(m.events, m.weekOffset, m.daysAhead)

	case "-", "[":
		m.daysAhead = max(m.daysAhead-7, 7)
		m.rows = buildRows(m.events, m.weekOffset, m.daysAhead)

	case "left", "h":
		m.weekOffset--
		m.cursor = 0
		return m, loadEvents(m.weekOffset, m.daysAhead)

	case "right", "l":
		m.weekOffset++
		m.cursor = 0
		return m, loadEvents(m.weekOffset, m.daysAhead)
	}

	return m, nil
}

func (m Model) submitCreate() (Model, tea.Cmd) {
	title := strings.TrimSpace(m.inputs[fTitle].Value())
	if title == "" {
		m.err = fmt.Errorf("title is required")
		return m, nil
	}
	m.submitting = true
	m.err = nil
	return m, createEventCmd(m.inputs)
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	var b strings.Builder
	b.WriteString(m.renderWeekNav())
	b.WriteString("\n")
	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	switch m.view {
	case viewCreate:
		b.WriteString(m.renderCreate())
	case viewDetail:
		b.WriteString(m.renderDetail())
	case viewFree:
		b.WriteString(m.renderFree())
	default:
		b.WriteString(m.renderList())
	}

	b.WriteString(m.renderStatusBar())
	return b.String()
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

func (m Model) renderHeader() string {
	left := styleHeader.Render("calctl") + "  " + time.Now().Format("Mon, Jan 02 2006")
	right := ""
	if m.submitting {
		right = styleLoading.Render("saving…")
	} else if m.syncing {
		right = styleLoading.Render("syncing…")
	} else if m.err != nil {
		right = styleError.Render("⚠ " + m.err.Error())
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m Model) renderList() string {
	if m.loading {
		return styleLoading.Render("\n  Loading events…\n")
	}

	var b strings.Builder
	b.WriteString("\n")

	contentHeight := m.height - 6
	visibleRows := m.visibleRows(contentHeight)

	for _, r := range visibleRows {
		if r.isHeader {
			b.WriteString("  " + styleDateBanner.Render(r.label) + "\n")
			b.WriteString("  " + styleDivider.Render(strings.Repeat("─", m.width-4)) + "\n")
			continue
		}

		e := r.event
		selected := m.rows[m.cursor] == r

		timeStr := styleTime.Render(e.StartTime.Format("15:04") + "–" + e.EndTime.Format("15:04"))
		if e.AllDay {
			timeStr = styleAllDay.Render("all day    ")
		}

		titleStyle := styleTitle
		if selected {
			titleStyle = styleTitleSelected
		}

		calLabel := ""
		if e.Calendar != "" {
			calLabel = styleCal.Render("  [" + e.Calendar + "]")
		}

		b.WriteString("  " + timeStr + " " + titleStyle.Render(" "+truncate(e.Title, m.width-30)+" ") + calLabel + "\n")
	}

	used := strings.Count(b.String(), "\n")
	for i := used; i < contentHeight; i++ {
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) renderCreate() string {
	var b strings.Builder
	b.WriteString("\n")

	inner := strings.Builder{}
	inner.WriteString(styleHeader.Render("New Event") + "\n\n")
	for i, inp := range m.inputs {
		label := formLabels[i]
		labelStyle := styleFormLabel
		if i == m.inputIdx {
			labelStyle = styleFormLabelActive
		}
		inner.WriteString(labelStyle.Render(label) + "  " + inp.View() + "\n")
	}

	b.WriteString(styleFormBox.Render(inner.String()))
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
			formatDur(e.EndTime.Sub(e.StartTime)),
		))
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
	return styleDetail.Render(b.String())
}

func (m Model) renderFree() string {
	s, err := store.New(config.DBPath())
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
			formatDur(sl.Duration),
		))
	}
	return b.String()
}

func (m Model) renderStatusBar() string {
	if m.view == viewCreate {
		return styleStatusBar.Render(
			key("tab") + " next field  " +
				key("enter") + " next / save  " +
				key("ctrl+s") + " save  " +
				key("esc") + " cancel",
		)
	}
	if m.view == viewDetail || m.view == viewFree {
		return styleStatusBar.Render(key("esc") + " back  " + key("q") + " quit")
	}
	if m.deleteTarget != nil {
		return styleDeleteConfirm.Render(
			fmt.Sprintf("  Delete %q?  ", m.deleteTarget.Title),
		) + styleStatusBar.Render(key("y")+" confirm  "+key("any")+" cancel")
	}
	return styleStatusBar.Render(
		key("↑↓") + " navigate  " +
			key("←→") + " week  " +
			key("enter") + " detail  " +
			key("n") + " new  " +
			key("d") + " delete  " +
			key("s") + " sync  " +
			key("f") + " free  " +
			key("+/-") + fmt.Sprintf(" %dd", m.daysAhead) + "  " +
			key("q") + " quit",
	)
}

func (m Model) visibleRows(height int) []row {
	if len(m.rows) == 0 {
		return nil
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
	return m.rows[start:end]
}

// ── Commands ──────────────────────────────────────────────────────────────────

func loadEvents(weekOffset, days int) tea.Cmd {
	return func() tea.Msg {
		s, err := store.New(config.DBPath())
		if err != nil {
			return errMsg{err}
		}
		defer s.Close()
		from := weekStart(weekOffset)
		to := from.AddDate(0, 0, days)
		events, err := s.ListEvents(context.Background(), from, to)
		if err != nil {
			return errMsg{err}
		}
		return eventsLoadedMsg{events}
	}
}

func syncCmd(weekOffset, days int) tea.Cmd {
	return func() tea.Msg {
		from := weekStart(weekOffset)
		to := from.AddDate(0, 0, days)
		events, err := calendar.FetchEvents(from, to)
		if err != nil {
			return syncDoneMsg{err: err}
		}
		s, err := store.New(config.DBPath())
		if err != nil {
			return syncDoneMsg{err: err}
		}
		defer s.Close()
		ctx := context.Background()
		_ = s.DeleteBySource(ctx, "apple", from, to)
		for i := range events {
			_ = s.UpsertEvent(ctx, &events[i])
		}
		stored, err := s.ListEvents(ctx, from, to)
		return syncDoneMsg{events: stored, err: err}
	}
}

func createEventCmd(inputs [fCount]textinput.Model) tea.Cmd {
	return func() tea.Msg {
		title := strings.TrimSpace(inputs[fTitle].Value())
		dateStr := strings.TrimSpace(inputs[fDate].Value())
		timeStr := strings.TrimSpace(inputs[fTime].Value())
		durStr := strings.TrimSpace(inputs[fDuration].Value())
		calName := strings.TrimSpace(inputs[fCalendar].Value())
		loc := strings.TrimSpace(inputs[fLocation].Value())

		if dateStr == "" {
			dateStr = time.Now().Format("2006-01-02")
		}
		if timeStr == "" {
			timeStr = "09:00"
		}
		if durStr == "" {
			durStr = "1h"
		}

		start, err := time.ParseInLocation("2006-01-02 15:04", dateStr+" "+timeStr, time.Local)
		if err != nil {
			return eventCreatedMsg{fmt.Errorf("invalid date/time: %w", err)}
		}
		dur, err := parseDuration(durStr)
		if err != nil {
			return eventCreatedMsg{err}
		}

		e := &models.Event{
			ID:        "calctl-" + uuid.New().String(),
			Title:     title,
			StartTime: start,
			EndTime:   start.Add(dur),
			Calendar:  calName,
			Location:  loc,
			Source:    "calctl",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := calendar.CreateEvent(e); err != nil {
			return eventCreatedMsg{err}
		}

		s, err := store.New(config.DBPath())
		if err == nil {
			defer s.Close()
			_ = s.UpsertEvent(context.Background(), e)
		}
		return eventCreatedMsg{}
	}
}

func deleteEventCmd(e *models.Event) tea.Cmd {
	return func() tea.Msg {
		if err := calendar.DeleteEvent(e); err != nil {
			return eventDeletedMsg{err: err}
		}
		s, err := store.New(config.DBPath())
		if err == nil {
			defer s.Close()
			_ = s.DeleteByID(context.Background(), e.ID)
		}
		return eventDeletedMsg{id: e.ID}
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func buildRows(events []models.Event, weekOffset, daysAhead int) []row {
	from := weekStart(weekOffset)
	today := startOfDay(time.Now())
	var rows []row

	for d := 0; d < daysAhead; d++ {
		day := from.AddDate(0, 0, d)
		dayEnd := day.Add(24*time.Hour - time.Second)

		var dayEvents []models.Event
		for _, e := range events {
			if !e.StartTime.Before(day) && !e.StartTime.After(dayEnd) {
				dayEvents = append(dayEvents, e)
			}
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

func removeByID(events []models.Event, id string) []models.Event {
	out := events[:0]
	for _, e := range events {
		if e.ID != id {
			out = append(out, e)
		}
	}
	return out
}

// parseDuration parses "1h", "30min", "90m", "1h30m", "60" (bare number = minutes).
func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 60 * time.Minute, nil
	}
	s2 := strings.ToLower(strings.TrimSpace(s))
	// bare number → minutes
	if d, err := fmt.Sscanf(s2, "%d", new(int)); d == 1 && err == nil {
		var n int
		fmt.Sscanf(s2, "%d", &n)
		return time.Duration(n) * time.Minute, nil
	}
	// "Xmin"
	if strings.HasSuffix(s2, "min") {
		var n int
		if _, err := fmt.Sscanf(strings.TrimSuffix(s2, "min"), "%d", &n); err == nil {
			return time.Duration(n) * time.Minute, nil
		}
	}
	// Go duration ("1h", "30m", "1h30m")
	d, err := time.ParseDuration(s2)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q (use 1h, 30min, 1h30m, 90)", s)
	}
	return d, nil
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

func truncate(s string, max int) string {
	if len([]rune(s)) <= max {
		return s
	}
	return string([]rune(s)[:max-1]) + "…"
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

func formatDur(d time.Duration) string {
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

func key(k string) string {
	return styleStatusKey.Render(k)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Run starts the TUI.
func Run() error {
	p := tea.NewProgram(New(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
