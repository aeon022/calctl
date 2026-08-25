package tui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/aeon022/calctl/internal/calendar"
	"github.com/aeon022/calctl/internal/config"
	"github.com/aeon022/calctl/internal/models"
	"github.com/aeon022/calctl/internal/store"
	"github.com/aeon022/missionctl-core/humanize"
	"github.com/aeon022/missionctl-core/keymap"
	"github.com/aeon022/missionctl-core/lastsync"
	"github.com/aeon022/missionctl-core/overlay"
	"github.com/aeon022/missionctl-core/palette"
	"github.com/aeon022/missionctl-core/theme"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"github.com/sahilm/fuzzy"
)

// ── Styles ────────────────────────────────────────────────────────────────────

var (
	// Shared across the suite via missionctl-core/theme.
	colorBlue   = theme.Blue
	colorGreen  = theme.Green
	colorRed    = theme.Red
	colorAmber  = theme.Amber
	colorMuted  = theme.Muted
	colorSubtle = theme.Subtle
	colorCyan   = lipgloss.AdaptiveColor{Light: "30", Dark: "43"}

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
				Foreground(theme.SelectedFg).
				Background(theme.SelectedBg)

	styleTitleHover = theme.Hover

	styleCal = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleAllDay = lipgloss.NewStyle().
			Foreground(colorGreen).
			Width(16)

	styleOK = lipgloss.NewStyle().Foreground(colorGreen)

	styleEmpty = lipgloss.NewStyle().
			Foreground(colorSubtle).
			Italic(true)

	styleStatusBar = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleStatusKey = lipgloss.NewStyle().
			Foreground(colorBlue).
			Bold(true)

	// Solid badge, not just colored text — plain foreground-only red in the
	// header's top-right corner (e.g. a delete's "no matching event found")
	// was easy to miss entirely against everything else competing for that
	// one line. A filled badge draws the eye regardless of where on screen
	// it sits.
	styleError = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.OnAccent).
			Background(colorRed).
			Padding(0, 1)

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
			Foreground(theme.SelectedFg).
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
type eventCreatedMsg struct {
	err     error
	warning string // non-fatal, e.g. "conflicts with X" — event was still created
}
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
	viewCalendarPicker // "c" — pick and save the default calendar
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

// doubleClickWindow opens the detail view on a second click within this
// window, same pattern and duration taskctl uses for its own double-click.
const doubleClickWindow = 400 * time.Millisecond

// undoWindow is how long after a delete "u" still restores it — same
// duration taskctl uses for its own delete-undo.
const undoWindow = 5 * time.Second

var formLabels = [fCount]string{"Title", "Date", "Time", "Duration", "Calendar", "Location"}

type Model struct {
	events       []models.Event
	rows         []row
	cursor       int
	hoverRow     int // m.rows index under the mouse cursor, -1 when none
	lastClickRow int // m.rows index of the previous left-click, -1 when none — double-click opens the detail view, same window/pattern taskctl uses
	lastClickAt  time.Time
	view         view
	loading      bool
	syncing      bool
	lastSynced   time.Time // zero = never synced this install; shown in the header when idle
	sp           spinner.Model
	err          error
	status       string // confirmation text (e.g. "Copied to clipboard"), cleared 3s after statusTime on the next keypress — same lazy pattern budgetctl/mailctl/notectl use
	statusTime   time.Time
	width        int
	height       int
	daysAhead    int
	weekOffset   int
	// create / edit form
	inputs     [fCount]textinput.Model
	inputIdx   int
	submitting bool
	editTarget *models.Event // non-nil when editing existing event
	// delete
	deleteTarget *models.Event
	// undo: "u" within undoWindow of a delete restores the deleted event —
	// same pattern and window taskctl uses for its own delete-undo.
	// statusTime doubles as its expiry clock (see handleKey).
	lastDeleted *models.Event
	// search / filter
	searching   bool
	searchInput textinput.Model
	searchQ     string

	// ":" command palette
	inPalette     bool
	paletteInput  textinput.Model
	paletteCursor int

	// "?" transient help popup
	helpVP   viewport.Model
	helpPopW int
	helpPopH int

	// "c" default-calendar picker
	availableCalendars []string
	calPickerCursor    int
}

