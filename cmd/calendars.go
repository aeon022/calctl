package cmd

import (
	"fmt"

	"github.com/aeon022/calctl/internal/calendar"
	"github.com/spf13/cobra"
)

var calendarsCmd = &cobra.Command{
	Use:   "calendars",
	Short: "List all available Apple Calendar names",
	RunE: func(cmd *cobra.Command, args []string) error {
		cals, err := calendar.ListCalendars()
		if err != nil {
			return err
		}
		if isJSON() {
			outputJSON(map[string]any{"tool": "calctl", "command": "calendars", "data": cals})
			return nil
		}
		for _, c := range cals {
			fmt.Println(c)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(calendarsCmd)
}
