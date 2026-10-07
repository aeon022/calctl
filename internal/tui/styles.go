package tui

import (
	"image/color"
	"os"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/aeon022/calctl/internal/config"
	"github.com/aeon022/missionctl-core/lastsync"
	"github.com/aeon022/missionctl-core/palette"
	"github.com/aeon022/missionctl-core/theme"
)

// ── Styles ────────────────────────────────────────────────────────────────────

// adaptive resolves a light/dark ANSI color pair once, at startup — v2 dropped
// AdaptiveColor, and these package-level styles are built once, not per render.
var adaptive = func() func(light, dark string) color.Color {
	pick := lipgloss.LightDark(lipgloss.HasDarkBackground(os.Stdin, os.Stdout))
	return func(light, dark string) color.Color { return pick(lipgloss.Color(light), lipgloss.Color(dark)) }
}()

var (
	// Shared across the suite via missionctl-core/theme.
	colorBlue   = theme.BlueV2
	colorGreen  = theme.GreenV2
	colorRed    = theme.RedV2
	colorAmber  = theme.AmberV2
	colorMuted  = theme.MutedV2
	colorSubtle = theme.SubtleV2
	colorCyan   = adaptive("30", "43")

	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorBlue)

	styleDateBanner = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorCyan)

	styleTitle = lipgloss.NewStyle()

	styleTitleSelected = lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.SelectedFgV2).
				Background(theme.SelectedBgV2)

	styleCal = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleOK = lipgloss.NewStyle().Foreground(colorGreen)

	styleEmpty = lipgloss.NewStyle().
			Foreground(colorSubtle).
			Italic(true)

	styleStatusBar = lipgloss.NewStyle().
			Foreground(colorMuted)

	// Solid badge, not just colored text — plain foreground-only red in the
	// header's top-right corner (e.g. a delete's "no matching event found")
	// was easy to miss entirely against everything else competing for that
	// one line. A filled badge draws the eye regardless of where on screen
	// it sits.
	styleError = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.OnAccentV2).
			Background(colorRed).
			Padding(0, 1)

	styleLoading = lipgloss.NewStyle().
			Foreground(colorAmber)

	styleFormLabel = lipgloss.NewStyle().
			Foreground(colorMuted).
			Width(12)

	styleFormLabelActive = lipgloss.NewStyle().
				Foreground(colorBlue).
				Bold(true).
				Width(12)

	styleDeleteConfirm = lipgloss.NewStyle().
				Foreground(colorRed).
				Bold(true)
)

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
	si.SetWidth(40)

	pi := textinput.New()
	pi.Placeholder = "command…"
	pi.CharLimit = 40
	pi.SetWidth(40)

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
		t.SetWidth(60)
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