type defaultCalendarSetMsg struct{ name string }

type calendarsLoadedMsg struct {
	names []string
	err   error
}

func loadCalendarsCmd() tea.Cmd {
	return func() tea.Msg {
		names, err := calendar.ListCalendars()
		return calendarsLoadedMsg{names: names, err: err}
	}
}

type row struct {
	isHeader bool
	label    string
	event    *models.Event
}

// ── command palette (":") ────────────────────────────────────────────────────
//
// Types out full words instead of memorizing single-key shortcuts. Reuses
// the exact same key handling every shortcut already goes through (the
// list-view switch in Update) by replaying the mapped keypress through
// Update itself. Matching logic lives in missionctl-core/palette (shared
// across the suite); this list is calctl-specific.
var paletteCommands = []palette.Command{
	{Name: "new", Desc: "New event", Key: "n"},
	{Name: "edit", Desc: "Edit selected event", Key: "e"},
	{Name: "delete", Desc: "Delete event (asks to confirm)", Key: "d"},
	{Name: "detail", Desc: "Event detail", Key: "enter"},
	{Name: "copy", Desc: "Copy title to clipboard", Key: "y"},
	{Name: "undo", Desc: "Undo last delete", Key: "u"},
	{Name: "free", Desc: "Free slots", Key: "f"},
	{Name: "calendar", Desc: "Set default calendar for new events", Key: "c"},
	{Name: "sync", Desc: "Sync from Apple Calendar", Key: "s"},
	{Name: "search", Desc: "Filter events", Key: "/"},
	{Name: "help", Desc: "Show help", Key: "?"},
	{Name: "quit", Desc: "Quit calctl", Key: "q"},
}

