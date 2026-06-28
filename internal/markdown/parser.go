package markdown

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/aeon022/calctl/internal/models"
	"gopkg.in/yaml.v3"
)

var frontmatterRe = regexp.MustCompile(`(?s)^---\n(.+?)\n---`)

// ParseFile reads a Markdown file and returns an EventImport.
func ParseFile(path string) (*models.EventImport, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read file: %w", err)
	}
	return ParseBytes(data)
}

// ParseBytes parses raw Markdown content into an EventImport + body text.
func ParseBytes(data []byte) (*models.EventImport, string, error) {
	content := string(data)

	m := frontmatterRe.FindStringSubmatch(content)
	if m == nil {
		return nil, "", fmt.Errorf("no YAML frontmatter found (expected --- block at top)")
	}

	var imp models.EventImport
	if err := yaml.Unmarshal([]byte(m[1]), &imp); err != nil {
		return nil, "", fmt.Errorf("parse frontmatter: %w", err)
	}

	body := strings.TrimSpace(content[len(m[0]):])
	if imp.Notes == "" && body != "" {
		imp.Notes = body
	}

	return &imp, body, nil
}

// ToEvent converts a parsed EventImport to a models.Event, resolving time fields.
func ToEvent(imp *models.EventImport) (*models.Event, error) {
	if imp.Title == "" {
		return nil, fmt.Errorf("title is required")
	}
	if imp.Date == "" {
		return nil, fmt.Errorf("date is required")
	}

	loc := time.Local

	var startTime time.Time
	var err error

	if imp.AllDay {
		startTime, err = time.ParseInLocation("2006-01-02", imp.Date, loc)
		if err != nil {
			return nil, fmt.Errorf("parse date %q: %w", imp.Date, err)
		}
	} else {
		if imp.Time == "" {
			imp.Time = "09:00"
		}
		startTime, err = time.ParseInLocation("2006-01-02 15:04", imp.Date+" "+imp.Time, loc)
		if err != nil {
			return nil, fmt.Errorf("parse date+time %q %q: %w", imp.Date, imp.Time, err)
		}
	}

	dur, err := parseDuration(imp.Duration)
	if err != nil {
		return nil, fmt.Errorf("parse duration %q: %w", imp.Duration, err)
	}
	if dur == 0 {
		if imp.AllDay {
			dur = 24 * time.Hour
		} else {
			dur = 60 * time.Minute
		}
	}

	now := time.Now()
	e := &models.Event{
		Title:     imp.Title,
		StartTime: startTime,
		EndTime:   startTime.Add(dur),
		AllDay:    imp.AllDay,
		Calendar:  imp.Calendar,
		Location:  imp.Location,
		Notes:     imp.Notes,
		Attendees: imp.Attendees,
		Source:    "apple",
		CreatedAt: now,
		UpdatedAt: now,
	}

	return e, nil
}

// parseDuration understands "60min", "1h30m", "90", "1.5h"
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}

	// bare number → minutes
	if n, err := strconv.Atoi(s); err == nil {
		return time.Duration(n) * time.Minute, nil
	}

	// "90min" or "90m"
	s = strings.ReplaceAll(s, "min", "m")

	// standard Go duration: "1h30m", "90m"
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("unrecognized duration format (use e.g. 60, 60min, 1h30m)")
	}
	return d, nil
}
