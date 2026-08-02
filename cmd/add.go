package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aeon022/calctl/internal/calendar"
	"github.com/aeon022/calctl/internal/config"
	"github.com/aeon022/calctl/internal/models"
	"github.com/aeon022/calctl/internal/store"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var (
	addDate     string
	addTime     string
	addDuration string
	addCal      string
	addLoc      string
	addNotes    string
	addAllDay   bool
	addRepeat   string
	addCount    int
	addUntil    string
)

var addCmd = &cobra.Command{
	Use:   "add <title>",
	Short: "Quickly create a calendar event",
	Long: `Quickly create a calendar event.

Note on --repeat: Calendar.app's own AppleScript interface does not
reliably support deleting recurring events afterward (confirmed by
direct testing — see the KNOWN LIMITATION comment on DeleteEvent). If
you need to remove a recurring event later, you may need to do it
manually in Calendar.app.`,
	Example: `  calctl add "Zahnarzt" --date 2026-07-05 --time 10:00 --duration 1h --cal Privat
  calctl add "Team Call" --date 2026-07-07 --time 14:00 --duration 30min --loc Zoom
  calctl add "Urlaub" --date 2026-08-01 --all-day
  calctl add "Standup" --date 2026-08-04 --time 09:00 --duration 15min --repeat weekly --count 10`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		title := args[0]

		// parse date
		loc := time.Local
		dateStr := addDate
		if dateStr == "" {
			dateStr = time.Now().Format("2006-01-02")
		}
		day, err := time.ParseInLocation("2006-01-02", dateStr, loc)
		if err != nil {
			return fmt.Errorf("invalid --date %q (use YYYY-MM-DD)", dateStr)
		}

		var start, end time.Time
		if addAllDay {
			start = day
			end = day.Add(24*time.Hour - time.Second)
		} else {
			// parse time
			timeStr := addTime
			if timeStr == "" {
				timeStr = "09:00"
			}
			start, err = time.ParseInLocation("2006-01-02 15:04", dateStr+" "+timeStr, loc)
			if err != nil {
				return fmt.Errorf("invalid --time %q (use HH:MM)", timeStr)
			}

			// parse duration
			dur, err := parseDuration(addDuration)
			if err != nil {
				return err
			}
			end = start.Add(dur)
		}

		calName := addCal
		if calName == "" {
			calName = config.Active.DefaultCalendar
		}

		recurrence, err := buildRecurrenceRule(addRepeat, addCount, addUntil, loc)
		if err != nil {
			return err
		}

		e := &models.Event{
			ID:         "calctl-" + uuid.New().String(),
			Title:      title,
			StartTime:  start,
			EndTime:    end,
			Calendar:   calName,
			Location:   addLoc,
			Notes:      addNotes,
			AllDay:     addAllDay,
			Recurrence: recurrence,
			Source:     "calctl",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		s, err := store.New(config.DBPath())
		if err == nil {
			defer s.Close()
			if !addAllDay {
				dayStart := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
				existing, cerr := s.ListEvents(context.Background(), dayStart, dayStart.Add(24*time.Hour))
				if cerr == nil {
					if conflicts := models.FindConflicts(*e, existing); len(conflicts) > 0 {
						names := make([]string, len(conflicts))
						for i, c := range conflicts {
							names[i] = fmt.Sprintf("%q (%s–%s)", c.Title, c.StartTime.Format("15:04"), c.EndTime.Format("15:04"))
						}
						fmt.Fprintf(os.Stderr, "⚠ conflicts with: %s\n", strings.Join(names, ", "))
					}
				}
			}
		}

		if err := calendar.CreateEvent(e); err != nil {
			return fmt.Errorf("create event: %w", err)
		}

		// save to local cache
		if s != nil {
			_ = s.UpsertEvent(context.Background(), e)
		}

		if isJSON() {
			outputJSON(map[string]any{
				"tool":    "calctl",
				"command": "add",
				"status":  "created",
				"event":   e,
			})
			return nil
		}

		timeRange := start.Format("15:04") + "–" + end.Format("15:04")
		if addAllDay {
			timeRange = "all day"
		}
		calSuffix := ""
		if calName != "" {
			calSuffix = "  [" + calName + "]"
		}
		fmt.Printf("Created: %s  %s %s%s\n",
			title,
			start.Format("Mon, Jan 02 2006"),
			timeRange,
			calSuffix,
		)
		return nil
	},
}

