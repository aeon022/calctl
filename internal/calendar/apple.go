package calendar

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aeon022/calctl/internal/models"
	"github.com/google/uuid"
)

// swiftScript is the embedded EventKit fetcher. It is written to disk on first
// sync so that swift can JIT-cache it between runs.
const swiftScript = `#!/usr/bin/swift
import EventKit
import Foundation

let args = CommandLine.arguments
guard args.count >= 3 else { fputs("usage: fetch_events.swift FROM TO\n", stderr); exit(1) }

let fmt2 = ISO8601DateFormatter()
fmt2.formatOptions = [.withInternetDateTime]

guard let from = fmt2.date(from: args[1]), let to = fmt2.date(from: args[2]) else {
    fputs("bad date args\n", stderr); exit(1)
}

let store = EKEventStore()
let sema = DispatchSemaphore(value: 0)

store.requestFullAccessToEvents { granted, _ in
    defer { sema.signal() }
    guard granted else { fputs("Calendar access denied\n", stderr); return }

    let pred = store.predicateForEvents(withStart: from, end: to, calendars: nil)
    let events = store.events(matching: pred)

    let local = ISO8601DateFormatter()
    local.formatOptions = [.withYear, .withMonth, .withDay, .withTime,
                           .withColonSeparatorInTime, .withDashSeparatorInDate]
    local.timeZone = TimeZone.current

    for evt in events {
        guard let cal = evt.calendar else { continue }
        if cal.type == .birthday || cal.type == .subscription { continue }
        let title = evt.title ?? ""
        let start = local.string(from: evt.startDate)
        let end   = local.string(from: evt.endDate)
        let loc   = evt.location ?? ""
        let allday = evt.isAllDay ? "1" : "0"
        let uid   = evt.eventIdentifier ?? ""
        let tz    = evt.timeZone?.identifier ?? ""
        print("TITLE:\(title)\nSTART:\(start)\nEND:\(end)\nCAL:\(cal.title)\nLOC:\(loc)\nALLDAY:\(allday)\nUID:\(uid)\nTZ:\(tz)\n---EVENT---")
    }
}
sema.wait()
`

// scriptPath returns the path where the Swift helper is stored.
func scriptPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "calctl", "fetch_events.swift")
}

// ensureScript writes the embedded Swift script to disk if it doesn't exist or is outdated.
func ensureScript() (string, error) {
	p := scriptPath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return "", err
	}
	existing, _ := os.ReadFile(p)
	if string(existing) != swiftScript {
		if err := os.WriteFile(p, []byte(swiftScript), 0644); err != nil {
			return "", err
		}
	}
	return p, nil
}

// FetchEvents fetches events from Apple Calendar via EventKit (Swift helper).
// Falls back to AppleScript if swift is unavailable.
func FetchEvents(from, to time.Time) ([]models.Event, error) {
	if _, err := exec.LookPath("swift"); err == nil {
		return fetchViaEventKit(from, to)
	}
	return fetchViaAppleScript(from, to)
}

func fetchViaEventKit(from, to time.Time) ([]models.Event, error) {
	script, err := ensureScript()
	if err != nil {
		return fetchViaAppleScript(from, to)
	}

	// ISO 8601 with timezone offset
	fromStr := from.Format("2006-01-02T15:04:05-07:00")
	toStr := to.Format("2006-01-02T15:04:05-07:00")

	cmd := exec.Command("swift", script, fromStr, toStr)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("swift: %s", string(exitErr.Stderr))
		}
		return nil, err
	}
	return parseEvents(strings.TrimSpace(string(out))), nil
}

// CreateEvent creates a new event in Apple Calendar via AppleScript.
//
// Requires e.Calendar to be set — it used to silently fall back to
// "whichever calendar Calendar.app happens to list first" (firstWritableCalendar,
// removed) when no calendar was given anywhere (no --cal, no configured
// default_calendar). That's genuinely dangerous on a account with shared
// calendars: it landed a test event in a family calendar, notifying every
// member, since "first writable" is an arbitrary property of account/list
// ordering, not a deliberate choice. Every caller (cmd/add.go, the TUI
// create form, the MCP server) already resolves --cal or
// config.Active.DefaultCalendar before reaching here, so this only fires
// when the user genuinely hasn't specified or configured one anywhere.
func CreateEvent(e *models.Event) error {
	if e.Calendar == "" {
		return fmt.Errorf("no calendar specified — pass --cal <name>, or set default_calendar in the calctl config; run `calctl calendars` to see available names")
	}
	script := buildCreateScript(e)
	_, err := runAppleScript(script)
	return err
}

