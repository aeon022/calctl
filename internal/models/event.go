package models

import "time"

// Event represents a single calendar event.
type Event struct {
	ID         string    `json:"id"          yaml:"id"`
	Title      string    `json:"title"       yaml:"title"`
	StartTime  time.Time `json:"start_time"  yaml:"start_time"`
	EndTime    time.Time `json:"end_time"    yaml:"end_time"`
	AllDay     bool      `json:"all_day"     yaml:"all_day"`
	Calendar   string    `json:"calendar"    yaml:"calendar"`
	Location   string    `json:"location"    yaml:"location"`
	Notes      string    `json:"notes"       yaml:"notes"`
	Attendees  []string  `json:"attendees"   yaml:"attendees"`
	Recurrence string    `json:"recurrence"  yaml:"recurrence"` // iCalendar RRULE, e.g. "FREQ=WEEKLY;COUNT=10" — empty means no recurrence
	Timezone   string    `json:"timezone"    yaml:"timezone"`   // IANA identifier the event was created in (e.g. "America/Los_Angeles"); "" for a floating/local-time event, or when synced via the AppleScript fallback which doesn't expose this
	Source     string    `json:"source"      yaml:"source"`     // "apple" | "google"
	ExternalID string    `json:"external_id" yaml:"external_id"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Duration returns the length of the event.
func (e Event) Duration() time.Duration {
	return e.EndTime.Sub(e.StartTime)
}

// Overlaps reports whether e and other occupy any of the same time — a
// classic half-open interval overlap check ([start, end) vs [start, end)).
// All-day events never conflict (they're not a real time-slot booking),
// and an event never conflicts with itself (same ID).
func (e Event) Overlaps(other Event) bool {
	if e.ID != "" && e.ID == other.ID {
		return false
	}
	if e.AllDay || other.AllDay {
		return false
	}
	return e.StartTime.Before(other.EndTime) && other.StartTime.Before(e.EndTime)
}

// FindConflicts returns every event in existing that overlaps e.
func FindConflicts(e Event, existing []Event) []Event {
	var conflicts []Event
	for _, other := range existing {
		if e.Overlaps(other) {
			conflicts = append(conflicts, other)
		}
	}
	return conflicts
}

// FreeSlot represents a gap between events during working hours.
type FreeSlot struct {
	Start    time.Time     `json:"start"`
	End      time.Time     `json:"end"`
	Duration time.Duration `json:"duration_minutes"`
	Date     string        `json:"date"`
}

// EventImport is the Markdown frontmatter schema for calctl import.
type EventImport struct {
	Title     string   `yaml:"title"`
	Date      string   `yaml:"date"`     // "2026-10-15"
	Time      string   `yaml:"time"`     // "14:00"
	Duration  string   `yaml:"duration"` // "60min" | "1h30m" | "90"
	Calendar  string   `yaml:"calendar"`
	Location  string   `yaml:"location"`
	Attendees []string `yaml:"attendees"`
	AllDay    bool     `yaml:"all_day"`
	Notes     string   `yaml:"notes"`
}
