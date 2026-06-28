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
)

// ── Messages ─────────────────────────────────────────────────────────────────

type eventsLoadedMsg struct{ events []models.Event }
type syncDoneMsg struct {
	events []models.Event
	err    error
}
type errMsg struct{ err error }

// ── Model ─────────────────────────────────────────────────────────────────────

type view int

const (
	viewList view = iota
	viewDetail
	viewFree
)

type Model struct {
	events   []models.Event
	rows     []row       // flattened display rows
	cursor   int
	view     view
	loading  bool
	syncing  bool
	err      error
	width    int
	height   int
	daysAhead int
}

// row is either a day header or an event entry.
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

func (m Model) Init() tea.Cmd {
	return loadEvents(7)
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
		m.rows = buildRows(msg.events, m.daysAhead)
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
			m.rows = buildRows(msg.events, m.daysAhead)
		}

	case errMsg:
		m.loading = false
		m.syncing = false
		m.err = msg.err

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.view {
	case viewDetail:
		switch msg.String() {
		case "q", "esc", "backspace":
			m.view = viewList
		}
		return m, nil

	case viewFree:
		switch msg.String() {
		case "q", "esc", "backspace":
			m.view = viewList
		}
		return m, nil
	}

	// viewList
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		m.cursor = max(0, m.cursor-1)
		// skip header rows
		for m.cursor > 0 && m.rows[m.cursor].isHeader {
			m.cursor--
		}

	case "down", "j":
		m.cursor = min(len(m.rows)-1, m.cursor+1)
		// skip header rows
		for m.cursor < len(m.rows)-1 && m.rows[m.cursor].isHeader {
			m.cursor++
		}

	case "enter":
		if m.cursor < len(m.rows) && !m.rows[m.cursor].isHeader {
			m.view = viewDetail
		}

	case "s":
		if !m.syncing {
			m.syncing = true
			m.err = nil
			return m, syncCmd(m.daysAhead)
		}

	case "f":
		m.view = viewFree

	case "+", "]":
		m.daysAhead = min(m.daysAhead+7, 90)
		m.rows = buildRows(m.events, m.daysAhead)

	case "-", "[":
		m.daysAhead = max(m.daysAhead-7, 7)
		m.rows = buildRows(m.events, m.daysAhead)
	}

	return m, nil
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	var b strings.Builder

	// Header
	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	switch m.view {
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

func (m Model) renderHeader() string {
	left := styleHeader.Render("calctl") + "  " + time.Now().Format("Mon, Jan 02 2006")
	right := ""
	if m.syncing {
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

	contentHeight := m.height - 5 // header + status bar + margins
	visibleRows := m.visibleRows(contentHeight)

	for idx, r := range visibleRows {
		_ = idx
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

		line := "  " + timeStr + " " + titleStyle.Render(" "+truncate(e.Title, m.width-30)+" ") + calLabel
		b.WriteString(line + "\n")
	}

	// fill remaining space
	used := strings.Count(b.String(), "\n")
	for i := used; i < contentHeight; i++ {
		b.WriteString("\n")
	}

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

	from := startOfDay(time.Now())
	to := from.AddDate(0, 0, m.daysAhead)
	events, _ := s.ListEvents(context.Background(), from, to)

	cfg := config.Active
	slots := calendar.FindFreeSlots(events, from, to, calendar.WorkingHours{
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
	if m.view == viewDetail || m.view == viewFree {
		return styleStatusBar.Render(key("esc") + " back  " + key("q") + " quit")
	}

	daysInfo := fmt.Sprintf(" next %d days", m.daysAhead)
	return styleStatusBar.Render(
		key("↑↓") + " navigate  " +
			key("enter") + " detail  " +
			key("s") + " sync  " +
			key("f") + " free slots  " +
			key("+/-") + daysInfo + "  " +
			key("q") + " quit",
	)
}

// visibleRows returns a window of rows that fits within height.
func (m Model) visibleRows(height int) []row {
	if len(m.rows) == 0 {
		return nil
	}

	// ensure cursor is visible
	start := 0
	end := len(m.rows)

	if end-start > height {
		// scroll so cursor is roughly centered
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

func loadEvents(days int) tea.Cmd {
	return func() tea.Msg {
		s, err := store.New(config.DBPath())
		if err != nil {
			return errMsg{err}
		}
		defer s.Close()

		from := startOfDay(time.Now())
		to := from.AddDate(0, 0, days)
		events, err := s.ListEvents(context.Background(), from, to)
		if err != nil {
			return errMsg{err}
		}
		return eventsLoadedMsg{events}
	}
}

func syncCmd(days int) tea.Cmd {
	return func() tea.Msg {
		from := startOfDay(time.Now())
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

		// reload from cache
		stored, err := s.ListEvents(ctx, from, to)
		return syncDoneMsg{events: stored, err: err}
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func buildRows(events []models.Event, daysAhead int) []row {
	from := startOfDay(time.Now())
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
		if d == 0 {
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

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
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
