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
	Source     string    `json:"source"      yaml:"source"` // "apple" | "google"
	ExternalID string    `json:"external_id" yaml:"external_id"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Duration returns the length of the event.
func (e Event) Duration() time.Duration {
	return e.EndTime.Sub(e.StartTime)
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
	Date      string   `yaml:"date"`      // "2026-10-15"
	Time      string   `yaml:"time"`      // "14:00"
	Duration  string   `yaml:"duration"`  // "60min" | "1h30m" | "90"
	Calendar  string   `yaml:"calendar"`
	Location  string   `yaml:"location"`
	Attendees []string `yaml:"attendees"`
	AllDay    bool     `yaml:"all_day"`
	Notes     string   `yaml:"notes"`
}
