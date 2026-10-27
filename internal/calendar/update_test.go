package calendar

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aeon022/calctl/internal/models"
	"github.com/aeon022/calctl/internal/store"
)

func str(s string) *string { return &s }

func seed(t *testing.T) (*store.Store, time.Time) {
	t.Helper()
	s, err := store.New(filepath.Join(t.TempDir(), "c.db"), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	day := time.Date(2026, 11, 3, 0, 0, 0, 0, time.Local)
	for _, e := range []models.Event{
		{ID: "a", Title: "Zahnarzt", StartTime: day.Add(10 * time.Hour), EndTime: day.Add(11 * time.Hour), Calendar: "Privat", Source: "apple"},
		{ID: "b", Title: "Call", StartTime: day.Add(14 * time.Hour), EndTime: day.Add(15 * time.Hour), Calendar: "Arbeit", Source: "apple"},
	} {
		e := e
		if err := s.UpsertEvent(context.Background(), &e); err != nil {
			t.Fatal(err)
		}
	}
	return s, day
}

func TestUpdateMovesKeepsDurationAndWarns(t *testing.T) {
	s, day := seed(t)
	var gotOld, gotNew models.Event
	orig := UpdateEvent
	UpdateEvent = func(o, n *models.Event) error { gotOld, gotNew = *o, *n; return nil }
	defer func() { UpdateEvent = orig }()

	start := day.Add(14*time.Hour + 30*time.Minute)
	r, err := Update(s, "Zahnarzt", day, "", Patch{Start: &start, Location: str("Praxis")})
	if err != nil {
		t.Fatal(err)
	}
	if gotOld.Title != "Zahnarzt" || !gotNew.EndTime.Equal(start.Add(time.Hour)) || gotNew.Location != "Praxis" {
		t.Fatalf("old=%+v new=%+v", gotOld, gotNew)
	}
	if len(r.Conflicts) != 1 || r.Conflicts[0].Title != "Call" {
		t.Fatalf("conflicts=%+v", r.Conflicts)
	}
	if !strings.Contains(r.New.Describe(), "@ Praxis") {
		t.Fatal(r.New.Describe())
	}
	// cache now has the new time, not the old one
	evs, _ := s.ListEvents(context.Background(), day, day.Add(24*time.Hour))
	for _, e := range evs {
		if e.Title == "Zahnarzt" && !e.StartTime.Equal(start) {
			t.Fatalf("stale cache: %v", e.StartTime)
		}
	}
}

func TestUpdateErrors(t *testing.T) {
	s, day := seed(t)
	orig := UpdateEvent
	UpdateEvent = func(*models.Event, *models.Event) error { return nil }
	defer func() { UpdateEvent = orig }()

	if _, err := Update(s, "Zahnarzt", day, "", Patch{}); err == nil {
		t.Error("empty patch should fail")
	}
	if _, err := Update(s, "Nope", day, "", Patch{Title: str("x")}); err == nil {
		t.Error("unknown event should fail")
	}
	if _, err := Update(s, "Zahnarzt", day, "Arbeit", Patch{Title: str("x")}); err == nil {
		t.Error("wrong calendar should not match")
	}
	early := day.Add(9 * time.Hour)
	if _, err := Update(s, "Zahnarzt", day, "", Patch{End: &early}); err == nil {
		t.Error("end before start should fail")
	}
}

func TestUpdateScriptCompiles(t *testing.T) {
	if _, err := exec.LookPath("osacompile"); err != nil {
		t.Skip("no osacompile")
	}
	d := time.Date(2026, 11, 3, 10, 0, 0, 0, time.Local)
	old := &models.Event{Title: `Za"hn`, Calendar: "Privat", StartTime: d, EndTime: d.Add(time.Hour)}
	for _, shift := range []time.Duration{30 * time.Minute, 48 * time.Hour} { // overlapping and later-than-old-end
		n := *old
		n.StartTime, n.EndTime = d.Add(shift), d.Add(shift+time.Hour)
		n.Notes = "mit \"Zitat\"\nzweite Zeile"
		out := filepath.Join(t.TempDir(), "x.scpt")
		cmd := exec.Command("osacompile", "-o", out, "-e", buildUpdateScript(old, &n))
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s\n%s", err, b, buildUpdateScript(old, &n))
		}
	}
}
