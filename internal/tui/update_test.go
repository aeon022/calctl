package tui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/aeon022/calctl/internal/config"
	"github.com/aeon022/calctl/internal/models"
)

// These tests drive Model.Update with synthetic messages and assert state.
// Returned tea.Cmds are never executed — they would call Calendar.app.
// HOME and the DB are temp so nothing real is read or written.

func press(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "ctrl+s":
		return tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	}
	r := []rune(s)
	return tea.KeyPressMsg{Code: r[0], Text: s}
}

func send(m Model, msgs ...tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, msg := range msgs {
		var mi tea.Model
		mi, cmd = m.Update(msg)
		m = mi.(Model)
	}
	return m, cmd
}

func typeText(m Model, s string) Model {
	for _, r := range s {
		m, _ = send(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("CALCTL_DATA_DIR", "")
	config.DBPathOverride = filepath.Join(t.TempDir(), "c.db")
	config.Active = config.Config{WorkingHoursFrom: "09:00", WorkingHoursTo: "18:00", MinFreeSlot: 30}
	t.Cleanup(func() { config.DBPathOverride = ""; config.Active = config.Config{} })
}

// at returns day (offset from this week's Monday) at h:m local time.
func at(dayOffset, h, m int) time.Time {
	d := weekStart(0).AddDate(0, 0, dayOffset)
	return time.Date(d.Year(), d.Month(), d.Day(), h, m, 0, 0, time.Local)
}

// loaded: Tue "Standup" 09:30, Tue "Review" 14:00, Thu "Offsite" (all day, notes),
// in the current week; cursor on the first real event.
func loaded(t *testing.T) Model {
	t.Helper()
	isolate(t)
	evs := []models.Event{
		{ID: "a", Title: "Standup", StartTime: at(1, 9, 30), EndTime: at(1, 10, 0), Calendar: "Arbeit"},
		{ID: "b", Title: "Review", StartTime: at(1, 14, 0), EndTime: at(1, 15, 0), Location: "Büro", Calendar: "Arbeit"},
		{ID: "c", Title: "Offsite", StartTime: at(3, 0, 0), EndTime: at(4, 0, 0), AllDay: true, Notes: "Zug um 8 Uhr — Tickets mitbringen", Attendees: []string{"x@y.test"}},
	}
	m := New()
	m, _ = send(m, tea.WindowSizeMsg{Width: 100, Height: 50}, eventsLoadedMsg{events: evs})
	return m
}

func cur(m Model) *models.Event {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return m.rows[m.cursor].event
}

func curTitle(m Model) string {
	if e := cur(m); e != nil {
		return e.Title
	}
	return ""
}

func eventTitles(m Model) (out []string) {
	for _, r := range m.rows {
		if !r.isHeader && r.event != nil && r.event.ID != "" {
			out = append(out, r.event.Title)
		}
	}
	return
}

func TestWindowSizeAndLoadedState(t *testing.T) {
	m := loaded(t)
	if m.loading {
		t.Error("loading must end")
	}
	if m.width != 100 || m.height != 49 {
		t.Errorf("size %dx%d, want 100x49", m.width, m.height)
	}
	if m.rows[0].isHeader != true || m.cursor == 0 {
		t.Errorf("cursor = %d, must skip the leading day header", m.cursor)
	}
	if got := strings.Join(eventTitles(m), ","); got != "Standup,Review,Offsite" {
		t.Errorf("events = %s", got)
	}
	m, _ = send(m, tea.WindowSizeMsg{Width: 5, Height: 0})
	if m.height != 1 {
		t.Errorf("height = %d, want clamp to 1", m.height)
	}
}

func TestNavigationSkipsHeadersAndPlaceholders(t *testing.T) {
	m := loaded(t)
	m.cursor = 0
	m, _ = send(m, eventsLoadedMsg{events: m.events})
	// first selectable row is the Monday placeholder "(no events)"
	if curTitle(m) != "(no events)" {
		t.Fatalf("cursor on %q", curTitle(m))
	}
	m, _ = send(m, press("j"))
	if curTitle(m) != "Standup" {
		t.Errorf("j: %q, want Standup (header skipped)", curTitle(m))
	}
	m, _ = send(m, press("j"), press("down"))
	if curTitle(m) != "(no events)" { // Wednesday is empty
		t.Errorf("after two more steps: %q", curTitle(m))
	}
	m, _ = send(m, press("k"), press("up"))
	if curTitle(m) != "Standup" {
		t.Errorf("up/k: %q", curTitle(m))
	}
	for i := 0; i < 40; i++ {
		m, _ = send(m, press("j"))
	}
	last := m.cursor
	m, _ = send(m, press("j"))
	if m.cursor != last || m.rows[m.cursor].isHeader {
		t.Error("j at the end must stay on the last selectable row")
	}
	for i := 0; i < 60; i++ {
		m, _ = send(m, press("k"))
	}
	if m.cursor != 1 {
		t.Errorf("k at the top: cursor %d, want the first non-header row (1)", m.cursor)
	}
}

func TestNumberKeysJumpToNthEventRow(t *testing.T) {
	m := loaded(t)
	m, _ = send(m, press("2")) // rows: Mon placeholder, Standup, Review...
	if curTitle(m) != "Standup" {
		t.Errorf("'2' → %q, want the second visible row (headers not counted)", curTitle(m))
	}
	m, _ = send(m, press("3"))
	if curTitle(m) != "Review" {
		t.Errorf("'3' → %q", curTitle(m))
	}
}

func TestMouseWheelAndClicks(t *testing.T) {
	m := loaded(t)
	m, _ = send(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	before := m.cursor
	m, _ = send(m, tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if m.cursor >= before {
		t.Errorf("wheel up must move up (%d → %d)", before, m.cursor)
	}

	y := -1
	for i := 0; i < 80; i++ {
		if idx := m.rowHitTest(i); idx >= 0 && m.rows[idx].event != nil && m.rows[idx].event.ID == "b" {
			y = i
		}
	}
	if y < 0 {
		t.Fatal("no screen row maps to Review")
	}
	click := tea.MouseClickMsg{Button: tea.MouseLeft, X: 4, Y: y}
	m, _ = send(m, click)
	if curTitle(m) != "Review" || m.view != viewList {
		t.Fatalf("click: %q view %v", curTitle(m), m.view)
	}
	m, _ = send(m, click)
	if m.view != viewDetail {
		t.Errorf("double click must open detail, view = %v", m.view)
	}

	m.view = viewList
	m, _ = send(m, tea.MouseClickMsg{Button: tea.MouseRight, X: 4, Y: y})
	if m.view != viewList {
		t.Error("only the left button acts")
	}
	m, _ = send(m, tea.MouseMotionMsg{X: 4, Y: y})
	if m.hoverRow < 0 || m.rows[m.hoverRow].event.Title != "Review" {
		t.Errorf("hoverRow = %d", m.hoverRow)
	}
}

func TestDoubleClickOnPlaceholderDoesNotOpenDetail(t *testing.T) {
	m := loaded(t)
	m.cursor = 0
	m, _ = send(m, eventsLoadedMsg{events: m.events})
	click := tea.MouseClickMsg{Button: tea.MouseLeft, X: 4, Y: 4 + 2} // first selectable row after the Mon header
	idx := m.rowHitTest(click.Y)
	if idx < 0 || m.rows[idx].event.Title != "(no events)" {
		t.Skipf("layout assumption changed (row %d)", idx)
	}
	m, _ = send(m, click, click)
	if m.view != viewList {
		t.Error("the \"(no events)\" placeholder is not an event")
	}
}

func TestEnterOpensDetailOnlyForRealEvents(t *testing.T) {
	m := loaded(t)
	m.cursor = 1 // Monday placeholder
	m, _ = send(m, press("enter"))
	if m.view != viewList {
		t.Error("enter on the placeholder must do nothing")
	}
	m, _ = send(m, press("j"), press("enter"))
	if m.view != viewDetail {
		t.Fatalf("view = %v", m.view)
	}
	m, _ = send(m, press("esc"))
	if m.view != viewList {
		t.Error("esc must leave the detail view")
	}
}

func TestWeekAndRangeKeys(t *testing.T) {
	m := loaded(t)
	m2, cmd := send(m, press("l"))
	if m2.weekOffset != 1 || cmd == nil || m2.cursor != 0 {
		t.Errorf("l: offset=%d cmd=%v cursor=%d", m2.weekOffset, cmd != nil, m2.cursor)
	}
	m2, _ = send(m2, press("h"), press("left"))
	if m2.weekOffset != -1 {
		t.Errorf("h, left: offset=%d", m2.weekOffset)
	}
	m2, _ = send(m, press("right"))
	if m2.weekOffset != 1 {
		t.Errorf("right: offset=%d", m2.weekOffset)
	}

	for i := 0; i < 20; i++ {
		m, _ = send(m, press("+"))
	}
	if m.daysAhead != 90 {
		t.Errorf("daysAhead = %d, + caps at 90", m.daysAhead)
	}
	for i := 0; i < 20; i++ {
		m, _ = send(m, press("-"))
	}
	if m.daysAhead != 7 {
		t.Errorf("daysAhead = %d, - floors at 7", m.daysAhead)
	}
	m, _ = send(m, press("]"))
	if m.daysAhead != 14 || len(m.rows) <= 8 {
		t.Errorf("] must widen the window: days=%d rows=%d", m.daysAhead, len(m.rows))
	}
	m, _ = send(m, press("["))
	if m.daysAhead != 7 {
		t.Errorf("[ → %d", m.daysAhead)
	}
}

func TestSearchFiltersEventsAndEscClears(t *testing.T) {
	m := loaded(t)
	m, _ = send(m, press("/"))
	if !m.searching {
		t.Fatal("/ must start search")
	}
	m = typeText(m, "rev")
	if got := strings.Join(eventTitles(m), ","); got != "Review" {
		t.Errorf("filter 'rev' = %s", got)
	}
	if curTitle(m) != "Review" {
		t.Errorf("cursor must land on the first match, got %q", curTitle(m))
	}
	m, _ = send(m, press("enter"))
	if m.searching || m.searchQ != "rev" {
		t.Errorf("enter keeps the filter: searching=%v q=%q", m.searching, m.searchQ)
	}
	m, _ = send(m, press("esc")) // list-level esc clears the active filter
	if m.searchQ != "" || len(eventTitles(m)) != 3 {
		t.Errorf("esc must clear the filter: q=%q events=%v", m.searchQ, eventTitles(m))
	}

	m, _ = send(m, press("/"))
	m = typeText(m, "zzzz")
	if len(eventTitles(m)) != 0 || len(m.rows) != 0 {
		t.Errorf("no match: rows=%d (days without matches are hidden)", len(m.rows))
	}
	m, _ = send(m, press("esc"))
	if m.searching || m.searchQ != "" || len(eventTitles(m)) != 3 {
		t.Error("esc in the search box must abort and restore the list")
	}
}

func TestSearchMatchesLocationAndNotes(t *testing.T) {
	m := loaded(t)
	m, _ = send(m, press("/"))
	m = typeText(m, "büro")
	if got := strings.Join(eventTitles(m), ","); got != "Review" {
		t.Errorf("location search = %s", got)
	}
	m, _ = send(m, press("esc"))
	m, _ = send(m, press("/"))
	m = typeText(m, "tickets")
	if got := strings.Join(eventTitles(m), ","); got != "Offsite" {
		t.Errorf("notes search = %s", got)
	}
}

func TestDeleteConfirmAndUndo(t *testing.T) {
	m := loaded(t)
	m, _ = send(m, press("2"), press("d"))
	if m.deleteTarget == nil || m.deleteTarget.Title != "Standup" {
		t.Fatalf("d: target = %+v", m.deleteTarget)
	}
	m, cmd := send(m, press("n"))
	if m.deleteTarget != nil || cmd != nil {
		t.Errorf("any key but y cancels: target=%v cmd=%v", m.deleteTarget, cmd != nil)
	}

	m, _ = send(m, press("d"))
	m, cmd = send(m, press("y"))
	if m.deleteTarget != nil || m.lastDeleted == nil || cmd == nil {
		t.Fatalf("y: target=%v last=%v cmd=%v", m.deleteTarget, m.lastDeleted, cmd != nil)
	}
	if !strings.Contains(m.status, "Standup") || !strings.Contains(m.status, "undo") {
		t.Errorf("status = %q, want an undo hint naming the event", m.status)
	}

	m, _ = send(m, eventDeletedMsg{id: "a"})
	for _, ti := range eventTitles(m) {
		if ti == "Standup" {
			t.Error("deleted event must disappear from the list")
		}
	}

	m, cmd = send(m, press("u"))
	if m.lastDeleted != nil || cmd == nil || m.status != "" {
		t.Errorf("u: last=%v cmd=%v status=%q", m.lastDeleted, cmd != nil, m.status)
	}
	if _, cmd = send(m, press("u")); cmd != nil {
		t.Error("u with nothing to undo is a no-op")
	}
}

func TestUndoWindowExpires(t *testing.T) {
	m := loaded(t)
	m.lastDeleted = &models.Event{ID: "x", Title: "gone"}
	m.statusTime = time.Now().Add(-undoWindow - time.Second)
	m, cmd := send(m, press("u"))
	if cmd != nil || m.lastDeleted != nil {
		t.Error("after the window, u must not restore anything")
	}

	m.lastDeleted = &models.Event{ID: "x", Title: "gone"}
	m.statusTime = time.Now().Add(-2 * time.Second) // beyond the plain 3s? no — inside undoWindow
	if _, cmd = send(m, press("u")); cmd == nil {
		t.Error("inside the window u must restore")
	}
}

func TestStatusClearsAfterThreeSeconds(t *testing.T) {
	m := loaded(t)
	m.status, m.statusTime = "Copied to clipboard", time.Now().Add(-4*time.Second)
	m, _ = send(m, press("x"))
	if m.status != "" {
		t.Errorf("status = %q, want expired", m.status)
	}
}

func TestDeleteErrorKeepsEvent(t *testing.T) {
	m := loaded(t)
	n := len(eventTitles(m))
	m.deleteTarget = cur(m)
	m, _ = send(m, eventDeletedMsg{err: errors.New("Calendar.app said no")})
	if m.err == nil || len(eventTitles(m)) != n || m.deleteTarget != nil {
		t.Errorf("err=%v events=%d target=%v", m.err, len(eventTitles(m)), m.deleteTarget)
	}
}

func TestDeleteLastRowClampsCursor(t *testing.T) {
	m := loaded(t)
	m.cursor = len(m.rows) - 1
	id := ""
	for i := len(m.rows) - 1; i >= 0; i-- {
		if e := m.rows[i].event; e != nil && e.ID != "" {
			id = e.ID
			break
		}
	}
	m, _ = send(m, eventDeletedMsg{id: id})
	if m.cursor >= len(m.rows) {
		t.Errorf("cursor %d out of range %d", m.cursor, len(m.rows))
	}
}

func TestCopyShowsStatusOnlyOnRealEvent(t *testing.T) {
	m := loaded(t)
	m.cursor = 1
	if m2, cmd := send(m, press("y")); m2.status != "" || cmd != nil {
		t.Error("y on the placeholder must do nothing")
	}
	m, _ = send(m, press("j"))
	m, cmd := send(m, press("y"))
	if m.status != "Copied to clipboard" || cmd == nil {
		t.Errorf("status=%q", m.status)
	}
}

func TestCreateFormFlow(t *testing.T) {
	m := loaded(t)
	m, _ = send(m, press("n"))
	if m.view != viewCreate || m.editTarget != nil || m.inputIdx != fTitle {
		t.Fatalf("view=%v idx=%d", m.view, m.inputIdx)
	}
	today := time.Now().Format("2006-01-02")
	if m.inputs[fDate].Value() != today {
		t.Errorf("date prefill = %q, want today", m.inputs[fDate].Value())
	}

	m, cmd := send(m, press("ctrl+s"))
	if m.err == nil || m.err.Error() != "title is required" || m.submitting || cmd != nil {
		t.Errorf("empty submit: err=%v submitting=%v", m.err, m.submitting)
	}

	m = typeText(m, "Lunch")
	m, _ = send(m, press("tab"))
	if m.inputIdx != fDate {
		t.Errorf("tab → %d", m.inputIdx)
	}
	m, _ = send(m, press("shift+tab"), press("up"))
	if m.inputIdx != fCount-1 {
		t.Errorf("up from the first field wraps to the last, got %d", m.inputIdx)
	}
	m, _ = send(m, press("down"))
	if m.inputIdx != fTitle {
		t.Errorf("down from the last wraps to the first, got %d", m.inputIdx)
	}
	m, _ = send(m, press("enter"))
	if m.inputIdx != fDate || m.submitting {
		t.Errorf("enter mid-form advances: idx=%d submitting=%v", m.inputIdx, m.submitting)
	}

	m, cmd = send(m, press("ctrl+s"))
	if !m.submitting || m.err != nil || cmd == nil {
		t.Errorf("valid submit: submitting=%v err=%v cmd=%v", m.submitting, m.err, cmd != nil)
	}

	m, _ = send(m, eventCreatedMsg{err: errors.New("Calendar not found")})
	if m.submitting || m.err == nil || m.view != viewCreate {
		t.Errorf("failed create keeps the form: submitting=%v err=%v view=%v", m.submitting, m.err, m.view)
	}
	m, cmd = send(m, eventCreatedMsg{warning: "⚠ conflicts with Review"})
	if m.view != viewList || m.err != nil || m.status != "⚠ conflicts with Review" || cmd == nil {
		t.Errorf("created: view=%v status=%q cmd=%v", m.view, m.status, cmd != nil)
	}
}

func TestCreateFormEnterOnLastFieldSubmitsAndEscCancels(t *testing.T) {
	m := loaded(t)
	m, _ = send(m, press("n"))
	m = typeText(m, "X")
	m.inputs[fTitle].Blur()
	m.inputIdx = fCount - 1
	m, cmd := send(m, press("enter"))
	if !m.submitting || cmd == nil {
		t.Error("enter on the last field submits")
	}
	m.view, m.submitting = viewCreate, false
	m, _ = send(m, press("esc"))
	if m.view != viewList || m.inputIdx != 0 {
		t.Errorf("esc: view=%v idx=%d", m.view, m.inputIdx)
	}
}

func TestEditFormPrefillsFromEvent(t *testing.T) {
	m := loaded(t)
	m, _ = send(m, press("3")) // Review
	m, _ = send(m, press("e"))
	if m.view != viewCreate || m.editTarget == nil || m.editTarget.ID != "b" {
		t.Fatalf("view=%v target=%v", m.view, m.editTarget)
	}
	want := [fCount]string{"Review", at(1, 14, 0).Format("2006-01-02"), "14:00", "1h", "Arbeit", "Büro"}
	for i, w := range want {
		if got := m.inputs[i].Value(); got != w {
			t.Errorf("field %d = %q, want %q", i, got, w)
		}
	}
	m, _ = send(m, press("esc"))
	for i := 0; i < 12 && curTitle(m) != "Offsite"; i++ {
		m, _ = send(m, press("j"))
	}
	if curTitle(m) != "Offsite" {
		t.Fatal("could not reach the all-day event")
	}
	m, _ = send(m, press("e"))
	if m.inputs[fTime].Value() != "" || m.inputs[fDuration].Value() != "" {
		t.Errorf("an all-day event has no time/duration to prefill: %q %q", m.inputs[fTime].Value(), m.inputs[fDuration].Value())
	}
}

func TestEditIsIgnoredOnPlaceholder(t *testing.T) {
	m := loaded(t)
	m.cursor = 1
	m, _ = send(m, press("e"), press("d"))
	if m.view != viewList || m.deleteTarget != nil {
		t.Error("e/d on the placeholder must do nothing")
	}
}

func TestSyncAndErrorMessages(t *testing.T) {
	m := loaded(t)
	m, cmd := send(m, press("s"))
	if !m.syncing || cmd == nil {
		t.Fatal("s starts a sync")
	}
	if _, cmd = send(m, press("s")); cmd != nil {
		t.Error("s while syncing is ignored")
	}
	m, _ = send(m, syncDoneMsg{err: errors.New("offline")})
	if m.syncing || m.err == nil || len(eventTitles(m)) != 3 {
		t.Errorf("failed sync keeps data: syncing=%v err=%v", m.syncing, m.err)
	}
	m.syncing = true
	m, _ = send(m, syncDoneMsg{events: []models.Event{{ID: "z", Title: "Fresh", StartTime: at(2, 10, 0), EndTime: at(2, 11, 0)}}})
	if m.syncing || m.err != nil || strings.Join(eventTitles(m), ",") != "Fresh" || m.lastSynced.IsZero() {
		t.Errorf("sync ok: %v %v %v", m.syncing, m.err, eventTitles(m))
	}
	m.loading, m.syncing = true, true
	m, _ = send(m, errMsg{err: errors.New("boom")})
	if m.loading || m.syncing || m.err == nil {
		t.Error("errMsg must end loading/syncing and show the error")
	}
	m, _ = send(m, lastSyncedLoadedMsg{t: time.Unix(1700000000, 0)})
	if m.lastSynced.Unix() != 1700000000 {
		t.Error("lastSyncedLoadedMsg must be stored")
	}
}

func TestCalendarPicker(t *testing.T) {
	m := loaded(t)
	config.Active.DefaultCalendar = "Privat"
	m, cmd := send(m, press("c"))
	if m.view != viewCalendarPicker || cmd == nil {
		t.Fatalf("view=%v", m.view)
	}
	m, _ = send(m, calendarsLoadedMsg{names: []string{"Arbeit", "Privat", "Familie"}})
	if m.calPickerCursor != 1 {
		t.Errorf("must preselect the current default, cursor = %d", m.calPickerCursor)
	}
	m, _ = send(m, press("j"), press("j"))
	if m.calPickerCursor != 2 {
		t.Errorf("j clamps at the last entry, got %d", m.calPickerCursor)
	}
	m, _ = send(m, press("k"), press("k"), press("k"))
	if m.calPickerCursor != 0 {
		t.Errorf("k clamps at 0, got %d", m.calPickerCursor)
	}
	m, cmd = send(m, press("enter"))
	if m.view != viewList || cmd == nil {
		t.Errorf("enter picks and returns: view=%v cmd=%v", m.view, cmd != nil)
	}
	m, _ = send(m, defaultCalendarSetMsg{name: "Arbeit"})
	if m.status != "Default calendar: Arbeit" {
		t.Errorf("status = %q", m.status)
	}

	m.view, m.availableCalendars = viewCalendarPicker, nil
	m, cmd = send(m, press("enter"))
	if m.view != viewList || cmd != nil {
		t.Error("enter with no calendars just closes the picker")
	}
	m.view = viewCalendarPicker
	m, _ = send(m, press("esc"))
	if m.view != viewList {
		t.Error("esc closes the picker")
	}
	m, _ = send(m, calendarsLoadedMsg{err: errors.New("no Calendar access")})
	if m.err == nil {
		t.Error("a failed calendar load must be shown")
	}
}

func TestHelpFreeAndQuit(t *testing.T) {
	m := loaded(t)
	m, _ = send(m, press("?"))
	if m.view != viewHelp {
		t.Fatalf("view = %v", m.view)
	}
	m, _ = send(m, press("?"))
	if m.view != viewList {
		t.Error("? again closes help")
	}
	m, _ = send(m, press("f"))
	if m.view != viewFree {
		t.Errorf("f → %v", m.view)
	}
	m, _ = send(m, press("backspace"))
	if m.view != viewList {
		t.Error("backspace leaves the free view")
	}
	if _, cmd := send(m, press("q")); cmd == nil {
		t.Error("q must quit from the list")
	}
}

func TestEventFromFormDefaultsAndExplicitValues(t *testing.T) {
	isolate(t)
	config.Active.DefaultCalendar = "Arbeit"
	in := newFormInputs()
	in[fTitle].SetValue("  Planung  ")
	e, err := eventFromForm(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if e.Title != "Planung" || e.Calendar != "Arbeit" || e.Source != "calctl" || !strings.HasPrefix(e.ID, "calctl-") {
		t.Errorf("event = %+v", e)
	}
	if e.StartTime.Format("2006-01-02 15:04") != time.Now().Format("2006-01-02")+" 09:00" || e.Duration() != time.Hour {
		t.Errorf("defaults: start %v dur %v, want today 09:00 for 1h", e.StartTime, e.Duration())
	}

	in[fDate].SetValue("2026-12-24")
	in[fTime].SetValue("18:30")
	in[fDuration].SetValue("90min")
	in[fCalendar].SetValue("Privat")
	in[fLocation].SetValue(" Zuhause ")
	e, err = eventFromForm(in, nil)
	if err != nil {
		t.Fatal(err)
	}
	if e.StartTime != time.Date(2026, 12, 24, 18, 30, 0, 0, time.Local) || e.Duration() != 90*time.Minute || e.Calendar != "Privat" || e.Location != "Zuhause" {
		t.Errorf("explicit = %+v", e)
	}
}

func TestEventFromFormRejectsBadInput(t *testing.T) {
	isolate(t)
	base := func() [fCount]textinput.Model { in := newFormInputs(); in[fTitle].SetValue("x"); return in }
	for _, c := range []struct {
		field     int
		val, want string
	}{
		{fDate, "2026-13-45", "invalid date/time"},
		{fTime, "9am", "invalid date/time"},
		{fDuration, "soon", "unrecognized duration"},
		{fDuration, "-1h", "negative"},
		{fDuration, "0", "positive"},
	} {
		in := base()
		in[c.field].SetValue(c.val)
		if _, err := eventFromForm(in, nil); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s=%q: err = %v, want %q", formLabels[c.field], c.val, err, c.want)
		}
	}
}

// Editing replaces the event (delete + create), so anything the form has no
// field for has to be carried over by hand.
func TestEventFromFormEditKeepsNotesAttendeesAndAllDay(t *testing.T) {
	isolate(t)
	orig := &models.Event{
		ID: "old", Title: "Offsite", AllDay: true, Notes: "bring tickets", Attendees: []string{"a@b.test"},
		StartTime: at(3, 0, 0), EndTime: at(5, 0, 0), Calendar: "Arbeit",
	}
	in := prefillForm(orig)
	in[fTitle].SetValue("Offsite (renamed)")
	e, err := eventFromForm(in, orig)
	if err != nil {
		t.Fatal(err)
	}
	if !e.AllDay || e.Notes != "bring tickets" || len(e.Attendees) != 1 {
		t.Errorf("edit lost data: allDay=%v notes=%q attendees=%v", e.AllDay, e.Notes, e.Attendees)
	}
	if e.StartTime.Hour() != 0 || e.Duration() != 48*time.Hour {
		t.Errorf("an untouched all-day edit must keep its span: start %v dur %v", e.StartTime, e.Duration())
	}
	if e.ID == "old" {
		t.Error("the replacement needs a fresh ID")
	}

	// giving it a real time turns it into a timed event on purpose
	in[fTime].SetValue("10:00")
	in[fDuration].SetValue("2h")
	e, err = eventFromForm(in, orig)
	if err != nil || e.AllDay || e.Duration() != 2*time.Hour {
		t.Errorf("explicit time must make it a timed event: %+v %v", e, err)
	}
}

func TestPrefillFormDurationFormats(t *testing.T) {
	for dur, want := range map[time.Duration]string{
		30 * time.Minute:            "30m",
		time.Hour:                   "1h",
		90 * time.Minute:            "1h30m",
		2*time.Hour + 5*time.Minute: "2h5m",
	} {
		e := &models.Event{Title: "x", StartTime: at(0, 9, 0), EndTime: at(0, 9, 0).Add(dur)}
		if got := prefillForm(e)[fDuration].Value(); got != want {
			t.Errorf("%v → %q, want %q", dur, got, want)
		}
	}
}

func TestBuildRowsDSTBoundaryDays(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no tzdata")
	}
	old := time.Local
	time.Local = berlin
	defer func() { time.Local = old }()

	// offset of the week that contains d
	offsetFor := func(d time.Time) int {
		days := int(startOfDay(d).Sub(weekStart(0)).Hours() / 24)
		if days < 0 {
			return (days - 6) / 7
		}
		return days / 7
	}
	count := func(rows []row, header string) (n int) {
		in := false
		for _, r := range rows {
			if r.isHeader {
				in = strings.HasSuffix(r.label, header)
				continue
			}
			if in && r.event != nil && r.event.ID == "e" {
				n++
			}
		}
		return
	}
	mk := func(start time.Time) []models.Event {
		return []models.Event{{ID: "e", Title: "x", StartTime: start, EndTime: start.Add(30 * time.Minute)}}
	}

	// Autumn: Sun 2026-10-25 is 25 hours long; 23:30 is inside the day.
	fall := time.Date(2026, 10, 25, 23, 30, 0, 0, berlin)
	rows := buildRows(mk(fall), offsetFor(fall), 7, "")
	if n := count(rows, "Oct 25"); n != 1 {
		t.Errorf("event at 23:30 on the 25-hour day shown %d times under Oct 25, want 1", n)
	}
	if n := count(rows, "Oct 26"); n != 0 {
		t.Errorf("…and %d times under Oct 26, want 0", n)
	}

	// Spring: Sun 2026-03-29 is 23 hours long; 00:30 on the 30th belongs to the 30th only.
	spring := time.Date(2026, 3, 30, 0, 30, 0, 0, berlin)
	rows = buildRows(mk(spring), offsetFor(spring), 7, "")
	if n := count(rows, "Mar 29"); n != 0 {
		t.Errorf("event on Mar 30 00:30 also listed under the 23-hour Mar 29 (%d×)", n)
	}
	if n := count(rows, "Mar 30"); n != 1 {
		t.Errorf("event shown %d times under Mar 30, want 1", n)
	}
}

func TestHelpersWordWrapRemoveAdvance(t *testing.T) {
	// widths count characters, not bytes: umlauts must not trigger early wraps
	got := wordWrap("ääää ööööö üüüüü", 20)
	if strings.Count(got, "\n") != 0 {
		t.Errorf("15 characters fit in width 20, got %q", got)
	}
	if got := wordWrap("one two three four", 12); got != "  one two\n  three four" {
		t.Errorf("wordWrap = %q", got)
	}
	if wordWrap("", 10) != "" {
		t.Error("empty text wraps to nothing")
	}

	evs := []models.Event{{ID: "1"}, {ID: "2"}, {ID: "3"}}
	if out := removeByID(evs, "2"); len(out) != 2 || out[0].ID != "1" || out[1].ID != "3" {
		t.Errorf("removeByID = %+v", out)
	}

	m := Model{rows: []row{{isHeader: true}, {isHeader: true}, {event: &models.Event{Title: "e"}}}}
	m.advanceCursorPastHeader()
	if m.cursor != 2 {
		t.Errorf("cursor = %d, want first event row", m.cursor)
	}
	m.advanceCursorPastHeader()
	if m.cursor != 2 {
		t.Error("already on an event: stay")
	}
}

func TestViewsRenderWithoutPanic(t *testing.T) {
	m := loaded(t)
	steps := []struct {
		name string
		keys []string
		want string
	}{
		{"list", nil, "Standup"},
		{"detail", []string{"3", "enter"}, "Review"},
		{"form", []string{"esc", "n"}, "New Event"},
		{"help", []string{"esc", "?"}, "calctl"},
		{"free", []string{"esc", "f"}, "Free Slots"},
		{"palette", []string{"esc", ":"}, "sync"},
		{"search", []string{"esc", "/"}, "Standup"},
	}
	for _, s := range steps {
		for _, k := range s.keys {
			m, _ = send(m, press(k))
		}
		if out := m.viewContent(); !strings.Contains(out, s.want) {
			t.Errorf("%s view lacks %q:\n%s", s.name, s.want, out)
		}
		if !m.View().AltScreen {
			t.Errorf("%s: alt screen not requested", s.name)
		}
	}

	m, _ = send(m, press("esc"), press("esc"))
	m.view = viewList
	m, _ = send(m, press("n"), press("esc"))
	m.view = viewList
	for _, label := range []struct {
		v    view
		want string
	}{{viewCreate, "New Event"}, {viewDetail, "Event Detail"}, {viewFree, "Free Slots"}, {viewHelp, "Help"}, {viewList, "Events"}} {
		m.view = label.v
		m.editTarget = nil
		if got := m.sectionLabel(); got != label.want {
			t.Errorf("sectionLabel(%v) = %q, want %q", label.v, got, label.want)
		}
	}
	m.view, m.editTarget = viewCreate, &models.Event{}
	if m.sectionLabel() != "Edit Event" {
		t.Error("editing is labelled Edit Event")
	}

	zero := New()
	if zero.viewContent() != "Loading..." {
		t.Error("before the first WindowSizeMsg the view says Loading...")
	}
}

func TestMotionThrottleDropsRapidMotionOnly(t *testing.T) {
	f := motionThrottleFilter()
	if f(nil, tea.MouseMotionMsg{}) == nil {
		t.Fatal("first motion passes")
	}
	if f(nil, tea.MouseMotionMsg{}) != nil {
		t.Error("an immediate second motion is dropped")
	}
	if f(nil, tea.MouseClickMsg{}) == nil {
		t.Error("other messages always pass")
	}
}
