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
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
)

// ── Styles ────────────────────────────────────────────────────────────────────

var (
	colorBlue   = lipgloss.AdaptiveColor{Light: "25",  Dark: "33"}
	colorGreen  = lipgloss.AdaptiveColor{Light: "28",  Dark: "42"}
	colorRed    = lipgloss.AdaptiveColor{Light: "160", Dark: "203"}
	colorAmber  = lipgloss.AdaptiveColor{Light: "214", Dark: "220"}
	colorMuted  = lipgloss.AdaptiveColor{Light: "243", Dark: "246"}
	colorSubtle = lipgloss.AdaptiveColor{Light: "250", Dark: "244"}
	colorCyan   = lipgloss.AdaptiveColor{Light: "30",  Dark: "43"}

	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorBlue)

	styleDateBanner = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorCyan)

	styleDivider = lipgloss.NewStyle().
			Foreground(colorSubtle)

	styleTime = lipgloss.NewStyle().
			Foreground(colorAmber).
			Width(16)

	styleTitle = lipgloss.NewStyle()

	styleTitleSelected = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "16", Dark: "255"}).
				Background(lipgloss.AdaptiveColor{Light: "189", Dark: "17"})

	styleCal = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleAllDay = lipgloss.NewStyle().
			Foreground(colorGreen).
			Width(16)

	styleEmpty = lipgloss.NewStyle().
			Foreground(colorSubtle).
			Italic(true)

	styleStatusBar = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleStatusKey = lipgloss.NewStyle().
			Foreground(colorBlue).
			Bold(true)

	styleError = lipgloss.NewStyle().
			Foreground(colorRed)

	styleLoading = lipgloss.NewStyle().
			Foreground(colorAmber)

	styleDetail = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBlue).
			Padding(1, 2).
			Margin(1, 2)

	styleFormLabel = lipgloss.NewStyle().
			Foreground(colorMuted).
			Width(12)

	styleFormLabelActive = lipgloss.NewStyle().
				Foreground(colorBlue).
				Bold(true).
				Width(12)

	styleFormBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBlue).
			Padding(1, 2).
			Margin(1, 2)

	styleDeleteConfirm = lipgloss.NewStyle().
				Foreground(colorRed).
				Bold(true)

	styleKWArrow = lipgloss.NewStyle().
			Foreground(colorBlue).
			Bold(true)

	styleKWLabel = lipgloss.NewStyle().
			Foreground(colorMuted).
			Bold(true)

	styleKWDay = lipgloss.NewStyle().
			Foreground(colorSubtle)

	styleKWDayToday = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "16", Dark: "255"}).
			Background(colorBlue).
			Bold(true).
			Padding(0, 1)

	styleKWDayEvent = lipgloss.NewStyle().
			Foreground(colorCyan)
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
	viewHelp
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

type Model struct {
	events       []models.Event
	rows         []row
	cursor       int
	view         view
	loading      bool
	syncing      bool
	sp           spinner.Model
	err          error
	width        int
	height       int
	daysAhead    int
	weekOffset   int
	// create / edit form
	inputs       [fCount]textinput.Model
	inputIdx     int
	submitting   bool
	editTarget   *models.Event // non-nil when editing existing event
	// delete
	deleteTarget *models.Event
	// search / filter
	searching   bool
	searchInput textinput.Model
	searchQ     string
}

type row struct {
	isHeader bool
	label    string
	event    *models.Event
}

func New() Model {
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = styleLoading

	si := textinput.New()
	si.Placeholder = "filter events…"
	si.CharLimit = 100
	return Model{
		daysAhead:   7,
		loading:     true,
		searchInput: si,
		sp:          sp,
	}
}