// buildRecurrenceRule translates the friendly --repeat/--count/--until flags
// into an iCalendar RRULE string (empty if --repeat wasn't given). --count
// and --until are mutually exclusive, matching RRULE's own COUNT/UNTIL.
func buildRecurrenceRule(repeat string, count int, until string, loc *time.Location) (string, error) {
	if repeat == "" {
		return "", nil
	}
	if count > 0 && until != "" {
		return "", fmt.Errorf("--count and --until are mutually exclusive")
	}
	var freq string
	switch strings.ToLower(repeat) {
	case "daily":
		freq = "DAILY"
	case "weekly":
		freq = "WEEKLY"
	case "monthly":
		freq = "MONTHLY"
	case "yearly":
		freq = "YEARLY"
	default:
		return "", fmt.Errorf("invalid --repeat %q (use daily, weekly, monthly, or yearly)", repeat)
	}
	rule := "FREQ=" + freq
	if count > 0 {
		rule += fmt.Sprintf(";COUNT=%d", count)
	} else if until != "" {
		untilDate, err := time.ParseInLocation("2006-01-02", until, loc)
		if err != nil {
			return "", fmt.Errorf("invalid --until %q (use YYYY-MM-DD)", until)
		}
		rule += ";UNTIL=" + untilDate.UTC().Format("20060102T150405Z")
	}
	return rule, nil
}

// parseDuration parses "1h", "30min", "90m", "1h30m", "60" (bare number = minutes).
func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 60 * time.Minute, nil
	}
	// bare number → minutes
	if n, err := strconv.Atoi(s); err == nil {
		return time.Duration(n) * time.Minute, nil
	}
	// "Xmin" → minutes
	s2 := strings.ToLower(s)
	if strings.HasSuffix(s2, "min") {
		n, err := strconv.Atoi(strings.TrimSuffix(s2, "min"))
		if err == nil {
			return time.Duration(n) * time.Minute, nil
		}
	}
	// "Xh" or "XhYm" — use Go's time.ParseDuration with m→m, h→h
	// Normalise: "1h30m" is already valid Go duration
	s2 = strings.ReplaceAll(s2, "min", "m")
	d, err := time.ParseDuration(s2)
	if err != nil {
		return 0, fmt.Errorf("invalid --duration %q (use 1h, 30min, 1h30m, 90)", s)
	}
	return d, nil
}

func init() {
	addCmd.Flags().StringVar(&addDate, "date", "", "Date (YYYY-MM-DD), default: today")
	addCmd.Flags().StringVar(&addTime, "time", "", "Start time (HH:MM 24h), default: 09:00")
	addCmd.Flags().StringVar(&addDuration, "duration", "", "Duration: 1h, 30min, 1h30m, 90 (default: 1h)")
	addCmd.Flags().StringVar(&addCal, "cal", "", "Calendar name (default: system default)")
	addCmd.Flags().StringVar(&addLoc, "loc", "", "Location")
	addCmd.Flags().StringVar(&addNotes, "notes", "", "Notes")
	addCmd.Flags().BoolVar(&addAllDay, "all-day", false, "All-day event")
	addCmd.Flags().StringVar(&addRepeat, "repeat", "", "Recurrence: daily, weekly, monthly, or yearly")
	addCmd.Flags().IntVar(&addCount, "count", 0, "Number of occurrences (with --repeat; mutually exclusive with --until)")
	addCmd.Flags().StringVar(&addUntil, "until", "", "Last occurrence date YYYY-MM-DD (with --repeat; mutually exclusive with --count)")

	rootCmd.AddCommand(addCmd)
}
