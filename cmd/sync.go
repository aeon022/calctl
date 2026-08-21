package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/aeon022/calctl/internal/calendar"
	"github.com/aeon022/calctl/internal/config"
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

		ctx := context.Background()
		s, err := store.New(config.DBPath(), config.Shared())
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer s.Close()

		events, err := calendar.Sync(ctx, s, from, to)
		if err != nil {
			return err
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

