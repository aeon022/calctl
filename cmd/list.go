package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/aeon022/calctl/internal/calendar"
	"github.com/aeon022/calctl/internal/config"
	"github.com/aeon022/calctl/internal/models"
	"github.com/aeon022/calctl/internal/store"
	"github.com/spf13/cobra"
)

var (
	listToday  bool
	listWeek   bool
	listFrom   string
	listTo     string
	listSync   bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List calendar events",
	Example: `  calctl list --today
  calctl list --week --format json
  calctl list --from 2026-10-01 --to 2026-10-31`,
	RunE: func(cmd *cobra.Command, args []string) error {
		from, to, err := resolveRange(listToday, listWeek, listFrom, listTo)
		if err != nil {
			return err
		}

		ctx := context.Background()
		s, err := store.New(config.DBPath())
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer s.Close()

		// Sync from Apple Calendar if requested or cache is empty
		if listSync {
			events, err := calendar.FetchEvents(from, to)
			if err != nil {
				return fmt.Errorf("fetch from Apple Calendar: %w", err)
			}
			if err := s.DeleteBySource(ctx, "apple", from, to); err != nil {
				return err
			}
			for i := range events {
				if err := s.UpsertEvent(ctx, &events[i]); err != nil {
					return err
				}
			}
		}

		events, err := s.ListEvents(ctx, from, to)
		if err != nil {
			return fmt.Errorf("list events: %w", err)
		}

		if isJSON() {
			outputJSON(listResponse{
				Tool:    "calctl",
				Command: "list",
				From:    from.Format("2006-01-02"),
				To:      to.Format("2006-01-02"),
				Count:   len(events),
				Data:    events,
			})
			return nil
		}

		printEvents(events, from, to)
		return nil
	},
}

type listResponse struct {
	Tool    string         `json:"tool"`
	Command string         `json:"command"`
	From    string         `json:"from"`
	To      string         `json:"to"`
	Count   int            `json:"count"`
	Data    []models.Event `json:"data"`
}

func printEvents(events []models.Event, from, to time.Time) {
	if len(events) == 0 {
		fmt.Printf("No events between %s and %s.\n", from.Format("Mon Jan 2"), to.Format("Mon Jan 2"))
		return
	}

	var lastDate string
	for _, e := range events {
		date := e.StartTime.Format("Mon, Jan 02")
		if date != lastDate {
			fmt.Printf("\n%s\n", date)
			fmt.Println(repeatChar("─", 40))
			lastDate = date
		}
		timeStr := e.StartTime.Format("15:04") + "–" + e.EndTime.Format("15:04")
		if e.AllDay {
			timeStr = "all day"
		}
		fmt.Printf("  %s  %s", timeStr, e.Title)
		if e.Calendar != "" {
			fmt.Printf("  [%s]", e.Calendar)
		}
		fmt.Println()
		if e.Location != "" && e.Location != "missing value" {
			fmt.Printf("    @ %s\n", e.Location)
		}
	}
	fmt.Println()
}

func resolveRange(today, week bool, fromStr, toStr string) (time.Time, time.Time, error) {
	now := time.Now()
	loc := now.Location()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	switch {
	case today:
		return dayStart, dayStart.Add(24*time.Hour - time.Second), nil
	case week:
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		monday := dayStart.AddDate(0, 0, -(weekday - 1))
		sunday := monday.AddDate(0, 0, 6).Add(24*time.Hour - time.Second)
		return monday, sunday, nil
	case fromStr != "" && toStr != "":
		from, err := time.ParseInLocation("2006-01-02", fromStr, loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse --from: %w", err)
		}
		to, err := time.ParseInLocation("2006-01-02", toStr, loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse --to: %w", err)
		}
		return from, to.Add(24*time.Hour - time.Second), nil
	default:
		// default: today
		return dayStart, dayStart.Add(24*time.Hour - time.Second), nil
	}
}

func repeatChar(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}

func init() {
	listCmd.Flags().BoolVar(&listToday, "today", false, "Show today's events")
	listCmd.Flags().BoolVar(&listWeek, "week", false, "Show this week's events")
	listCmd.Flags().StringVar(&listFrom, "from", "", "Start date (YYYY-MM-DD)")
	listCmd.Flags().StringVar(&listTo, "to", "", "End date (YYYY-MM-DD)")
	listCmd.Flags().BoolVar(&listSync, "sync", false, "Sync from Apple Calendar before listing")

	rootCmd.AddCommand(listCmd)
}
