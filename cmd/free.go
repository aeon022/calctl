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
	freeDays int
	freeMin  int
)

var freeCmd = &cobra.Command{
	Use:   "free",
	Short: "Find free time slots in your calendar",
	Example: `  calctl free
  calctl free --next 7 --format json
  calctl free --next 14 --min 60`,
	RunE: func(cmd *cobra.Command, args []string) error {
		now := time.Now()
		from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		to := from.AddDate(0, 0, freeDays)

		ctx := context.Background()
		s, err := store.New(config.DBPath())
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer s.Close()

		events, err := s.ListEvents(ctx, from, to)
		if err != nil {
			return fmt.Errorf("list events: %w", err)
		}

		cfg := config.Active
		minMin := freeMin
		if minMin == 0 {
			minMin = cfg.MinFreeSlot
		}

		slots := calendar.FindFreeSlots(events, from, to, calendar.WorkingHours{
			From: cfg.WorkingHoursFrom,
			To:   cfg.WorkingHoursTo,
		}, minMin)

		if isJSON() {
			outputJSON(freeResponse{
				Tool:        "calctl",
				Command:     "free",
				From:        from.Format("2006-01-02"),
				To:          to.Format("2006-01-02"),
				MinMinutes:  minMin,
				WorkingFrom: cfg.WorkingHoursFrom,
				WorkingTo:   cfg.WorkingHoursTo,
				Count:       len(slots),
				Data:        formatSlots(slots),
			})
			return nil
		}

		if len(slots) == 0 {
			fmt.Println("No free slots found in the given range.")
			return nil
		}

		fmt.Printf("Free slots (min %dmin, %s–%s):\n\n", minMin, cfg.WorkingHoursFrom, cfg.WorkingHoursTo)
		var lastDate string
		for _, sl := range slots {
			if sl.Date != lastDate {
				fmt.Printf("%s\n", formatDateLabel(sl.Start))
				lastDate = sl.Date
			}
			h := int(sl.Duration.Minutes()) / 60
			m := int(sl.Duration.Minutes()) % 60
			durStr := ""
			if h > 0 {
				durStr = fmt.Sprintf("%dh%02dm", h, m)
			} else {
				durStr = fmt.Sprintf("%dm", m)
			}
			fmt.Printf("  %s – %s  (%s)\n",
				sl.Start.Format("15:04"),
				sl.End.Format("15:04"),
				durStr,
			)
		}
		fmt.Println()
		return nil
	},
}

type freeResponse struct {
	Tool        string         `json:"tool"`
	Command     string         `json:"command"`
	From        string         `json:"from"`
	To          string         `json:"to"`
	MinMinutes  int            `json:"min_minutes"`
	WorkingFrom string         `json:"working_from"`
	WorkingTo   string         `json:"working_to"`
	Count       int            `json:"count"`
	Data        []slotJSON     `json:"data"`
}

type slotJSON struct {
	Date            string `json:"date"`
	Start           string `json:"start"`
	End             string `json:"end"`
	DurationMinutes int    `json:"duration_minutes"`
}

func formatSlots(slots []models.FreeSlot) []slotJSON {
	out := make([]slotJSON, len(slots))
	for i, sl := range slots {
		out[i] = slotJSON{
			Date:            sl.Date,
			Start:           sl.Start.Format("2006-01-02T15:04:05"),
			End:             sl.End.Format("2006-01-02T15:04:05"),
			DurationMinutes: int(sl.Duration.Minutes()),
		}
	}
	return out
}

func formatDateLabel(t time.Time) string {
	return t.Format("Mon, Jan 02")
}

func init() {
	freeCmd.Flags().IntVar(&freeDays, "next", 7, "Number of days to look ahead")
	freeCmd.Flags().IntVar(&freeMin, "min", 0, "Minimum slot duration in minutes (default from config)")
	rootCmd.AddCommand(freeCmd)
}
