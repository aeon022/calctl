package tui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/aeon022/calctl/internal/calendar"
	"github.com/aeon022/calctl/internal/config"
	"github.com/aeon022/calctl/internal/models"
	"github.com/aeon022/calctl/internal/store"
	"github.com/google/uuid"
)

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

// eventFromForm builds the event a submitted form describes. Editing replaces
// the event (delete + create), so fields the form has no input for — notes,
// attendees — carry over from the original, and an all-day event whose time
// and duration were left untouched stays all-day instead of silently becoming
// a 1-hour 09:00 event.
func eventFromForm(inputs [fCount]textinput.Model, editTarget *models.Event) (*models.Event, error) {
	title := strings.TrimSpace(inputs[fTitle].Value())
	dateStr := strings.TrimSpace(inputs[fDate].Value())
	timeStr := strings.TrimSpace(inputs[fTime].Value())
	durStr := strings.TrimSpace(inputs[fDuration].Value())
	calName := strings.TrimSpace(inputs[fCalendar].Value())
	if calName == "" {
		calName = config.Active.DefaultCalendar
	}

	keepAllDay := editTarget != nil && editTarget.AllDay && timeStr == "" && durStr == ""

	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}
	if timeStr == "" {
		timeStr = "09:00"
		if keepAllDay {
			timeStr = "00:00"
		}
	}
	if durStr == "" {
		durStr = "1h"
	}

	start, err := time.ParseInLocation("2006-01-02 15:04", dateStr+" "+timeStr, time.Local)
	if err != nil {
		return nil, fmt.Errorf("invalid date/time: %w", err)
	}
	dur, err := models.ParseDuration(durStr)
	if err != nil {
		return nil, err
	}
	if dur <= 0 {
		return nil, fmt.Errorf("duration must be positive")
	}
	if keepAllDay {
		if dur = editTarget.EndTime.Sub(editTarget.StartTime); dur <= 0 {
			dur = 24 * time.Hour
		}
	}

	e := &models.Event{
		ID:        "calctl-" + uuid.New().String(),
		Title:     title,
		StartTime: start,
		EndTime:   start.Add(dur),
		AllDay:    keepAllDay,
		Calendar:  calName,
		Location:  strings.TrimSpace(inputs[fLocation].Value()),
		Source:    "calctl",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if editTarget != nil {
		e.Notes, e.Attendees = editTarget.Notes, editTarget.Attendees
	}
	return e, nil
}

func createEventCmd(inputs [fCount]textinput.Model, editTarget *models.Event) tea.Cmd {
	return func() tea.Msg {
		e, err := eventFromForm(inputs, editTarget)
		if err != nil {
			return eventCreatedMsg{err: err}
		}
		start := e.StartTime

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