func New() Model {
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = styleLoading

	si := textinput.New()
	si.Placeholder = "filter events…"
	si.CharLimit = 100

	pi := textinput.New()
	pi.Placeholder = "command…"
	pi.CharLimit = 40

	return Model{
		daysAhead:    7,
		loading:      true,
		searchInput:  si,
		paletteInput: pi,
		sp:           sp,
		hoverRow:     -1,
		lastClickRow: -1,
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
	return tea.Batch(loadEvents(0, 7), m.sp.Tick, loadLastSyncedCmd())
}

type lastSyncedLoadedMsg struct{ t time.Time }

func loadLastSyncedCmd() tea.Cmd {
	return func() tea.Msg {
		t, _ := lastsync.Load(config.LastSyncedPath())
		return lastSyncedLoadedMsg{t: t}
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		// -1, not msg.Height: View() fills its height budget exactly and
		// never ends in a trailing newline — that combination is a
		// long-standing bubbletea quirk (charmbracelet/bubbletea#304) where
		// the renderer can fail to fully redraw right at the exact-height
		// boundary. One row of slack keeps m.height off that boundary.
		m.height = msg.Height - 1
		if m.height < 1 {
			m.height = 1
		}

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

	case lastSyncedLoadedMsg:
		m.lastSynced = msg.t

	case calendarsLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.availableCalendars = msg.names
			// preselect the current default, if it's in the list
			for i, n := range msg.names {
				if n == config.Active.DefaultCalendar {
					m.calPickerCursor = i
					break
				}
			}
		}

	case defaultCalendarSetMsg:
		m.status = "Default calendar: " + msg.name
		m.statusTime = time.Now()

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
			m.lastSynced = time.Now()
			_ = lastsync.Save(config.LastSyncedPath(), m.lastSynced)
		}

	case eventCreatedMsg:
		m.submitting = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			m.view = viewList
			m.editTarget = nil
			if msg.warning != "" {
				m.status = msg.warning
				m.statusTime = time.Now()
			}
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
		case tea.MouseButtonLeft:
			if msg.Action != tea.MouseActionPress || m.view != viewList {
				return m, nil
			}
			if i := m.rowHitTest(msg.Y); i >= 0 {
				now := time.Now()
				if i == m.lastClickRow && now.Sub(m.lastClickAt) < doubleClickWindow {
					m.cursor = i
					m.lastClickRow = -1 // consumed, so a third click starts fresh
					if e := m.rows[i].event; e != nil && e.Title != "" && e.Title != "(no events)" {
						m.view = viewDetail
					}
					return m, nil
				}
				m.cursor = i
				m.lastClickRow = i
				m.lastClickAt = now
			}
		case tea.MouseButtonNone:
			if msg.Action == tea.MouseActionMotion && m.view == viewList {
				m.hoverRow = m.rowHitTest(msg.Y)
			}
		}
		return m, nil

	case spinner.TickMsg:
		if m.syncing || m.submitting || m.loading {
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
	// The delete-undo toast gets the longer undoWindow instead of the
	// usual 3s — it's also the window "u" checks below, so the message
	// and the capability it describes expire together.
	clearAfter := 3 * time.Second
	if m.lastDeleted != nil {
		clearAfter = undoWindow
	}
	if time.Since(m.statusTime) > clearAfter {
		m.status = ""
		m.lastDeleted = nil
	}

	// ── help overlay ──────────────────────────────────────────────────────────
	if m.view == viewHelp {
		switch msg.String() {
		case "q", "esc", "?":
			m.view = viewList
			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		}
		var cmd tea.Cmd
		m.helpVP, cmd = m.helpVP.Update(msg)
		return m, cmd
	}

	// ── calendar picker ("c" — set default calendar) ────────────────────────────
	if m.view == viewCalendarPicker {
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc", "q":
			m.view = viewList
			return m, nil
		case "j", "down":
			if m.calPickerCursor < len(m.availableCalendars)-1 {
				m.calPickerCursor++
			}
		case "k", "up":
			if m.calPickerCursor > 0 {
				m.calPickerCursor--
			}
		case "enter":
			if m.calPickerCursor < len(m.availableCalendars) {
				name := m.availableCalendars[m.calPickerCursor]
				m.view = viewList
				return m, func() tea.Msg {
					if err := config.SetDefaultCalendar(name); err != nil {
						return errMsg{err}
					}
					return defaultCalendarSetMsg{name: name}
				}
			}
			m.view = viewList
		}
		return m, nil
	}

	// ── command palette ───────────────────────────────────────────────────────
	if m.inPalette {
		closePalette := func(mm Model) Model {
			mm.inPalette = false
			mm.paletteInput.Blur()
			mm.paletteInput.SetValue("")
			mm.paletteCursor = 0
			return mm
		}
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			return closePalette(m), nil
		case "up", "ctrl+p":
			if m.paletteCursor > 0 {
				m.paletteCursor--
			}
			return m, nil
		case "down", "ctrl+n":
			matches := palette.Match(paletteCommands, m.paletteInput.Value())
			if m.paletteCursor < len(matches)-1 {
				m.paletteCursor++
			}
			return m, nil
		case "enter":
			matches := palette.Match(paletteCommands, m.paletteInput.Value())
			if len(matches) == 0 {
				return closePalette(m), nil
			}
			if m.paletteCursor >= len(matches) {
				m.paletteCursor = len(matches) - 1
			}
			chosen := matches[m.paletteCursor]
			m = closePalette(m)
			replay := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(chosen.Key)}
			if chosen.Key == "enter" {
				replay = tea.KeyMsg{Type: tea.KeyEnter}
			}
			newM, cmd := m.Update(replay)
			return newM.(Model), cmd
		}
		var cmd tea.Cmd
		m.paletteInput, cmd = m.paletteInput.Update(msg)
		m.paletteCursor = 0
		return m, cmd
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
			m.lastDeleted = target
			m.status = fmt.Sprintf("Deleted %q — press u to undo", target.Title)
			m.statusTime = time.Now()
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

	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		// jump to the nth visible (on-screen) event row, headers not
		// counted — mirrors the same visibleRowsWithStart/contentHeight
		// math rowHitTest uses, so a digit lands on the same row a click at
		// that screen position would.
		n := int(msg.String()[0] - '0')
		contentHeight := m.height - 6
		if m.searching || m.searchQ != "" {
			contentHeight -= 2
		}
		if m.inPalette {
			contentHeight -= 8
		}
		visible, start := m.visibleRowsWithStart(contentHeight)
		count := 0
		for i, r := range visible {
			if r.isHeader {
				continue
			}
			count++
			if count == n {
				m.cursor = start + i
				break
			}
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

	case "u":
		if m.lastDeleted != nil {
			e := m.lastDeleted
			m.lastDeleted = nil
			m.status = ""
			return m, undoDeleteEventCmd(e)
		}

	case "y":
		if m.cursor < len(m.rows) && !m.rows[m.cursor].isHeader {
			e := m.rows[m.cursor].event
			if e != nil && e.Title != "" && e.Title != "(no events)" {
				m.status = "Copied to clipboard"
				m.statusTime = time.Now()
				return m, copyToClipboardCmd(e.Title)
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

	case "c":
		m.calPickerCursor = 0
		m.view = viewCalendarPicker
		return m, loadCalendarsCmd()

	case "/":
		m.searching = true
		m.searchInput.SetValue("")
		return m, m.searchInput.Focus()

	case ":":
		m.inPalette = true
		m.paletteCursor = 0
		m.paletteInput.SetValue("")
		return m, m.paletteInput.Focus()

	case "?":
		m = m.openHelp()

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

func (m Model) View() string {
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
		return overlay.Center(bg, m.renderHelpPopup(), m.width, m.height, 0)
	case viewCalendarPicker:
		bg := m.assembleFrame(m.renderList())
		return overlay.Center(bg, m.renderCalendarPicker(), m.width, m.height, 0)
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
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		// No room left for right (sync time, status, etc.) — drop it
		// instead of appending it anyway with zero padding, which would
		// silently push the line past m.width.
		right = ""
		gap = m.width - lipgloss.Width(left)
		if gap < 0 {
			gap = 0
		}
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m Model) renderList() string {
	if m.loading {
		return "\n  " + m.sp.View() + styleLoading.Render(" Loading events…") + "\n"
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
		titleRendered := titleStyle.Render(" ") + highlightMatches(truncate(e.Title, m.width-30), matchIdx, titleStyle) + titleStyle.Render(" ")
		b.WriteString("  " + timeStr + " " + titleRendered + calLabel + "\n")
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
	if m.view == viewCreate {
		return styleStatusBar.Render(
			key("tab") + "next field  " +
				key("enter") + "next / save  " +
				key("ctrl+s") + "save  " +
				key("esc") + "cancel",
		)
	}
	if m.view == viewDetail || m.view == viewFree {
		return styleStatusBar.Render(key("esc") + "back  " + key("q") + "quit")
	}
	if m.deleteTarget != nil {
		return styleDeleteConfirm.Render(
			fmt.Sprintf("  Delete %q?  ", m.deleteTarget.Title),
		) + styleStatusBar.Render(key("y")+"confirm  "+key("any")+"cancel")
	}
	return styleStatusBar.Render(
		key("↑↓") + "navigate  " +
			key("←→") + "week  " +
			key("enter") + "detail  " +
			key("n") + "new  " +
			key("e") + "edit  " +
			key("d") + "delete  " +
			key("u") + "undo  " +
			key("y") + "copy  " +
			key("/") + "filter  " +
			key("s") + "sync  " +
			key("f") + "free  " +
			key("+/-") + fmt.Sprintf("%dd", m.daysAhead) + "  " +
			key("?") + "help  " +
			key("q") + "quit",
	)
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

	vp := viewport.New(popW-6, popH-5) // border 1+1, padding(1,2) → 2 rows/4 cols; -1 row for footer
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
	if m.helpVP.TotalLineCount() > m.helpVP.Height {
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

// ── Commands ──────────────────────────────────────────────────────────────────

func loadEvents(weekOffset, days int) tea.Cmd {
	return func() tea.Msg {
		s, err := store.New(config.DBPath(), config.Shared())
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
		s, err := store.New(config.DBPath(), config.Shared())
		if err != nil {
			return syncDoneMsg{err: err}
		}
		defer s.Close()
		ctx := context.Background()
		if _, err := calendar.Sync(ctx, s, from, to); err != nil {
			return syncDoneMsg{err: err}
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
		if calName == "" {
			calName = config.Active.DefaultCalendar
		}
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
			return eventCreatedMsg{err: fmt.Errorf("invalid date/time: %w", err)}
		}
		dur, err := models.ParseDuration(durStr)
		if err != nil {
			return eventCreatedMsg{err: err}
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

		s, err := store.New(config.DBPath(), config.Shared())
		if err != nil {
			return eventCreatedMsg{err: err}
		}
		defer s.Close()
		ctx := context.Background()

		// edit = delete old + create new. Abort before creating the
		// replacement if the old event's real Calendar.app deletion fails —
		// used to ignore this error and delete the DB row unconditionally,
		// which on failure left the original event alive and untracked in
		// Calendar.app while a new, separate event got created alongside
		// it (same ghost-event class of bug as a plain failed delete, just
		// reachable via edit instead).
		if editTarget != nil {
			if err := calendar.DeleteEvent(editTarget); err != nil {
				return eventCreatedMsg{err: fmt.Errorf("could not remove original event before edit: %w", err)}
			}
			_ = s.DeleteByID(ctx, editTarget.ID)
		}

		warning := ""
		dayStart := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.Local)
		if existing, cerr := s.ListEvents(ctx, dayStart, dayStart.Add(24*time.Hour)); cerr == nil {
			if conflicts := models.FindConflicts(*e, existing); len(conflicts) > 0 {
				names := make([]string, len(conflicts))
				for i, c := range conflicts {
					names[i] = c.Title
				}
				warning = "⚠ conflicts with " + strings.Join(names, ", ")
			}
		}

		if err := calendar.CreateEvent(e); err != nil {
			return eventCreatedMsg{err: err}
		}
		_ = s.UpsertEvent(ctx, e)
		return eventCreatedMsg{warning: warning}
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
		s, err := store.New(config.DBPath(), config.Shared())
		if err == nil {
			defer s.Close()
			_ = s.DeleteByID(context.Background(), e.ID)
		}
		return eventDeletedMsg{id: e.ID}
	}
}

// undoDeleteEventCmd re-creates a deleted event — used by "u" within
// undoWindow of a delete. Recreates in both Apple Calendar and the local
// DB, same delete+recreate plumbing the edit flow already uses; the
// restored event gets a fresh ID/source ("calctl") since Calendar.app
// assigns its own new identifier on create, same tradeoff taskctl accepts
// for its own delete-undo.
func undoDeleteEventCmd(e *models.Event) tea.Cmd {
	return func() tea.Msg {
		restored := &models.Event{
			ID:        "calctl-" + uuid.New().String(),
			Title:     e.Title,
			StartTime: e.StartTime,
			EndTime:   e.EndTime,
			AllDay:    e.AllDay,
			Calendar:  e.Calendar,
			Location:  e.Location,
			Notes:     e.Notes,
			Source:    "calctl",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := calendar.CreateEvent(restored); err != nil {
			return eventCreatedMsg{err: err}
		}
		s, err := store.New(config.DBPath(), config.Shared())
		if err != nil {
			return eventCreatedMsg{err: err}
		}
		defer s.Close()
		_ = s.UpsertEvent(context.Background(), restored)
		return eventCreatedMsg{}
	}
}

// copyToClipboardCmd shells out to pbcopy — same approach taskctl/mailctl/
// notectl use for their own "y" copy shortcuts, no clipboard library needed.
func copyToClipboardCmd(text string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("pbcopy")
		cmd.Stdin = strings.NewReader(text)
		_ = cmd.Run()
		return nil
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

// key renders a footer key-hint in the suite-wide "key:label" format —
// callers append the label text (no leading space) right after this call.
func key(k string) string {
	return styleStatusKey.Render(k + ":")
}

// Run starts the TUI.
func Run() error {
	// WithFPS(30), not the 60 default: WithMouseAllMotion forces a full
	// re-render on every pixel of mouse movement, and 60fps of heavily
	// styled frames can outpace what the terminal can keep up with —
	// confirmed as the cause of a severe duplicate-content rendering bug
	// in notectl (same bubbletea setup). Halving the rate gives the
	// terminal breathing room.
	p := tea.NewProgram(New(), tea.WithAltScreen(), tea.WithMouseAllMotion(), tea.WithFPS(30))
	_, err := p.Run()
	return err
}
