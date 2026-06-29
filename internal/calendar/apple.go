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
        print("TITLE:\(title)\nSTART:\(start)\nEND:\(end)\nCAL:\(cal.title)\nLOC:\(loc)\nALLDAY:\(allday)\nUID:\(uid)\n---EVENT---")
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
func CreateEvent(e *models.Event) error {
	script := buildCreateScript(e)
	_, err := runAppleScript(script)
	return err
}

// DeleteEvent removes an event from Apple Calendar by matching title + start time within its calendar.
func DeleteEvent(e *models.Event) error {
	calName := e.Calendar
	if calName == "" {
		calName = "Calendar"
	}
	startISO := e.StartTime.Format("2006-01-02T15:04:05")
	script := fmt.Sprintf(`
set nowUnix to (do shell script "date '+%%s'") as integer
set targetDate to (current date) + ((do shell script "date -jf '%%Y-%%m-%%dT%%H:%%M:%%S' '%s' '+%%s'") as integer - nowUnix)
tell application "Calendar"
	set theCal to first calendar whose name is "%s"
	set evts to (every event of theCal whose summary = "%s" and start date = targetDate)
	if (count of evts) > 0 then
		delete first item of evts
	end if
	reload calendars
end tell
`, startISO, escapeAppleScript(calName), escapeAppleScript(e.Title))
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

func buildCreateScript(e *models.Event) string {
	calName := e.Calendar
	if calName == "" {
		// use first writable calendar rather than hardcoding "Calendar"
		calName = firstWritableCalendar()
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

	startISO := e.StartTime.Format("2006-01-02T15:04:05")
	endISO := e.EndTime.Format("2006-01-02T15:04:05")

	return fmt.Sprintf(`
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

func firstWritableCalendar() string {
	script := `
tell application "Calendar"
	repeat with c in calendars
		if writable of c then return name of c
	end repeat
end tell`
	out, err := runAppleScript(script)
	if err != nil || strings.TrimSpace(out) == "" {
		return "Calendar"
	}
	return strings.TrimSpace(out)
}

func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
