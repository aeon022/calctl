package calendar

import (
	"context"
	"fmt"
	"time"

	"github.com/aeon022/calctl/internal/models"
	"github.com/aeon022/calctl/internal/store"
	"github.com/aeon022/missionctl-core/applescript"
)

// Patch holds the fields to change; nil leaves a field as it is.
type Patch struct {
	Title, Location, Notes, Calendar *string
	Start, End                       *time.Time
}

// Empty reports whether the patch changes nothing.
func (p Patch) Empty() bool {
	return p.Title == nil && p.Location == nil && p.Notes == nil && p.Calendar == nil && p.Start == nil && p.End == nil
}

// Apply returns a copy of e with the patch applied. Moving the start without
// an explicit end keeps the duration.
func (p Patch) Apply(e models.Event) (models.Event, error) {
	n := e
	if p.Title != nil {
		n.Title = *p.Title
	}
	if p.Location != nil {
		n.Location = *p.Location
	}
	if p.Notes != nil {
		n.Notes = *p.Notes
	}
	if p.Calendar != nil {
		n.Calendar = *p.Calendar
	}
	if p.Start != nil {
		n.StartTime = *p.Start
		n.EndTime = n.StartTime.Add(e.Duration())
	}
	if p.End != nil {
		n.EndTime = *p.End
	}
	if n.Title == "" {
		return e, fmt.Errorf("title must not be empty")
	}
	if !n.AllDay && !n.EndTime.After(n.StartTime) {
		return e, fmt.Errorf("end %s must be after start %s", n.EndTime.Format("15:04"), n.StartTime.Format("15:04"))
	}
	return n, nil
}

// UpdateEvent writes old → new to the calendar. A variable so tests never touch Calendar.app.
var UpdateEvent = updateEvent

func updateEvent(old, n *models.Event) error {
	if n.Calendar != old.Calendar {
		// Calendar.app cannot move an event between calendars: create the copy, then remove the original.
		if err := CreateEvent(n); err != nil {
			return err
		}
		return DeleteEvent(old)
	}
	_, err := applescript.Run(buildUpdateScript(old, n))
	return err
}

func buildUpdateScript(old, n *models.Event) string {
	// Set whichever end moves away from the old range first so start never passes end.
	order := []string{"start date", "end date"}
	if !n.StartTime.Before(old.EndTime) {
		order = []string{"end date", "start date"}
	}
	vars := map[string]time.Time{"start date": n.StartTime, "end date": n.EndTime}
	dates, sets := "", ""
	for i, prop := range order {
		v := fmt.Sprintf("newDate%d", i)
		dates += appleScriptSetDate(v, vars[prop]) + "\n"
		sets += fmt.Sprintf("\tset %s of evt to %s\n", prop, v)
	}
	return fmt.Sprintf(`%s
%s
set matches to {}
tell application "Calendar"
	repeat with c in calendars
		if name of c is "%s" then
			set matches to matches & (every event of c whose summary = "%s" and start date = targetDate)
		end if
	end repeat
	if (count of matches) is 0 then error "no matching event found"
	if (count of matches) > 1 then error "more than one matching event"
	set evt to item 1 of matches
	set summary of evt to "%s"
	set location of evt to "%s"
	set description of evt to "%s"
%s	reload calendars
end tell
return "ok"`,
		appleScriptSetDate("targetDate", old.StartTime), dates,
		applescript.Escape(old.Calendar), applescript.Escape(old.Title),
		applescript.Escape(n.Title), applescript.Escape(n.Location), applescript.Escape(n.Notes),
		sets)
}

// Result describes a finished update.
type Result struct {
	Old, New  models.Event
	Conflicts []models.Event
}

// Update finds the one event with this title on this day (optionally in one
// calendar), applies the patch, writes it and refreshes the cache. Overlaps
// with other events are reported, not blocked.
func Update(s *store.Store, title string, day time.Time, cal string, p Patch) (*Result, error) {
	if p.Empty() {
		return nil, fmt.Errorf("nothing to change: pass at least one field")
	}
	ctx := context.Background()
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.Local)
	events, err := s.ListEvents(ctx, start, start.Add(24*time.Hour-time.Second))
	if err != nil {
		return nil, err
	}
	var hits []models.Event
	for _, e := range events {
		if e.Title == title && (cal == "" || e.Calendar == cal) {
			hits = append(hits, e)
		}
	}
	switch len(hits) {
	case 0:
		return nil, fmt.Errorf("event %q on %s not found in cache — run sync first", title, start.Format("2006-01-02"))
	case 1:
	default:
		return nil, fmt.Errorf("%d events named %q on %s — pass the calendar to pick one", len(hits), title, start.Format("2006-01-02"))
	}
	old := hits[0]
	n, err := p.Apply(old)
	if err != nil {
		return nil, err
	}
	if err := UpdateEvent(&old, &n); err != nil {
		return nil, fmt.Errorf("update failed: %w", err)
	}
	n.UpdatedAt = time.Now()
	_ = s.DeleteByID(ctx, old.ID)
	n.ID, n.Source = "calctl-upd-"+fmt.Sprint(n.StartTime.UnixNano()), "calctl" // the next sync replaces this echo
	_ = s.UpsertEvent(ctx, &n)

	r := &Result{Old: old, New: n}
	if !n.AllDay {
		day0 := time.Date(n.StartTime.Year(), n.StartTime.Month(), n.StartTime.Day(), 0, 0, 0, 0, time.Local)
		if same, err := s.ListEvents(ctx, day0, day0.Add(24*time.Hour-time.Second)); err == nil {
			r.Conflicts = models.FindConflicts(n, same)
		}
	}
	return r, nil
}

// ParseWhen reads "2026-10-31T14:00[:00]" or "2026-10-31 14:00" in local time.
func ParseWhen(s string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time %q (use YYYY-MM-DDTHH:MM)", s)
}
