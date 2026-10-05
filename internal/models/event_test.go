package models

import (
	"testing"
	"time"
)

func mkEvent(id string, startH, endH int, allDay bool) Event {
	base := time.Date(2026, 8, 10, 0, 0, 0, 0, time.Local)
	return Event{
		ID:        id,
		StartTime: base.Add(time.Duration(startH) * time.Hour),
		EndTime:   base.Add(time.Duration(endH) * time.Hour),
		AllDay:    allDay,
	}
}

func TestOverlaps(t *testing.T) {
	base := time.Date(2026, 8, 10, 0, 0, 0, 0, time.Local)
	partialB := Event{ID: "b", StartTime: base.Add(9*time.Hour + 30*time.Minute), EndTime: base.Add(10*time.Hour + 30*time.Minute)}

	cases := []struct {
		name string
		a, b Event
		want bool
	}{
		{"identical range", mkEvent("a", 9, 10, false), mkEvent("b", 9, 10, false), true},
		{"partial overlap", mkEvent("a", 9, 10, false), partialB, true},
		{"back to back, no overlap", mkEvent("a", 9, 10, false), mkEvent("b", 10, 11, false), false},
		{"fully disjoint", mkEvent("a", 9, 10, false), mkEvent("b", 14, 15, false), false},
		{"same ID never conflicts with itself", mkEvent("x", 9, 10, false), mkEvent("x", 9, 10, false), false},
		{"all-day never conflicts", mkEvent("a", 9, 10, true), mkEvent("b", 9, 10, false), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.a.Overlaps(c.b); got != c.want {
				t.Errorf("Overlaps() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestFindConflicts(t *testing.T) {
	e := mkEvent("new", 9, 10, false)
	existing := []Event{
		mkEvent("a", 8, 9, false),   // ends exactly when e starts — no overlap
		mkEvent("b", 9, 30, false),  // overlaps
		mkEvent("c", 12, 13, false), // no overlap
	}
	got := FindConflicts(e, existing)
	if len(got) != 1 || got[0].ID != "b" {
		t.Errorf("FindConflicts() = %+v, want just event b", got)
	}
}

func TestParseDurationFormatsAndRejects(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"":      0,
		"90":    90 * time.Minute,
		"60min": time.Hour,
		"1h":    time.Hour,
		"1h30m": 90 * time.Minute,
		" 45M ": 45 * time.Minute,
	} {
		got, err := ParseDuration(in)
		if err != nil || got != want {
			t.Errorf("ParseDuration(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"abc", "1x", "-30", "-1h", "-90min"} {
		if d, err := ParseDuration(bad); err == nil {
			t.Errorf("ParseDuration(%q) = %v, want an error (a negative duration would put the end before the start)", bad, d)
		}
	}
}

func TestFormatDurationAndEventDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		90 * time.Minute:            "1h30m",
		65 * time.Minute:            "1h05m",
		time.Hour:                   "1h",
		45 * time.Minute:            "45m",
		0:                           "0m",
		25 * time.Hour:              "25h",
		2*time.Hour + 5*time.Minute: "2h05m",
	} {
		if got := FormatDuration(d); got != want {
			t.Errorf("FormatDuration(%v) = %q, want %q", d, got, want)
		}
	}
	start := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	if got := (Event{StartTime: start, EndTime: start.Add(75 * time.Minute)}).Duration(); got != 75*time.Minute {
		t.Errorf("Duration = %v", got)
	}
}

func TestParseThenFormatRoundTrips(t *testing.T) {
	for _, in := range []string{"1h30m", "1h05m", "45m", "2h"} {
		d, err := ParseDuration(in)
		if err != nil {
			t.Fatal(err)
		}
		if got := FormatDuration(d); got != in {
			t.Errorf("round trip %q → %q", in, got)
		}
	}
}
