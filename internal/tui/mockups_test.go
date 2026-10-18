package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aeon022/calctl/internal/models"
	"github.com/aeon022/missionctl-core/tuitest"
)

// TestRenderLandingMockups writes an ANSI screenshot of the real TUI (made-up demo data) for the landing
// page: CALCTL_MOCKUP_DIR=/dir go test ./internal/tui -run TestRenderLandingMockups
func TestRenderLandingMockups(t *testing.T) {
	dir := os.Getenv("CALCTL_MOCKUP_DIR")
	if dir == "" {
		t.Skip("set CALCTL_MOCKUP_DIR to render landing mockups")
	}
	isolate(t)
	now := time.Now()
	// spread over the current week (Mon..Sun), whatever weekday it is today
	monday := now.AddDate(0, 0, -((int(now.Weekday()) + 6) % 7))
	at := func(days, h, m int) time.Time {
		return time.Date(monday.Year(), monday.Month(), monday.Day()+days, h, m, 0, 0, now.Location())
	}
	ev := func(id, title, cal, loc string, s, e time.Time) models.Event {
		return models.Event{ID: id, Title: title, Calendar: cal, Location: loc, StartTime: s, EndTime: e, Source: "apple"}
	}
	events := []models.Event{
		ev("1", "Team standup", "Work", "Zoom", at(0, 9, 30), at(0, 9, 45)),
		ev("2", "Client call — redesign", "Work", "", at(0, 14, 0), at(0, 15, 0)),
		ev("3", "Focus block", "Work", "", at(0, 16, 30), at(0, 18, 0)),
		ev("4", "Dentist", "Personal", "Annenstraße 12", at(1, 8, 15), at(1, 9, 0)),
		ev("5", "Design review", "Work", "Studio", at(1, 11, 0), at(1, 12, 0)),
		ev("6", "Lunch with Mara", "Personal", "Café Central", at(1, 12, 30), at(1, 13, 30)),
		ev("7", "Sprint planning", "Work", "", at(2, 10, 0), at(2, 11, 30)),
		ev("8", "Climbing", "Personal", "Boulderhalle", at(2, 18, 30), at(2, 20, 0)),
		ev("9", "Offsite planning", "Work", "", at(3, 0, 0), at(4, 0, 0)),
		ev("10", "Retro", "Work", "", at(4, 15, 0), at(4, 16, 0)),
		ev("11", "Farmers market", "Personal", "Lendplatz", at(5, 9, 0), at(5, 11, 0)),
		ev("12", "Hike with friends", "Personal", "Schöckl", at(6, 10, 0), at(6, 15, 0)),
	}
	events[8].AllDay = true
	m, _ := tuitest.Send(New(), tuitest.Resize(118, 26), eventsLoadedMsg{events: events})
	out := m.(Model).View().Content
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "calctl.ansi"), []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
}
