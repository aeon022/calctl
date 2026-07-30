package cmd

import (
	"testing"
	"time"

	"github.com/aeon022/calctl/internal/models"
)

func TestStaleEchoIDs_MatchesByTitleAndStartTime(t *testing.T) {
	start := time.Date(2026, 7, 26, 22, 0, 0, 0, time.Local)
	echoes := []models.Event{
		{ID: "calctl-abc", Title: "Zahnarzt", StartTime: start, Source: "calctl"},
		{ID: "calctl-def", Title: "Unmatched", StartTime: start, Source: "calctl"},
	}
	synced := []models.Event{
		{ID: "apple-xyz", Title: "Zahnarzt", StartTime: start, Source: "apple"},
	}

	ids := staleEchoIDs(echoes, synced)
	if len(ids) != 1 || ids[0] != "calctl-abc" {
		t.Fatalf("expected only the matching echo (calctl-abc) to be flagged stale, got %v", ids)
	}
}

func TestStaleEchoIDs_NoMatchWhenTimeDiffers(t *testing.T) {
	echoes := []models.Event{
		{ID: "calctl-abc", Title: "Zahnarzt", StartTime: time.Date(2026, 7, 26, 22, 0, 0, 0, time.Local)},
	}
	synced := []models.Event{
		{ID: "apple-xyz", Title: "Zahnarzt", StartTime: time.Date(2026, 7, 26, 23, 0, 0, 0, time.Local)},
	}

	if ids := staleEchoIDs(echoes, synced); len(ids) != 0 {
		t.Fatalf("expected no match for a different start time, got %v", ids)
	}
}