// newFormInputs returns fresh text inputs. Call focusInput(0) separately to get the blink cmd.
// Placeholders that depend on runtime config (e.g. DefaultCalendar) are set here, not at
// package-init time, because config.Load() hasn't run yet during package initialization.
func newFormInputs() [fCount]textinput.Model {
	var inputs [fCount]textinput.Model
	placeholders := [fCount]string{
		"Meeting mit Team",
		time.Now().Format("2006-01-02"),
		"09:00",
		"1h",
		config.Active.DefaultCalendar, // safe here: config.Load() runs before any command
		"optional",
	}
	for i := range inputs {
		t := textinput.New()
		t.Placeholder = placeholders[i]
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
		m.rows = buildRows(msg.events, m.weekOffset, m.daysAhead, m.searchQ)
		if m.cursor < 0 || m.cursor >= len(m.rows) {
			m.cursor = 0
		}
		// Advance past the leading header row so the cursor starts on an event.
		if len(m.rows) > 0 && m.rows[m.cursor].isHeader {
			for i := m.cursor + 1; i < len(m.rows); i++ {
				if !m.rows[i].isHeader {
					m.cursor = i
					break
				}
			}
		}

	case syncDoneMsg:
		m.syncing = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			m.events = msg.events
			m.rows = buildRows(msg.events, m.weekOffset, m.daysAhead, m.searchQ)
			if m.cursor < 0 || m.cursor >= len(m.rows) {
				m.cursor = 0
			}
		}

	case eventCreatedMsg:
		m.submitting = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			m.view = viewList
			m.editTarget = nil
			return m, loadEvents(m.weekOffset, m.daysAhead)
		}

	case eventDeletedMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			m.events = removeByID(m.events, msg.id)
			m.rows = buildRows(m.events, m.weekOffset, m.daysAhead, m.searchQ)
			if m.cursor >= len(m.rows) {
				m.cursor = max(0, len(m.rows)-1)
			}
		}
		m.deleteTarget = nil

	case errMsg:
		m.loading = false
		m.syncing = false
		m.err = msg.err

	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if len(m.rows) > 0 {
				prev := m.cursor - 1
				for prev > 0 && m.rows[prev].isHeader {
					prev--
				}
				if prev >= 0 && !m.rows[prev].isHeader {
					m.cursor = prev
				}
			}
		case tea.MouseButtonWheelDown:
			if len(m.rows) > 0 {
				next := m.cursor + 1
				for next < len(m.rows) && m.rows[next].isHeader {
					next++
				}
				if next < len(m.rows) {
					m.cursor = next
				}
			}
		}
		return m, nil

	case spinner.TickMsg:
		if m.syncing || m.submitting {
			var cmd tea.Cmd
			m.sp, cmd = m.sp.Update(msg)
			return m, cmd
		}
		return m, nil

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
	// ── help overlay ──────────────────────────────────────────────────────────
	if m.view == viewHelp {
		switch msg.String() {
		case "q", "esc", "?":
			m.view = viewList
		case "ctrl+c":
			return m, tea.Quit
		}
		return m, nil
	}

	// ── search input ──────────────────────────────────────────────────────────
	if m.searching {
		switch msg.String() {
		case "enter":
			m.searchQ = strings.TrimSpace(m.searchInput.Value())
			m.searching = false
			m.rows = buildRows(m.events, m.weekOffset, m.daysAhead, m.searchQ)
			m.cursor = 0
			m.advanceCursorPastHeader()
		case "esc":
			m.searching = false
			m.searchInput.SetValue("")
			m.searchQ = ""
			m.rows = buildRows(m.events, m.weekOffset, m.daysAhead, "")
			m.cursor = 0
			m.advanceCursorPastHeader()
		case "ctrl+c":
			return m, tea.Quit
		default:
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			// live filtering while typing
			m.searchQ = strings.TrimSpace(m.searchInput.Value())
			m.rows = buildRows(m.events, m.weekOffset, m.daysAhead, m.searchQ)
			m.cursor = 0
			m.advanceCursorPastHeader()
			return m, cmd
		}
		return m, nil
	}

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

	case "esc":
		if m.searchQ != "" {
			m.searchQ = ""
			m.searchInput.SetValue("")
			m.rows = buildRows(m.events, m.weekOffset, m.daysAhead, "")
			m.cursor = 0
			m.advanceCursorPastHeader()
		}

	case "up", "k":
		if len(m.rows) == 0 {
			break
		}
		// walk backwards to the previous non-header row
		prev := m.cursor - 1
		for prev > 0 && m.rows[prev].isHeader {
			prev--
		}
		if prev >= 0 && !m.rows[prev].isHeader {
			m.cursor = prev
		}

	case "down", "j":
		if len(m.rows) == 0 {
			break
		}
		// walk forwards to the next non-header row
		next := m.cursor + 1
		for next < len(m.rows) && m.rows[next].isHeader {
			next++
		}
		if next < len(m.rows) {
			m.cursor = next
		}

	case "enter":
		if m.cursor < len(m.rows) && !m.rows[m.cursor].isHeader {
			e := m.rows[m.cursor].event
			if e != nil && e.Title != "" && e.Title != "(no events)" {
				m.view = viewDetail
			}
		}

	case "n":
		m.view = viewCreate
		m.inputs = newFormInputs()
		m.editTarget = nil
		m.inputIdx = 0
		return m, m.inputs[fTitle].Focus()

	case "e":
		if m.cursor < len(m.rows) && !m.rows[m.cursor].isHeader {
			e := m.rows[m.cursor].event
			if e != nil && e.Title != "" && e.Title != "(no events)" {
				m.view = viewCreate
				m.inputs = prefillForm(e)
				m.editTarget = e
				m.inputIdx = 0
				return m, m.inputs[fTitle].Focus()
			}
		}

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
			return m, tea.Batch(syncCmd(m.weekOffset, m.daysAhead), m.sp.Tick)
		}

	case "f":
		m.view = viewFree

	case "/":
		m.searching = true
		m.searchInput.SetValue("")
		return m, m.searchInput.Focus()

	case "?":
		m.view = viewHelp

	case "+", "]":
		m.daysAhead = min(m.daysAhead+7, 90)
		m.rows = buildRows(m.events, m.weekOffset, m.daysAhead, m.searchQ)

	case "-", "[":
		m.daysAhead = max(m.daysAhead-7, 7)
		m.rows = buildRows(m.events, m.weekOffset, m.daysAhead, m.searchQ)

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
	return m, tea.Batch(createEventCmd(m.inputs, m.editTarget), m.sp.Tick)
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n")
	b.WriteString(m.renderWeekNav())
	b.WriteString("\n")
	b.WriteString(styleDivider.Render(strings.Repeat("─", m.width)))
	b.WriteString("\n")

	switch m.view {
	case viewCreate:
		b.WriteString(m.renderCreate())
	case viewDetail:
		b.WriteString(m.renderDetail())
	case viewFree:
		b.WriteString(m.renderFree())
	case viewHelp:
		b.WriteString(m.renderHelp())
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
	if m.searching {
		b.WriteString("  / " + m.searchInput.View() + "\n\n")
		contentHeight -= 2
	} else if m.searchQ != "" {
		b.WriteString("  " + styleCal.Render("filter: /"+m.searchQ+"  (esc to clear)") + "\n\n")
		contentHeight -= 2
	}
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

	if len(visibleRows) == 0 {
		switch {
		case m.searchQ != "":
			b.WriteString("  " + styleCal.Render("No events match your search.") + "\n")
		case len(m.rows) == 0:
			b.WriteString("  " + styleCal.Render("No events yet — press s to sync from Apple Calendar, or calctl import to add one.") + "\n")
		}
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
			key("e") + " edit  " +
			key("d") + " delete  " +
			key("/") + " filter  " +
			key("s") + " sync  " +
			key("f") + " free  " +
			key("+/-") + fmt.Sprintf(" %dd", m.daysAhead) + "  " +
			key("?") + " help  " +
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

func (m Model) renderHelp() string {
	row := func(k, desc string) string {
		return "  " + styleStatusKey.Render(fmt.Sprintf("%-9s", k)) + styleStatusBar.Render(desc) + "\n"
	}
	section := func(t string) string { return "\n  " + styleHeader.Render(t) + "\n" }

	var b strings.Builder
	b.WriteString(section("Navigation"))
	b.WriteString(row("j / ↓", "next event"))
	b.WriteString(row("k / ↑", "previous event"))
	b.WriteString(row("h / ←", "previous week"))
	b.WriteString(row("l / →", "next week"))
	b.WriteString(row("+ / -", "show more / fewer days (7–90)"))
	b.WriteString(section("Events"))
	b.WriteString(row("enter", "event detail"))
	b.WriteString(row("n", "new event"))
	b.WriteString(row("e", "edit event"))
	b.WriteString(row("d", "delete event (asks to confirm)"))
	b.WriteString(section("Views & Data"))
	b.WriteString(row("/", "filter events (title, location, calendar, notes)"))
	b.WriteString(row("esc", "clear active filter"))
	b.WriteString(row("f", "free slots"))
	b.WriteString(row("s", "sync from Apple Calendar"))
	b.WriteString(section("Other"))
	b.WriteString(row("?", "toggle this help"))
	b.WriteString(row("q", "quit"))
	return styleDetail.Render(b.String())
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

func createEventCmd(inputs [fCount]textinput.Model, editTarget *models.Event) tea.Cmd {
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

		s, err := store.New(config.DBPath())
		if err != nil {
			return eventCreatedMsg{err}
		}
		defer s.Close()
		ctx := context.Background()

		// edit = delete old + create new
		if editTarget != nil {
			_ = calendar.DeleteEvent(editTarget)
			_ = s.DeleteByID(ctx, editTarget.ID)
		}

		if err := calendar.CreateEvent(e); err != nil {
			return eventCreatedMsg{err}
		}
		_ = s.UpsertEvent(ctx, e)
		return eventCreatedMsg{}
	}
}

// prefillForm creates a form pre-filled with an existing event's values.
func prefillForm(e *models.Event) [fCount]textinput.Model {
	inputs := newFormInputs()
	inputs[fTitle].SetValue(e.Title)
	inputs[fDate].SetValue(e.StartTime.Format("2006-01-02"))
	if !e.AllDay {
		inputs[fTime].SetValue(e.StartTime.Format("15:04"))
		dur := e.EndTime.Sub(e.StartTime)
		h := int(dur.Hours())
		m := int(dur.Minutes()) % 60
		if h > 0 && m > 0 {
			inputs[fDuration].SetValue(fmt.Sprintf("%dh%dm", h, m))
		} else if h > 0 {
			inputs[fDuration].SetValue(fmt.Sprintf("%dh", h))
		} else {
			inputs[fDuration].SetValue(fmt.Sprintf("%dm", m))
		}
	}
	inputs[fCalendar].SetValue(e.Calendar)
	inputs[fLocation].SetValue(e.Location)
	return inputs
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

func eventMatches(e *models.Event, q string) bool {
	return strings.Contains(strings.ToLower(e.Title), q) ||
		strings.Contains(strings.ToLower(e.Location), q) ||
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
	p := tea.NewProgram(New(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
