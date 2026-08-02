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

var syncDays int

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync events from Apple Calendar into local cache",
	Example: `  calctl sync
  calctl sync --days 60`,
	RunE: func(cmd *cobra.Command, args []string) error {
		now := time.Now()
		from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		to := from.AddDate(0, 0, syncDays)

		if !isJSON() {
			fmt.Printf("Syncing %d days of Apple Calendar events...\n", syncDays)
		}

		events, err := calendar.FetchEvents(from, to)
		if err != nil {
			return fmt.Errorf("fetch from Apple Calendar: %w", err)
		}

		ctx := context.Background()
		s, err := store.New(config.DBPath(), config.Shared())
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer s.Close()

		if err := s.DeleteBySource(ctx, "apple", from, to); err != nil {
			return fmt.Errorf("clear old events: %w", err)
		}

		for i := range events {
			if err := s.UpsertEvent(ctx, &events[i]); err != nil {
				return fmt.Errorf("save event: %w", err)
			}
		}

		// Reconcile local echoes: `calctl add` inserts a row immediately
		// (source "calctl") AND creates the real event in Apple Calendar, so
		// once that same event comes back through this sync as a fresh
		// "apple"-sourced row, the echo is a stale duplicate — drop it.
		if echoes, err := s.ListBySource(ctx, "calctl", from, to); err == nil {
			for _, id := range staleEchoIDs(echoes, events) {
				_ = s.DeleteByID(ctx, id)
			}
		}

		if isJSON() {
			outputJSON(map[string]any{
				"ok":    true,
				"synced": len(events),
				"days":  syncDays,
				"from":  from.Format("2006-01-02"),
				"to":    to.Format("2006-01-02"),
			})
		} else {
			fmt.Printf("Synced %d events (%s → %s)\n",
				len(events), from.Format("Jan 2"), to.Format("Jan 2 2006"))
		}
		return nil
	},
}

func init() {
	syncCmd.Flags().IntVar(&syncDays, "days", 30, "Number of days to sync ahead")
	rootCmd.AddCommand(syncCmd)
}

// staleEchoIDs returns the IDs of local ("calctl"-sourced) echo rows whose
// title and start time (exact instant) match one of the freshly-synced
// Apple events — matched this way, not by ID, since AppleScript-created
// events don't hand back their EventKit identifier at creation time.
func staleEchoIDs(echoes, synced []models.Event) []string {
	var ids []string
	for _, echo := range echoes {
		for _, ev := range synced {
			if ev.Title == echo.Title && ev.StartTime.Equal(echo.StartTime) {
				ids = append(ids, echo.ID)
				break
			}
		}
	}
	return ids
}
