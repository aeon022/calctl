package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aeon022/calctl/internal/calendar"
	"github.com/aeon022/calctl/internal/config"
	"github.com/aeon022/calctl/internal/markdown"
	"github.com/aeon022/calctl/internal/models"
	"github.com/aeon022/calctl/internal/store"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var importDryRun bool

var importCmd = &cobra.Command{
	Use:   "import [file.md or directory]",
	Short: "Create calendar events from Markdown files",
	Long: `Import one or more Markdown files as calendar events.
Each file must have a YAML frontmatter block with at least 'title' and 'date'.

Example frontmatter:
  ---
  title: Product Launch Call
  date: 2026-10-15
  time: 14:00
  duration: 60min
  calendar: Work
  location: Zoom
  attendees: [jan@example.com]
  ---`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var targetPath string

		if len(args) == 0 {
			fmt.Print("Path to Markdown file or directory: ")
			fmt.Scanln(&targetPath)
			targetPath = strings.Trim(strings.TrimSpace(targetPath), `"'`)
		} else {
			targetPath = args[0]
		}

		if targetPath == "" {
			return fmt.Errorf("no path provided")
		}

		files, err := collectMarkdownFiles(targetPath)
		if err != nil {
			return err
		}
		if len(files) == 0 {
			return fmt.Errorf("no Markdown files found at %s", targetPath)
		}

		ctx := context.Background()
		var s *store.Store
		if !importDryRun {
			s, err = store.New(config.DBPath())
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			defer s.Close()
		}

		var imported, failed int
		for _, f := range files {
			imp, _, err := markdown.ParseFile(f)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  skip %s: %v\n", filepath.Base(f), err)
				failed++
				continue
			}

			event, err := markdown.ToEvent(imp)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  skip %s: %v\n", filepath.Base(f), err)
				failed++
				continue
			}

			event.ID = "import-" + uuid.New().String()

			if importDryRun {
				fmt.Printf("  [dry-run] %s  %s %s  [%s]\n",
					event.StartTime.Format("2006-01-02 15:04"),
					event.Title,
					durationStr(event),
					event.Calendar,
				)
				imported++
				continue
			}

			// Write to Apple Calendar
			if err := calendar.CreateEvent(event); err != nil {
				fmt.Fprintf(os.Stderr, "  error creating %q in Apple Calendar: %v\n", event.Title, err)
				failed++
				continue
			}

			// Cache in SQLite
			if err := s.UpsertEvent(ctx, event); err != nil {
				fmt.Fprintf(os.Stderr, "  warning: saved to Calendar but cache failed: %v\n", err)
			}

			if !isJSON() {
				fmt.Printf("  ✓ %s  %s\n", event.StartTime.Format("Mon Jan 2, 15:04"), event.Title)
			}
			imported++
		}

		if isJSON() {
			outputJSON(map[string]any{
				"ok":       failed == 0,
				"dry_run":  importDryRun,
				"imported": imported,
				"failed":   failed,
				"files":    len(files),
			})
		} else {
			if importDryRun {
				fmt.Printf("\nDry run: %d events validated, %d errors\n", imported, failed)
			} else {
				fmt.Printf("\nImported %d events", imported)
				if failed > 0 {
					fmt.Printf(", %d failed", failed)
				}
				fmt.Println()
			}
		}
		return nil
	},
}

func collectMarkdownFiles(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("path not found: %w", err)
	}
	if !info.IsDir() {
		return []string{path}, nil
	}
	var files []string
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() && (strings.HasSuffix(e.Name(), ".md") || strings.HasSuffix(e.Name(), ".markdown")) {
			files = append(files, filepath.Join(path, e.Name()))
		}
	}
	return files, nil
}

func durationStr(e *models.Event) string {
	d := e.EndTime.Sub(e.StartTime)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 && m > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	if h > 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dm", m)
}

func init() {
	importCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "Validate without creating events")
	rootCmd.AddCommand(importCmd)
}
