package tui

import (
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/aeon022/calctl/internal/calendar"
	"github.com/aeon022/calctl/internal/models"
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
	lastLoad     time.Time // when events last arrived; FocusMsg reloads only if stale
	focusLoading bool      // a focus-triggered reload is in flight (keeps cursor on the same event)
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
