package cmd

import (
	"context"
	"fmt"
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
)

var addCmd = &cobra.Command{
	Use:   "add <title>",
	Short: "Quickly create a calendar event",
	Example: `  calctl add "Zahnarzt" --date 2026-07-05 --time 10:00 --duration 1h --cal Privat
  calctl add "Team Call" --date 2026-07-07 --time 14:00 --duration 30min --loc Zoom
  calctl add "Urlaub" --date 2026-08-01 --all-day`,
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

		e := &models.Event{
			ID:        "calctl-" + uuid.New().String(),
			Title:     title,
			StartTime: start,
			EndTime:   end,
			Calendar:  calName,
			Location:  addLoc,
			Notes:     addNotes,
			AllDay:    addAllDay,
			Source:    "calctl",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := calendar.CreateEvent(e); err != nil {
			return fmt.Errorf("create event: %w", err)
		}

		// save to local cache
		s, err := store.New(config.DBPath())
		if err == nil {
			defer s.Close()
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

	rootCmd.AddCommand(addCmd)
}
