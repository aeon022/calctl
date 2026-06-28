package calendar

import (
	"fmt"
	"os/exec"
	"strconv"
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
// Key insight: on German macOS, `(date) as integer` fails because AppleScript routes
// the coercion through a locale-dependent string representation. Instead we use:
//   - `(current date) + N`  to build dates from second offsets   (date arithmetic ✓)
//   - `date1 - date2`       to extract seconds between two dates  (always numeric ✓)
//
// We anchor against a reference date built from numeric properties (not strings),
// then add the known Unix timestamp of AppleScript's epoch via one shell call.
//
// Output format per event (separated by "---EVENT---"):
//
//	TITLE:<title>
//	START:<unix timestamp>
//	END:<unix timestamp>
//	CAL:<calendar name>
//	LOC:<location>
//	ALLDAY:<0|1>
//	UID:<uid>
func buildFetchScript(from, to time.Time) string {
	fromEpoch := from.Unix()
	toEpoch := to.Unix()

	return fmt.Sprintf(`
-- Build AppleScript's epoch date using numeric properties — no locale string needed.
-- January is the AppleScript constant 1 regardless of locale.
set appleEpochDate to current date
set year of appleEpochDate to 2001
set month of appleEpochDate to January
set day of appleEpochDate to 1
set time of appleEpochDate to 0

-- Get the Unix timestamp of that epoch moment via shell (also locale-independent).
set appleEpochUnix to (do shell script "date -jf '%%Y-%%m-%%d %%H:%%M:%%S' '2001-01-01 00:00:00' '+%%s'") as integer

-- Build from/to dates using second offsets from now — avoids any date→string coercion.
set nowUnix to (do shell script "date '+%%s'") as integer
set fromDate to (current date) + (%d - nowUnix)
set toDate   to (current date) + (%d - nowUnix)

set output to ""
tell application "Calendar"
	repeat with cal in calendars
		set calName to name of cal
		set evts to (every event of cal whose start date >= fromDate and start date <= toDate)
		repeat with evt in evts
			set t to summary of evt
			-- date - date = seconds (pure integer arithmetic, no locale involved)
			set s to ((start date of evt) - appleEpochDate) + appleEpochUnix
			set e to ((end date of evt) - appleEpochDate) + appleEpochUnix
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
			-- "uid" is a Calendar property name — use evtUID to avoid name collision
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

	startUnix := e.StartTime.Unix()
	endUnix := e.EndTime.Unix()

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

	return fmt.Sprintf(`
-- Locale-independent date construction via second offsets
set nowUnix to (do shell script "date '+%%s'") as integer
set startDate to (current date) + (%d - nowUnix)
set endDate   to (current date) + (%d - nowUnix)

tell application "Calendar"
	tell calendar "%s"
		set newEvent to make new event with properties {summary:"%s", start date:startDate, end date:endDate}
		%s
		%s
		%s
	end tell
	reload calendars
end tell
`, startUnix, endUnix, escapeAppleScript(calName), escapeAppleScript(e.Title),
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
				// AppleScript formats numbers with locale decimal separator.
				// German macOS uses comma: "1,782659481E+9" → replace with period.
				if f, err := strconv.ParseFloat(strings.ReplaceAll(val, ",", "."), 64); err == nil {
					e.StartTime = time.Unix(int64(f), 0).Local()
				}
			case "END":
				if f, err := strconv.ParseFloat(strings.ReplaceAll(val, ",", "."), 64); err == nil {
					e.EndTime = time.Unix(int64(f), 0).Local()
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
