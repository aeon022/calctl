package calendar

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/aeon022/calctl/internal/models"
	"github.com/google/uuid"
)

// FetchEvents returns all Apple Calendar events between from and to.
func FetchEvents(from, to time.Time) ([]models.Event, error) {
	script := buildFetchScript(from, to)
	out, err := runAppleScript(script)
	if err != nil {
		return nil, fmt.Errorf("applescript: %w", err)
	}
	return parseEvents(out), nil
}

// CreateEvent creates a new event in Apple Calendar.
func CreateEvent(e *models.Event) error {
	script := buildCreateScript(e)
	_, err := runAppleScript(script)
	return err
}

// ListCalendars returns all calendar names from Apple Calendar.
func ListCalendars() ([]string, error) {
	script := `
tell application "Calendar"
	set names to {}
	repeat with c in calendars
		set end of names to name of c
	end repeat
	return names
end tell`
	out, err := runAppleScript(script)
	if err != nil {
		return nil, err
	}
	var cals []string
	for _, name := range strings.Split(out, ", ") {
		name = strings.TrimSpace(name)
		if name != "" {
			cals = append(cals, name)
		}
	}
	return cals, nil
}

// buildFetchScript produces a locale-independent AppleScript that dumps events.
//
// Date output strategy: use numeric date component properties (year, month, day,
// hours, minutes, seconds) to build an ISO-8601 string — fully locale-independent
// and always in local time, matching exactly what Apple Calendar displays.
// This avoids all epoch arithmetic and DST ambiguity.
//
// Output format per event (separated by "---EVENT---"):
//
//	TITLE:<title>
//	START:YYYY-MM-DDTHH:MM:SS   (local time)
//	END:YYYY-MM-DDTHH:MM:SS     (local time)
//	CAL:<calendar name>
//	LOC:<location>
//	ALLDAY:<0|1>
//	UID:<uid>
func buildFetchScript(from, to time.Time) string {
	fromEpoch := from.Unix()
	toEpoch := to.Unix()

	return fmt.Sprintf(`
-- Build from/to dates using second offsets from now — locale-independent.
set nowUnix to (do shell script "date '+%%s'") as integer
set fromDate to (current date) + (%d - nowUnix)
set toDate   to (current date) + (%d - nowUnix)

-- Zero-pad helper: returns a 2-digit string for a number 0-59
on zp(n)
	if n < 10 then
		return "0" & (n as text)
	else
		return (n as text)
	end if
end zp

-- Format a date object as "YYYY-MM-DDTHH:MM:SS" using numeric properties only.
-- This is always local time and locale-independent.
on isoDate(d)
	set yr to (year of d) as text
	set mo to my zp(month of d as integer)
	set dy to my zp(day of d)
	set hr to my zp(hours of d)
	set mi to my zp(minutes of d)
	set sc to my zp(seconds of d)
	return yr & "-" & mo & "-" & dy & "T" & hr & ":" & mi & ":" & sc
end isoDate

set output to ""
tell application "Calendar"
	repeat with cal in calendars
		set calName to name of cal
		set evts to (every event of cal whose start date >= fromDate and start date <= toDate)
		repeat with evt in evts
			set t to summary of evt
			set s to my isoDate(start date of evt)
			set e to my isoDate(end date of evt)
			set evtLoc to ""
			try
				if location of evt is not missing value then
					set evtLoc to location of evt
				end if
			end try
			set evtAD to 0
			try
				if allday event of evt then set evtAD to 1
			end try
			-- "uid" is a Calendar property name — use evtUID to avoid collision
			set evtUID to ""
			try
				set evtUID to uid of evt
			end try
			set output to output & "TITLE:" & t & "\nSTART:" & s & "\nEND:" & e & "\nCAL:" & calName & "\nLOC:" & evtLoc & "\nALLDAY:" & evtAD & "\nUID:" & evtUID & "\n---EVENT---\n"
		end repeat
	end repeat
end tell
return output
`, fromEpoch, toEpoch)
}

func buildCreateScript(e *models.Event) string {
	calName := e.Calendar
	if calName == "" {
		calName = "Calendar"
	}

	locationLine := ""
	if e.Location != "" {
		locationLine = fmt.Sprintf(`set location of newEvent to "%s"`, escapeAppleScript(e.Location))
	}
	notesLine := ""
	if e.Notes != "" {
		notesLine = fmt.Sprintf(`set description of newEvent to "%s"`, escapeAppleScript(e.Notes))
	}
	allDayLine := ""
	if e.AllDay {
		allDayLine = "set allday event of newEvent to true"
	}

	// Build ISO strings for start/end in local time — AppleScript reads them back correctly.
	startISO := e.StartTime.Format("2006-01-02T15:04:05")
	endISO := e.EndTime.Format("2006-01-02T15:04:05")

	return fmt.Sprintf(`
-- Parse ISO date strings via shell into AppleScript date objects (locale-independent).
set startDate to (current date) + ((do shell script "date -jf '%%Y-%%m-%%dT%%H:%%M:%%S' '%s' '+%%s'") as integer - (do shell script "date '+%%s'") as integer)
set endDate   to (current date) + ((do shell script "date -jf '%%Y-%%m-%%dT%%H:%%M:%%S' '%s' '+%%s'") as integer - (do shell script "date '+%%s'") as integer)

tell application "Calendar"
	tell calendar "%s"
		set newEvent to make new event with properties {summary:"%s", start date:startDate, end date:endDate}
		%s
		%s
		%s
	end tell
	reload calendars
end tell
`, startISO, endISO, escapeAppleScript(calName), escapeAppleScript(e.Title),
		locationLine, notesLine, allDayLine)
}

func parseEvents(raw string) []models.Event {
	var events []models.Event
	blocks := strings.Split(raw, "---EVENT---")

	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}

		e := models.Event{
			Source:    "apple",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			key, val, found := strings.Cut(line, ":")
			if !found {
				continue
			}
			val = strings.TrimSpace(val)
			switch key {
			case "TITLE":
				e.Title = val
			case "START":
				// AppleScript outputs local time as "YYYY-MM-DDTHH:MM:SS".
				// Parse as local time to match exactly what Apple Calendar shows.
				if t, err := time.ParseInLocation("2006-01-02T15:04:05", val, time.Local); err == nil {
					e.StartTime = t
				}
			case "END":
				if t, err := time.ParseInLocation("2006-01-02T15:04:05", val, time.Local); err == nil {
					e.EndTime = t
				}
			case "CAL":
				e.Calendar = val
			case "LOC":
				if val != "missing value" {
					e.Location = val
				}
			case "ALLDAY":
				e.AllDay = val == "1"
			case "UID":
				e.ExternalID = val
				if val != "" {
					e.ID = "apple-" + val
				}
			}
		}

		if e.Title == "" {
			continue
		}
		if e.ID == "" {
			e.ID = "apple-" + uuid.New().String()
		}
		events = append(events, e)
	}
	return events
}

func runAppleScript(script string) (string, error) {
	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("osascript error: %s", string(exitErr.Stderr))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
