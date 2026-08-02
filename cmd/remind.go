package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/aeon022/calctl/internal/config"
	"github.com/aeon022/calctl/internal/store"
	"github.com/spf13/cobra"
)

var remindWithinMin int

var remindCmd = &cobra.Command{
	Use:   "remind",
	Short: "Send a macOS notification for events starting soon",
	Long: `Check for events starting within the next N minutes (15 by
default) and send a macOS notification. Same pattern habctl's own
"remind" uses — ideal as a launchd job running every few minutes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.New(config.DBPath(), config.Shared())
		if err != nil {
			return err
		}
		defer s.Close()

		now := time.Now()
		window := now.Add(time.Duration(remindWithinMin) * time.Minute)
		events, err := s.ListEvents(context.Background(), now, window)
		if err != nil {
			return err
		}

		var titles []string
		for _, e := range events {
			if e.StartTime.Before(now) {
				continue // already started before "now" (e.g. an all-day event) — not "starting soon"
			}
			titles = append(titles, fmt.Sprintf("%s (%s)", e.Title, e.StartTime.Format("15:04")))
		}

		if len(titles) == 0 {
			fmt.Printf("No events starting within %dm — nothing to remind.\n", remindWithinMin)
			return nil
		}

		title := fmt.Sprintf("%d event(s) starting soon", len(titles))
		body := strings.Join(titles, ", ")

		script := fmt.Sprintf(`display notification %q with title %q`, body, title)
		out, err := exec.Command("osascript", "-e", script).CombinedOutput()
		if err != nil {
			fmt.Printf("Reminder: %s — %s\n", title, body)
			if len(out) > 0 {
				fmt.Printf("osascript: %s\n", strings.TrimSpace(string(out)))
			}
			return nil
		}

		fmt.Printf("Notified: %s\n", body)
		return nil
	},
}

func init() {
	remindCmd.Flags().IntVar(&remindWithinMin, "within", 15, "Notify for events starting within this many minutes")
	rootCmd.AddCommand(remindCmd)
}
