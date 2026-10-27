package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/aeon022/calctl/internal/calendar"
	"github.com/aeon022/calctl/internal/config"
	"github.com/aeon022/calctl/internal/store"
	"github.com/aeon022/missionctl-core/activity"
	"github.com/aeon022/missionctl-core/dateutil"
	"github.com/spf13/cobra"
)

var (
	updOn, updIn                                         string
	updTitle, updStart, updEnd, updLoc, updNotes, updCal string
)

var updateCmd = &cobra.Command{
	Use:   "update <title>",
	Short: "Change an existing event (time, title, location, notes, calendar)",
	Long: `Change an existing event. The event is found by its exact title and the day
given with --on (from the last sync; run 'calctl sync' first). Only the flags
you pass are changed; moving --start keeps the duration unless --end is given.
Prints the old and the new values, and warns about overlaps.

Moving to another calendar (--calendar) creates a copy there and deletes the
original, so recurring events cannot be moved that way.`,
	Example: `  calctl update "Zahnarzt" --on 2026-07-05 --start 2026-07-06T10:30
  calctl update "Team Call" --on 2026-07-07 --title "Team Call (Zoom)" --location Zoom
  calctl update "Standup" --on 2026-08-04 --calendar Arbeit`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		day, err := dateutil.ParseDateArg(updOn)
		if err != nil || updOn == "" {
			return fmt.Errorf("--on YYYY-MM-DD is required (the day the event is on now)")
		}
		var p calendar.Patch
		f := cmd.Flags()
		for name, dst := range map[string]**string{"title": &p.Title, "location": &p.Location, "notes": &p.Notes, "calendar": &p.Calendar} {
			if f.Changed(name) {
				v, _ := f.GetString(name)
				*dst = &v
			}
		}
		if f.Changed("start") {
			t, err := calendar.ParseWhen(updStart)
			if err != nil {
				return err
			}
			p.Start = &t
		}
		if f.Changed("end") {
			t, err := calendar.ParseWhen(updEnd)
			if err != nil {
				return err
			}
			p.End = &t
		}

		s, err := store.New(config.DBPath(), config.Shared())
		if err != nil {
			return err
		}
		defer s.Close()
		r, err := calendar.Update(s, args[0], day, updIn, p)
		if err != nil {
			return err
		}
		activity.Log("calctl", "updated", r.New.Title)

		if len(r.Conflicts) > 0 {
			names := make([]string, len(r.Conflicts))
			for i, c := range r.Conflicts {
				names[i] = fmt.Sprintf("%q (%s–%s)", c.Title, c.StartTime.Format("15:04"), c.EndTime.Format("15:04"))
			}
			fmt.Fprintf(os.Stderr, "⚠ conflicts with: %s\n", strings.Join(names, ", "))
		}
		if isJSON() {
			outputJSON(map[string]any{"tool": "calctl", "command": "update", "status": "updated", "old": r.Old, "new": r.New, "conflicts": r.Conflicts})
			return nil
		}
		fmt.Printf("Before: %s\nAfter:  %s\n", r.Old.Describe(), r.New.Describe())
		return nil
	},
}

func init() {
	fl := updateCmd.Flags()
	fl.StringVar(&updOn, "on", "", "Day the event is on now (YYYY-MM-DD, required)")
	fl.StringVar(&updIn, "in", "", "Only look in this calendar (if the title exists twice)")
	fl.StringVar(&updTitle, "title", "", "New title")
	fl.StringVar(&updStart, "start", "", "New start, YYYY-MM-DDTHH:MM (keeps the duration)")
	fl.StringVar(&updEnd, "end", "", "New end, YYYY-MM-DDTHH:MM")
	fl.StringVar(&updLoc, "location", "", "New location (empty clears it)")
	fl.StringVar(&updNotes, "notes", "", "New notes (empty clears them)")
	fl.StringVar(&updCal, "calendar", "", "Move to this calendar")
	rootCmd.AddCommand(updateCmd)
}
