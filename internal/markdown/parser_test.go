package markdown

import (
	"testing"
	"time"

	"github.com/aeon022/calctl/internal/models"
)

func TestParseBytesFrontmatter(t *testing.T) {
	src := `---
title: Product Launch Call
date: 2026-10-15
time: "14:00"
duration: 60min
calendar: Work
attendees: [jan@example.com, lisa@example.com]
---

Discuss Q4 strategy.
`
	imp, body, err := ParseBytes([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if imp.Title != "Product Launch Call" {
		t.Errorf("unexpected title: %q", imp.Title)
	}
	if imp.Calendar != "Work" {
		t.Errorf("unexpected calendar: %q", imp.Calendar)
	}
	if len(imp.Attendees) != 2 {
		t.Errorf("unexpected attendees: %v", imp.Attendees)
	}
	if body != "Discuss Q4 strategy." {
		t.Errorf("unexpected body: %q", body)
	}
	// body becomes notes when notes not set explicitly
	if imp.Notes != "Discuss Q4 strategy." {
		t.Errorf("body not promoted to notes: %q", imp.Notes)
	}
}

func TestParseBytesNoFrontmatter(t *testing.T) {
	if _, _, err := ParseBytes([]byte("no frontmatter here")); err == nil {
		t.Fatal("want error for missing frontmatter")
	}
}

func TestToEventTimed(t *testing.T) {
	imp, _, err := ParseBytes([]byte("---\ntitle: Standup\ndate: 2026-10-15\ntime: \"09:30\"\nduration: 30min\n---\n"))
	if err != nil {
		t.Fatal(err)
	}
	e, err := ToEvent(imp)
	if err != nil {
		t.Fatal(err)
	}
	if e.StartTime.Format("2006-01-02 15:04") != "2026-10-15 09:30" {
		t.Errorf("unexpected start: %v", e.StartTime)
	}
	if e.EndTime.Sub(e.StartTime) != 30*time.Minute {
		t.Errorf("unexpected duration: %v", e.EndTime.Sub(e.StartTime))
	}
}

func TestToEventDefaults(t *testing.T) {
	imp, _, _ := ParseBytes([]byte("---\ntitle: X\ndate: 2026-10-15\n---\n"))
	e, err := ToEvent(imp)
	if err != nil {
		t.Fatal(err)
	}
	// default time 09:00, default duration 60min
	if e.StartTime.Hour() != 9 || e.StartTime.Minute() != 0 {
		t.Errorf("unexpected default start: %v", e.StartTime)
	}
	if e.EndTime.Sub(e.StartTime) != time.Hour {
		t.Errorf("unexpected default duration: %v", e.EndTime.Sub(e.StartTime))
	}
}

func TestToEventAllDay(t *testing.T) {
	imp, _, _ := ParseBytes([]byte("---\ntitle: Holiday\ndate: 2026-12-24\nall_day: true\n---\n"))
	e, err := ToEvent(imp)
	if err != nil {
		t.Fatal(err)
	}
	if !e.AllDay {
		t.Fatal("want all-day event")
	}
	if e.EndTime.Sub(e.StartTime) != 24*time.Hour {
		t.Errorf("unexpected all-day duration: %v", e.EndTime.Sub(e.StartTime))
	}
}

func TestToEventValidation(t *testing.T) {
	if _, err := ToEvent(&models.EventImport{Date: "2026-01-01"}); err == nil {
		t.Error("want error for missing title")
	}
	if _, err := ToEvent(&models.EventImport{Title: "X"}); err == nil {
		t.Error("want error for missing date")
	}
}

func TestParseDuration(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"90", 90 * time.Minute},
		{"60min", time.Hour},
		{"1h30m", 90 * time.Minute},
	}
	for _, c := range cases {
		got, err := models.ParseDuration(c.in)
		if err != nil {
			t.Errorf("ParseDuration(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseDuration(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	if _, err := models.ParseDuration("banana"); err == nil {
		t.Error("want error for invalid duration")
	}
}
