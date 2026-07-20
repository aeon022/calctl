package calendar

import (
	"testing"
	"time"

	"github.com/aeon022/calctl/internal/models"
)

func at(day time.Time, hh, mm int) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), hh, mm, 0, 0, day.Location())
}

func TestFindFreeSlotsEmptyDayIsOneBigSlot(t *testing.T) {
	day := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	wh := WorkingHours{From: "09:00", To: "18:00"}

	slots := FindFreeSlots(nil, at(day, 0, 0), at(day, 23, 59), wh, 30)

	if len(slots) != 1 {
		t.Fatalf("expected 1 free slot on an empty day, got %d: %+v", len(slots), slots)
	}
	if !slots[0].Start.Equal(at(day, 9, 0)) || !slots[0].End.Equal(at(day, 18, 0)) {
		t.Errorf("expected full working-hours slot, got %s-%s", slots[0].Start, slots[0].End)
	}
}

func TestFindFreeSlotsGapsBetweenEvents(t *testing.T) {
	day := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	wh := WorkingHours{From: "09:00", To: "18:00"}

	events := []models.Event{
		{ID: "1", StartTime: at(day, 10, 0), EndTime: at(day, 11, 0)},
		{ID: "2", StartTime: at(day, 14, 0), EndTime: at(day, 15, 0)},
	}

	slots := FindFreeSlots(events, at(day, 0, 0), at(day, 23, 59), wh, 30)

	want := []struct{ start, end time.Time }{
		{at(day, 9, 0), at(day, 10, 0)},
		{at(day, 11, 0), at(day, 14, 0)},
		{at(day, 15, 0), at(day, 18, 0)},
	}
	if len(slots) != len(want) {
		t.Fatalf("expected %d slots, got %d: %+v", len(want), len(slots), slots)
	}
	for i, w := range want {
		if !slots[i].Start.Equal(w.start) || !slots[i].End.Equal(w.end) {
			t.Errorf("slot %d: expected %s-%s, got %s-%s", i, w.start, w.end, slots[i].Start, slots[i].End)
		}
	}
}

func TestFindFreeSlotsMinDurationFiltersShortGaps(t *testing.T) {
	day := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	wh := WorkingHours{From: "09:00", To: "18:00"}

	// 15-minute gap between two back-to-back-ish meetings.
	events := []models.Event{
		{ID: "1", StartTime: at(day, 10, 0), EndTime: at(day, 11, 0)},
		{ID: "2", StartTime: at(day, 11, 15), EndTime: at(day, 12, 0)},
	}

	slots := FindFreeSlots(events, at(day, 10, 0), at(day, 12, 0), wh, 30)

	for _, s := range slots {
		if s.Duration < 30*time.Minute {
			t.Errorf("expected no slot shorter than 30 minutes, got %s (%s)", s.Duration, s.Start)
		}
	}
}

func TestFindFreeSlotsAllDayEventsAreIgnored(t *testing.T) {
	day := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	wh := WorkingHours{From: "09:00", To: "18:00"}

	events := []models.Event{
		{ID: "1", StartTime: at(day, 0, 0), EndTime: at(day, 23, 59), AllDay: true},
	}

	slots := FindFreeSlots(events, at(day, 0, 0), at(day, 23, 59), wh, 30)

	if len(slots) != 1 {
		t.Fatalf("expected all-day event to be ignored, still 1 free slot, got %d: %+v", len(slots), slots)
	}
}

func TestFindFreeSlotsOverlappingEventCollapses(t *testing.T) {
	day := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	wh := WorkingHours{From: "09:00", To: "18:00"}

	// Overlapping meetings should not produce a negative-duration "gap" between them.
	events := []models.Event{
		{ID: "1", StartTime: at(day, 10, 0), EndTime: at(day, 12, 0)},
		{ID: "2", StartTime: at(day, 11, 0), EndTime: at(day, 13, 0)},
	}

	slots := FindFreeSlots(events, at(day, 9, 0), at(day, 18, 0), wh, 30)

	for _, s := range slots {
		if s.End.Before(s.Start) {
			t.Errorf("got a slot with negative duration: %+v", s)
		}
	}
}
