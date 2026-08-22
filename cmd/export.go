package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/aeon022/calctl/internal/config"
	"github.com/aeon022/calctl/internal/store"
	"github.com/spf13/cobra"
)

var (
	exportWeek   bool
	exportFrom   string
	exportTo     string
	exportOutput string
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export calendar events to a file (or stdout)",
	Example: `  calctl export --week --format json
  calctl export --week --format json --output week.json
  calctl export --from 2026-10-01 --to 2026-10-31 -o october.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		from, to, err := resolveRange(false, exportWeek, exportFrom, exportTo)
		if err != nil {
			return err
		}

		ctx := context.Background()
		s, err := store.New(config.DBPath(), config.Shared())
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer s.Close()

		events, err := s.ListEvents(ctx, from, to)
		if err != nil {
			return fmt.Errorf("list events: %w", err)
		}

		w := os.Stdout
		if exportOutput != "" {
			f, err := os.Create(exportOutput)
			if err != nil {
				return fmt.Errorf("create output file: %w", err)
			}
			defer f.Close()
			w = f
		}

		if isJSON() {
			outputJSONTo(w, listResponse{
				Tool:    "calctl",
				Command: "export",
				From:    from.Format("2006-01-02"),
				To:      to.Format("2006-01-02"),
				Count:   len(events),
				Data:    events,
			})
		} else {
			printEventsTo(w, events, from, to)
		}

		if exportOutput != "" {
			fmt.Fprintf(os.Stderr, "Exported %d event(s) -> %s\n", len(events), exportOutput)
		}
		return nil
	},
}

func init() {
	exportCmd.Flags().BoolVar(&exportWeek, "week", false, "Export this week's events")
	exportCmd.Flags().StringVar(&exportFrom, "from", "", "Start date (YYYY-MM-DD)")
	exportCmd.Flags().StringVar(&exportTo, "to", "", "End date (YYYY-MM-DD)")
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "Output file (default: stdout)")

	rootCmd.AddCommand(exportCmd)
}
