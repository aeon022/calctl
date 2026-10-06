package mcpserver

import (
	"testing"
	"time"

	"github.com/aeon022/calctl/internal/calendar"
	"github.com/aeon022/calctl/internal/models"
	"github.com/aeon022/missionctl-core/activity"
)

func TestMCPCreateAndDeleteAreLogged(t *testing.T) {
	start := setupTestDB(t) // temp DB seeded with "Standup" today at 10:00
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MISSIONCTL_DATA_DIR", t.TempDir())
	t.Setenv("MISSIONCTL_ACTIVITY", "")
	oc, od := calendar.CreateEvent, calendar.DeleteEvent
	calendar.CreateEvent = func(*models.Event) error { return nil }
	calendar.DeleteEvent = func(*models.Event) error { return nil }
	t.Cleanup(func() { calendar.CreateEvent, calendar.DeleteEvent = oc, od })

	callTool(t, handleCreateEvent, map[string]any{
		"title": "Review", "start": start.Add(2 * time.Hour).Format("2006-01-02T15:04:05"),
		"end": start.Add(3 * time.Hour).Format("2006-01-02T15:04:05"), "calendar": "Work", "notes": "vertraulich",
	})
	callTool(t, handleDeleteEvent, map[string]any{"title": "Standup", "date": start.Format("2006-01-02")})

	from, to := activity.Day(time.Now())
	evs, err := activity.Read(from, to)
	if err != nil || len(evs) != 2 {
		t.Fatalf("events = %+v, %v", evs, err)
	}
	if evs[0].Action != "added" || evs[0].Title != "Review" || evs[1].Action != "deleted" || evs[1].Title != "Standup" || evs[1].Tool != "calctl" {
		t.Errorf("events = %+v", evs)
	}
}
