package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/aeon022/calctl/internal/models"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "calctl.db")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func sampleEvent(id string, start time.Time) *models.Event {
	return &models.Event{
		ID:        id,
		Title:     "Event " + id,
		StartTime: start,
		EndTime:   start.Add(time.Hour),
		Calendar:  "Work",
		Source:    "apple",
		CreatedAt: start,
		UpdatedAt: start,
	}
}

func TestUpsertAndListEvents(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	day := time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC)

	if err := s.UpsertEvent(ctx, sampleEvent("1", day)); err != nil {
		t.Fatalf("UpsertEvent: %v", err)
	}
	if err := s.UpsertEvent(ctx, sampleEvent("2", day.Add(2*time.Hour))); err != nil {
		t.Fatalf("UpsertEvent: %v", err)
	}

	events, err := s.ListEvents(ctx, day.Add(-time.Hour), day.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].ID != "1" || events[1].ID != "2" {
		t.Errorf("expected events ordered by start time, got %s, %s", events[0].ID, events[1].ID)
	}
}

func TestUpsertEventIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	day := time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC)

	e := sampleEvent("dup", day)
	if err := s.UpsertEvent(ctx, e); err != nil {
		t.Fatalf("UpsertEvent: %v", err)
	}
	e.Title = "Renamed"
	if err := s.UpsertEvent(ctx, e); err != nil {
		t.Fatalf("UpsertEvent (update): %v", err)
	}

	events, err := s.ListEvents(ctx, day.Add(-time.Hour), day.Add(time.Hour))
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event after upsert-update, got %d", len(events))
	}
	if events[0].Title != "Renamed" {
		t.Errorf("expected renamed title, got %q", events[0].Title)
	}
}

func TestListEventsRespectsRange(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	day := time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC)

	if err := s.UpsertEvent(ctx, sampleEvent("in-range", day)); err != nil {
		t.Fatalf("UpsertEvent: %v", err)
	}
	if err := s.UpsertEvent(ctx, sampleEvent("out-of-range", day.AddDate(0, 0, 10))); err != nil {
		t.Fatalf("UpsertEvent: %v", err)
	}

	events, err := s.ListEvents(ctx, day.Add(-time.Hour), day.Add(time.Hour))
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 1 || events[0].ID != "in-range" {
		t.Fatalf("expected only in-range event, got %+v", events)
	}
}

func TestDeleteByID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	day := time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC)

	if err := s.UpsertEvent(ctx, sampleEvent("gone", day)); err != nil {
		t.Fatalf("UpsertEvent: %v", err)
	}
	if err := s.DeleteByID(ctx, "gone"); err != nil {
		t.Fatalf("DeleteByID: %v", err)
	}
	events, err := s.ListEvents(ctx, day.Add(-time.Hour), day.Add(time.Hour))
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events after delete, got %d", len(events))
	}
}

func TestDeleteBySource(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	day := time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC)

	apple := sampleEvent("apple-1", day)
	apple.Source = "apple"
	google := sampleEvent("google-1", day.Add(time.Hour))
	google.Source = "google"

	for _, e := range []*models.Event{apple, google} {
		if err := s.UpsertEvent(ctx, e); err != nil {
			t.Fatalf("UpsertEvent: %v", err)
		}
	}

	if err := s.DeleteBySource(ctx, "apple", day.Add(-time.Hour), day.Add(24*time.Hour)); err != nil {
		t.Fatalf("DeleteBySource: %v", err)
	}
	events, err := s.ListEvents(ctx, day.Add(-time.Hour), day.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 1 || events[0].ID != "google-1" {
		t.Fatalf("expected only google-1 to remain, got %+v", events)
	}
}

func TestAllDayEventRoundTrips(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	day := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)

	e := sampleEvent("allday", day)
	e.AllDay = true
	e.Attendees = []string{"a@example.com", "b@example.com"}
	if err := s.UpsertEvent(ctx, e); err != nil {
		t.Fatalf("UpsertEvent: %v", err)
	}

	events, err := s.ListEvents(ctx, day.Add(-time.Hour), day.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if !events[0].AllDay {
		t.Error("expected AllDay to round-trip as true")
	}
	if len(events[0].Attendees) != 2 {
		t.Errorf("expected 2 attendees to round-trip, got %d", len(events[0].Attendees))
	}
}
