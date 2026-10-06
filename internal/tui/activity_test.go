package tui

import (
	"path/filepath"
	"testing"
	"time"

	"charm.land/bubbles/v2/textinput"
	"github.com/aeon022/calctl/internal/calendar"
	"github.com/aeon022/calctl/internal/config"
	"github.com/aeon022/calctl/internal/models"
	"github.com/aeon022/missionctl-core/activity"
)

func actSandbox(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MISSIONCTL_DATA_DIR", t.TempDir())
	t.Setenv("MISSIONCTL_ACTIVITY", "")
	config.DBPathOverride = filepath.Join(t.TempDir(), "calctl.db")
	oc, od := calendar.CreateEvent, calendar.DeleteEvent
	calendar.CreateEvent = func(*models.Event) error { return nil }
	calendar.DeleteEvent = func(*models.Event) error { return nil }
	t.Cleanup(func() {
		config.DBPathOverride = ""
		calendar.CreateEvent, calendar.DeleteEvent = oc, od
	})
}

func logToday(t *testing.T) []activity.Event {
	t.Helper()
	from, to := activity.Day(time.Now())
	evs, err := activity.Read(from, to)
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

func form(title string) [fCount]textinput.Model {
	var in [fCount]textinput.Model
	for i := range in {
		in[i] = textinput.New()
	}
	in[fTitle].SetValue(title)
	in[fCalendar].SetValue("Arbeit")
	return in
}

func TestTUIAddAndDeleteAreLogged(t *testing.T) {
	actSandbox(t)
	if msg := createEventCmd(form("Retro"), nil)(); msg.(eventCreatedMsg).err != nil {
		t.Fatalf("create: %v", msg)
	}
	e := &models.Event{ID: "x", Title: "Retro", StartTime: time.Now(), EndTime: time.Now().Add(time.Hour), Calendar: "Arbeit"}
	deleteEventCmd(e)()
	evs := logToday(t)
	if len(evs) != 2 || evs[0].Action != "added" || evs[0].Title != "Retro" || evs[1].Action != "deleted" || evs[1].Title != "Retro" {
		t.Errorf("events = %+v", evs)
	}
}

func TestTUIEditIsNotAnAdd(t *testing.T) {
	actSandbox(t)
	orig := &models.Event{ID: "old", Title: "Alt", StartTime: time.Now(), EndTime: time.Now().Add(time.Hour), Calendar: "Arbeit"}
	if msg := createEventCmd(form("Neu"), orig)(); msg.(eventCreatedMsg).err != nil {
		t.Fatalf("edit: %+v", msg)
	}
	if evs := logToday(t); len(evs) != 0 {
		t.Errorf("an edit re-creates the event but is not an add: %+v", evs)
	}
}

func TestTUIActivityOffStillActs(t *testing.T) {
	actSandbox(t)
	t.Setenv("MISSIONCTL_ACTIVITY", "off")
	if msg := createEventCmd(form("Ruhig"), nil)(); msg.(eventCreatedMsg).err != nil {
		t.Fatalf("create must succeed with logging off: %+v", msg)
	}
	if evs := logToday(t); len(evs) != 0 {
		t.Errorf("logged while off: %+v", evs)
	}
}
