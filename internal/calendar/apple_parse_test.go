package calendar

import (
	"strings"
	"testing"
	"time"

	"github.com/aeon022/calctl/internal/models"
)

func TestParseEvents(t *testing.T) {
	raw := `---EVENT---
TITLE: Standup
START: 2026-10-06T09:30:00
END: 2026-10-06T10:00:00
CAL: Arbeit
LOC: missing value
ALLDAY: 0
UID: ABC-1
TZ: Europe/Berlin
---EVENT---
TITLE: Urlaub: Italien
START: 2026-10-10T00:00:00
END: 2026-10-11T00:00:00
CAL: Privat
LOC: Rom
ALLDAY: 1
UID:
---EVENT---
START: 2026-10-12T08:00:00
CAL: Privat
---EVENT---
garbage without colon
TITLE: Ohne Zeiten
`
	evs := parseEvents(raw)
	if len(evs) != 3 {
		t.Fatalf("got %d events, want 3 (title-less block dropped): %+v", len(evs), evs)
	}

	a := evs[0]
	if a.Title != "Standup" || a.Calendar != "Arbeit" || a.ExternalID != "ABC-1" || a.ID != "apple-ABC-1" || a.Timezone != "Europe/Berlin" {
		t.Errorf("first event fields: %+v", a)
	}
	if a.Location != "" {
		t.Errorf("Location = %q, AppleScript's \"missing value\" must become empty", a.Location)
	}
	if a.AllDay {
		t.Error("ALLDAY:0 parsed as all-day")
	}
	if want := time.Date(2026, 10, 6, 9, 30, 0, 0, time.Local); !a.StartTime.Equal(want) {
		t.Errorf("StartTime = %v, want %v", a.StartTime, want)
	}
	if a.Duration() != 30*time.Minute {
		t.Errorf("Duration = %v", a.Duration())
	}

	b := evs[1]
	if b.Title != "Urlaub: Italien" {
		t.Errorf("Title = %q — only the first colon separates key from value", b.Title)
	}
	if !b.AllDay || b.Location != "Rom" {
		t.Errorf("second event: %+v", b)
	}
	if !strings.HasPrefix(b.ID, "apple-") || b.ID == "apple-" || b.ExternalID != "" {
		t.Errorf("empty UID must get a generated ID, got ID=%q ext=%q", b.ID, b.ExternalID)
	}
	if evs[2].Title != "Ohne Zeiten" || !evs[2].StartTime.IsZero() {
		t.Errorf("third event: %+v", evs[2])
	}
}

func TestParseEventsEmpty(t *testing.T) {
	if got := parseEvents("  \n"); len(got) != 0 {
		t.Errorf("parseEvents(blank) = %+v", got)
	}
}

func TestBuildCreateScript(t *testing.T) {
	e := &models.Event{
		Title:      `Dr. "Müller" \ Termin`,
		Calendar:   "Arbeit",
		Location:   `Büro "3"`,
		Notes:      "Notiz",
		Recurrence: "FREQ=WEEKLY;COUNT=3",
		AllDay:     true,
		StartTime:  time.Date(2026, 10, 6, 9, 0, 0, 0, time.Local),
		EndTime:    time.Date(2026, 10, 6, 10, 0, 0, 0, time.Local),
	}
	s := buildCreateScript(e)

	for _, want := range []string{
		`summary:"Dr. \"Müller\" \\ Termin"`, // quotes/backslashes escaped, can't break out of the literal
		`set location of newEvent0 to "Büro \"3\""`,
		`set description of newEvent0 to "Notiz"`,
		`set allday event of newEvent0 to true`,
		`set recurrence of newEvent0 to "FREQ=WEEKLY;COUNT=3"`,
		`if name of c is "Arbeit"`,
		`error "Calendar not found: Arbeit"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("script missing %q\n%s", want, s)
		}
	}
	if !strings.HasSuffix(s, "tell application \"Calendar\" to reload calendars\n") {
		t.Error("standalone create must end with a calendar reload")
	}
}

func TestBuildCreateScriptOmitsEmptyOptionals(t *testing.T) {
	e := &models.Event{Title: "X", Calendar: "C", StartTime: time.Now(), EndTime: time.Now().Add(time.Hour)}
	s := buildCreateScriptIndexed(e, 3)
	for _, bad := range []string{"set location", "set description", "allday event", "set recurrence"} {
		if strings.Contains(s, bad) {
			t.Errorf("unset field emitted %q", bad)
		}
	}
	// batch creates share one script: variable names must be unique per index
	if !strings.Contains(s, "newEvent3") || strings.Contains(s, "newEvent0") || strings.Contains(s, "reload") {
		t.Errorf("indexed script should use idx 3 and not reload:\n%s", s)
	}
}
