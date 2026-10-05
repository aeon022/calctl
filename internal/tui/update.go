package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/aeon022/calctl/internal/config"
	"github.com/aeon022/missionctl-core/lastsync"
	"github.com/aeon022/missionctl-core/palette"
)

// ── Update ────────────────────────────────────────────────────────────────────

// browsing reports whether the user is just looking at the event list: no
// form, popup, search, palette or delete confirm, and nothing in flight.
func (m Model) browsing() bool {
	return m.view == viewList && !m.loading && !m.syncing && !m.focusLoading &&
		!m.searching && !m.inPalette && m.deleteTarget == nil
}

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

	case tea.FocusMsg:
		// Back from another window (e.g. Calendar.app): reload stale events,
		// but only while just browsing — never under a form, search, palette,
		// confirm or popup.
		if m.browsing() && time.Since(m.lastLoad) > 5*time.Second {
			m.focusLoading = true
			return m, loadEvents(m.weekOffset, m.daysAhead)
		}
		return m, nil

	case eventsLoadedMsg:
		keepID := ""
		if m.focusLoading && m.cursor >= 0 && m.cursor < len(m.rows) && m.rows[m.cursor].event != nil {
			keepID = m.rows[m.cursor].event.ID
		}
		m.focusLoading = false
		m.lastLoad = time.Now()
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
		for i, r := range m.rows {
			if keepID != "" && r.event != nil && r.event.ID == keepID {
				m.cursor = i
				break
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

	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			if len(m.rows) > 0 {
				prev := m.cursor - 1
				for prev > 0 && m.rows[prev].isHeader {
					prev--
				}
				if prev >= 0 && !m.rows[prev].isHeader {
					m.cursor = prev
				}
			}
		case tea.MouseWheelDown:
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

	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft || m.view != viewList {
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
		return m, nil

	case tea.MouseMotionMsg:
		if m.view == viewList {
			m.hoverRow = m.rowHitTest(msg.Y)
		}
		return m, nil

	case spinner.TickMsg:
		if m.syncing || m.submitting || m.loading {
			var cmd tea.Cmd
			m.sp, cmd = m.sp.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.KeyPressMsg:
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

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
			replay := tea.KeyPressMsg{Text: chosen.Key, Code: []rune(chosen.Key)[0]}
			if chosen.Key == "enter" {
				replay = tea.KeyPressMsg{Code: tea.KeyEnter}
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
