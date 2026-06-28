package calendar

import (
	"fmt"
	"sort"
	"time"

	"github.com/aeon022/calctl/internal/models"
)

// WorkingHours defines the daily window for free-slot search.
type WorkingHours struct {
	From string // "09:00"
	To   string // "18:00"
}

// FindFreeSlots returns gaps between events within working hours over [from, to].
func FindFreeSlots(events []models.Event, from, to time.Time, wh WorkingHours, minMinutes int) []models.FreeSlot {
	// Sort events by start time
	sort.Slice(events, func(i, j int) bool {
		return events[i].StartTime.Before(events[j].StartTime)
	})

	var slots []models.FreeSlot
	current := from

	for current.Before(to) {
		dayStart := dayBoundary(current, wh.From)
		dayEnd := dayBoundary(current, wh.To)

		if dayStart.After(to) {
			break
		}
		if dayEnd.After(to) {
			dayEnd = to
		}

		// collect events that overlap with this working day
		var dayEvents []models.Event
		for _, e := range events {
			if e.AllDay {
				continue
			}
			if e.EndTime.After(dayStart) && e.StartTime.Before(dayEnd) {
				dayEvents = append(dayEvents, e)
			}
		}

		// walk through the day finding gaps
		cursor := dayStart
		for _, e := range dayEvents {
			evStart := e.StartTime
			if evStart.Before(cursor) {
				evStart = cursor
			}
			if evStart.After(cursor) {
				dur := evStart.Sub(cursor)
				if int(dur.Minutes()) >= minMinutes {
					slots = append(slots, models.FreeSlot{
						Start:    cursor,
						End:      evStart,
						Duration: dur,
						Date:     current.Format("2006-01-02"),
					})
				}
			}
			if e.EndTime.After(cursor) {
				cursor = e.EndTime
			}
		}

		// gap between last event and end of working day
		if cursor.Before(dayEnd) {
			dur := dayEnd.Sub(cursor)
			if int(dur.Minutes()) >= minMinutes {
				slots = append(slots, models.FreeSlot{
					Start:    cursor,
					End:      dayEnd,
					Duration: dur,
					Date:     current.Format("2006-01-02"),
				})
			}
		}

		current = current.AddDate(0, 0, 1)
		current = time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, current.Location())
	}

	return slots
}

// dayBoundary returns the time.Time for a given day at "HH:MM".
func dayBoundary(day time.Time, hhmm string) time.Time {
	var h, m int
	fmt.Sscanf(hhmm, "%d:%d", &h, &m)
	return time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, day.Location())
}
