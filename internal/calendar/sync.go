package calendar

import (
	"context"
	"fmt"
	"time"

	"github.com/aeon022/calctl/internal/models"
	"github.com/aeon022/calctl/internal/store"
)

// Sync fetches events from Apple Calendar for [from, to), replaces the
// previously synced Apple-sourced events in that range with the fresh set,
// and reconciles echoes (locally-created events that now match a synced
// Apple event). It's the same fetch → clear → upsert → reconcile sequence
// every caller (cmd sync/list, the MCP server, the TUI) needs before it can
// read events back out of the store. Returns the freshly fetched events.
func Sync(ctx context.Context, s *store.Store, from, to time.Time) ([]models.Event, error) {
	events, err := FetchEvents(from, to)
	if err != nil {
		return nil, fmt.Errorf("fetch from Apple Calendar: %w", err)
	}

	if err := s.DeleteBySource(ctx, "apple", from, to); err != nil {
		return nil, fmt.Errorf("clear old events: %w", err)
	}

	for i := range events {
		if err := s.UpsertEvent(ctx, &events[i]); err != nil {
			return nil, fmt.Errorf("save event: %w", err)
		}
	}

	if err := s.ReconcileEchoes(ctx, events, from, to); err != nil {
		return nil, fmt.Errorf("reconcile echoes: %w", err)
	}

	return events, nil
}