// DeleteEvent removes an event from Apple Calendar by matching title + start
// time, searching every calendar across all accounts rather than requiring
// e.Calendar to exactly match an existing calendar name — a stored/derived
// Calendar value ("" falling back to a guessed default, or one that's since
// been renamed) previously meant the delete silently matched nothing and
// left the real event behind untouched while the caller believed it had
// succeeded. Returns an error if no matching event was found anywhere, so
// callers can tell a real deletion from a silent no-op.
//
// KNOWN LIMITATION, confirmed by direct testing while adding recurring-event
// support (models.Event.Recurrence): Calendar.app's AppleScript `delete`
// command does not reliably remove a *recurring* event — reproduced with a
// disposable test event (FREQ=DAILY;COUNT=2): `delete` returned success
// (deletedCount > 0, no error) but the event was still present on every
// re-query, including after quitting and relaunching Calendar.app, clearing
// the event's recurrence property first, deleting by exact uid instead of a
// whose-clause match, and a bulk `delete (every event whose ...)` — none of
// it took effect. This is a Calendar.app/EventKit AppleScript-bridge
// limitation, not something fixable from calctl's side. A recurring event
// created via `calctl add --repeat` may need to be deleted manually in
// Calendar.app if this function reports success but the event persists.
func DeleteEvent(e *models.Event) error {
	startISO := e.StartTime.Format("2006-01-02T15:04:05")
	escapedTitle := escapeAppleScript(e.Title)

	script := fmt.Sprintf(`
set nowUnix to (do shell script "date '+%%s'") as integer
set targetDate to (current date) + ((do shell script "date -jf '%%Y-%%m-%%dT%%H:%%M:%%S' '%s' '+%%s'") as integer - nowUnix)
set deletedCount to 0
tell application "Calendar"
	repeat with c in calendars
		set evts to (every event of c whose summary = "%s" and start date = targetDate)
		repeat with e in evts
			delete e
			set deletedCount to deletedCount + 1
		end repeat
	end repeat
	reload calendars
end tell
return deletedCount as string
`, startISO, escapedTitle)
	out, err := runAppleScript(script)
	if err != nil {
		return err
	}
	if out == "0" {
		return fmt.Errorf("no matching event found in any calendar (title %q, start %s)", e.Title, startISO)
	}
	return nil
}

// ListCalendars returns all calendar names from Apple Calendar, deduplicated.
// calendar "calendars" in Calendar.app already spans all accounts.
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
	seen := make(map[string]bool)
	var cals []string
	for _, name := range strings.Split(out, ", ") {
		name = strings.TrimSpace(name)
		if name != "" && !seen[name] {
			seen[name] = true
			cals = append(cals, name)
		}
	}
	return cals, nil
}

// fetchViaAppleScript is the fallback when swift is not available.
// It is slow on large calendar sets (birthday/holiday calendars especially).
func fetchViaAppleScript(from, to time.Time) ([]models.Event, error) {
	fromEpoch := from.Unix()
	toEpoch := to.Unix()

	script := fmt.Sprintf(`
set nowUnix to (do shell script "date '+%%s'") as integer
set fromDate to (current date) + (%d - nowUnix)
set toDate   to (current date) + (%d - nowUnix)

on zp(n)
	if n < 10 then return "0" & (n as text)
	return (n as text)
end zp

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
		if writable of cal then
		set calName to name of cal
		set evts to (every event of cal whose start date >= fromDate and start date <= toDate)
		repeat with evt in evts
			set t to summary of evt
			set s to my isoDate(start date of evt)
			set e to my isoDate(end date of evt)
			set evtLoc to ""
			try
				if location of evt is not missing value then set evtLoc to location of evt
			end try
			set evtAD to 0
			try
				if allday event of evt then set evtAD to 1
			end try
			set evtUID to ""
			try
				set evtUID to uid of evt
			end try
			set output to output & "TITLE:" & t & "\nSTART:" & s & "\nEND:" & e & "\nCAL:" & calName & "\nLOC:" & evtLoc & "\nALLDAY:" & evtAD & "\nUID:" & evtUID & "\n---EVENT---\n"
		end repeat
		end if
	end repeat
end tell
return output
`, fromEpoch, toEpoch)

	out, err := runAppleScript(script)
	if err != nil {
		return nil, fmt.Errorf("applescript: %w", err)
	}
	return parseEvents(out), nil
}

// buildCreateScript assumes e.Calendar is already set — CreateEvent checks
// that before calling this.
func buildCreateScript(e *models.Event) string {
	calName := e.Calendar

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
	recurrenceLine := ""
	if e.Recurrence != "" {
		recurrenceLine = fmt.Sprintf(`set recurrence of newEvent to "%s"`, escapeAppleScript(e.Recurrence))
	}

	startISO := e.StartTime.Format("2006-01-02T15:04:05")
	endISO := e.EndTime.Format("2006-01-02T15:04:05")
	escapedCal := escapeAppleScript(calName)
	escapedTitle := escapeAppleScript(e.Title)

	// Search all calendars (which spans all accounts in Calendar.app) by name.
	// "tell calendar NAME" only resolves iCloud calendars reliably on some macOS
	// versions; iterating calendars finds Exchange/Google/other account calendars too.
	return fmt.Sprintf(`
set startDate to (current date) + ((do shell script "date -jf '%%Y-%%m-%%dT%%H:%%M:%%S' '%s' '+%%s'") as integer - (do shell script "date '+%%s'") as integer)
set endDate   to (current date) + ((do shell script "date -jf '%%Y-%%m-%%dT%%H:%%M:%%S' '%s' '+%%s'") as integer - (do shell script "date '+%%s'") as integer)

tell application "Calendar"
	set foundCal to missing value
	repeat with c in calendars
		if name of c is "%s" then
			set foundCal to c
			exit repeat
		end if
	end repeat
	if foundCal is missing value then
		error "Calendar not found: %s"
	end if
	tell foundCal
		set newEvent to make new event with properties {summary:"%s", start date:startDate, end date:endDate}
		%s
		%s
		%s
		%s
	end tell
	reload calendars
end tell
`, startISO, endISO, escapedCal, escapedCal, escapedTitle,
		locationLine, notesLine, allDayLine, recurrenceLine)
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
				// EventKit outputs local time as "YYYY-MM-DDTHH:MM:SS" (no zone offset in local fmt)
				// AppleScript also outputs the same format.
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
			case "TZ":
				e.Timezone = val
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
