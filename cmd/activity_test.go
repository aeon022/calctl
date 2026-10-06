package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aeon022/calctl/internal/calendar"
	"github.com/aeon022/calctl/internal/models"
	"github.com/aeon022/missionctl-core/activity"
)

func stubCalendar(t *testing.T, fail error) {
	t.Helper()
	oc, od, om := calendar.CreateEvent, calendar.DeleteEvent, calendar.CreateEvents
	calendar.CreateEvent = func(*models.Event) error { return fail }
	calendar.DeleteEvent = func(*models.Event) error { return fail }
	calendar.CreateEvents = func(evs []*models.Event) ([]error, error) {
		return make([]error, len(evs)), fail
	}
	t.Cleanup(func() { calendar.CreateEvent, calendar.DeleteEvent, calendar.CreateEvents = oc, od, om })
	t.Setenv("MISSIONCTL_DATA_DIR", t.TempDir())
	t.Setenv("MISSIONCTL_ACTIVITY", "")
}

func todaysActivity(t *testing.T) []activity.Event {
	t.Helper()
	from, to := activity.Day(time.Now())
	evs, err := activity.Read(from, to)
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

func TestCLIAddIsLoggedOnceWithTitleOnly(t *testing.T) {
	setup(t)
	stubCalendar(t, nil)
	mustRun(t, "add", "Zahnarzt", "--cal", "Privat", "--notes", "Praxis Dr. Müller, Schmerzen links")
	evs := todaysActivity(t)
	if len(evs) != 1 || evs[0].Tool != "calctl" || evs[0].Action != "added" || evs[0].Title != "Zahnarzt" {
		t.Fatalf("events = %+v, want one calctl/added/Zahnarzt", evs)
	}
}

func TestCLIAddFailureAndOffLogNothing(t *testing.T) {
	setup(t)
	stubCalendar(t, errStub{})
	if _, err := run(t, "add", "Fehlschlag", "--cal", "Privat"); err == nil {
		t.Fatal("add must fail when the provider fails")
	}
	if evs := todaysActivity(t); len(evs) != 0 {
		t.Errorf("failed add logged: %+v", evs)
	}

	stubCalendar(t, nil)
	t.Setenv("MISSIONCTL_ACTIVITY", "off")
	mustRun(t, "add", "Still", "--cal", "Privat") // the action itself must still work
	if evs := todaysActivity(t); len(evs) != 0 {
		t.Errorf("MISSIONCTL_ACTIVITY=off logged: %+v", evs)
	}
}

type errStub struct{}

func (errStub) Error() string { return "stub failure" }

func TestImportLogsEachCreatedEvent(t *testing.T) {
	setup(t)
	stubCalendar(t, nil)
	dir := t.TempDir()
	for name, title := range map[string]string{"a.md": "Kickoff", "b.md": "Retro"} {
		src := "---\ntitle: " + title + "\ndate: 2026-10-15\ntime: \"14:00\"\nduration: 1h\ncalendar: Work\n---\n\ngeheimer Inhalt\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustRun(t, "import", dir)
	evs := todaysActivity(t)
	if len(evs) != 2 {
		t.Fatalf("events = %+v, want 2 (one per imported event)", evs)
	}
	got := map[string]bool{evs[0].Title: true, evs[1].Title: true}
	if !got["Kickoff"] || !got["Retro"] || evs[0].Action != "added" {
		t.Errorf("events = %+v", evs)
	}
}
