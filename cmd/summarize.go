package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/aeon022/calctl/internal/calendar"
	"github.com/aeon022/calctl/internal/config"
	"github.com/aeon022/calctl/internal/models"
	"github.com/aeon022/calctl/internal/store"
	"github.com/aeon022/missionctl-core/dateutil"
	"github.com/spf13/cobra"
)

var summarizeCmd = &cobra.Command{
	Use:   "summarize",
	Short: "Generate a meeting summary using AI — missionctl Bundle feature",
	Example: `  calctl summarize --event-title "Sprint Planning"
  calctl summarize --date 2026-10-01 --event-title "Standup"
  calctl summarize --event-title "Design Review" --email`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !config.IsPro() {
			return fmt.Errorf("calctl summarize is a missionctl Bundle feature — get it at https://missionctl.sh/#pricing, then: calctl license activate <key>")
		}
		eventTitle, _ := cmd.Flags().GetString("event-title")
		dateStr, _ := cmd.Flags().GetString("date")
		emailFlag, _ := cmd.Flags().GetBool("email")

		// Resolve the date
		day, err := dateutil.ParseDateArg(dateStr)
		if err != nil {
			return fmt.Errorf("invalid --date %q: %w", dateStr, err)
		}
		day = dateutil.StartOfDay(day)
		from := day
		to := dateutil.EndOfDay(day)

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
		if len(events) == 0 {
			return fmt.Errorf("no events found for %s", day.Format("2006-01-02"))
		}

		event := findBestMatch(events, eventTitle)
		if event == nil {
			return fmt.Errorf("no matching event for %q on %s", eventTitle, day.Format("2006-01-02"))
		}

		summary, err := calendar.Summarize(ctx, event)
		if err != nil {
			return fmt.Errorf("summarize: %w", err)
		}

		if isJSON() {
			outputJSON(map[string]any{
				"tool":    "calctl",
				"command": "summarize",
				"event":   event.Title,
				"date":    event.StartTime.Format("2006-01-02"),
				"summary": summary,
			})
			return nil
		}

		fmt.Printf("Meeting Summary: %s\n", event.Title)
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println(summary)
		fmt.Println()

		if emailFlag {
			if len(event.Attendees) == 0 {
				fmt.Fprintln(os.Stderr, "warn: no attendees in event, skipping draft")
			} else {
				draftPath, err := writeDraftFile(event, summary)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warn: could not write draft file: %v\n", err)
				} else {
					fmt.Printf("Draft command:\n  mailctl draft %s\n", draftPath)
				}
			}
		}

		return nil
	},
}

// findBestMatch returns the event whose title best matches the query.
// If query is empty, returns the first non-all-day event (or first event).
func findBestMatch(events []models.Event, query string) *models.Event {
	if len(events) == 0 {
		return nil
	}
	if query == "" {
		for i := range events {
			if !events[i].AllDay {
				return &events[i]
			}
		}
		return &events[0]
	}
	q := strings.ToLower(query)
	// Exact substring match first
	for i := range events {
		if strings.Contains(strings.ToLower(events[i].Title), q) {
			return &events[i]
		}
	}
	// Word-level partial match
	words := strings.FieldsFunc(q, func(r rune) bool { return unicode.IsSpace(r) })
	best := -1
	bestScore := 0
	for i, e := range events {
		title := strings.ToLower(e.Title)
		score := 0
		for _, w := range words {
			if strings.Contains(title, w) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			best = i
		}
	}
	if best >= 0 {
		return &events[best]
	}
	return &events[0]
}

// writeDraftFile writes a mailctl-compatible Markdown draft file to /tmp and returns its path.
func writeDraftFile(event *models.Event, summary string) (string, error) {
	safeTitle := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return '-'
	}, event.Title)
	path := fmt.Sprintf("/tmp/calctl-summary-%s.md", safeTitle)

	var toLines []string
	for _, a := range event.Attendees {
		toLines = append(toLines, fmt.Sprintf("  - %s", a))
	}

	content := fmt.Sprintf("---\nto:\n%s\nsubject: \"Meeting Summary: %s\"\n---\n\n%s\n",
		strings.Join(toLines, "\n"), event.Title, summary)

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func init() {
	summarizeCmd.Flags().String("event-title", "", "Partial title match for the event")
	summarizeCmd.Flags().String("date", "", "Date to look up (YYYY-MM-DD, default: today)")
	summarizeCmd.Flags().Bool("email", false, "Write a mailctl draft file and print the draft command")
	rootCmd.AddCommand(summarizeCmd)
}
