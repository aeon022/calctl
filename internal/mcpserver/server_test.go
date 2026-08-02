package mcpserver

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aeon022/calctl/internal/config"
	"github.com/aeon022/calctl/internal/models"
	"github.com/aeon022/calctl/internal/store"
	"github.com/mark3labs/mcp-go/mcp"
)

// setupTestDB points config.DBPath() at a temporary database, seeds it with an
// event today, and sets config.Active directly (never touching the real
// ~/.config/calctl/config.yaml). Only handlers that are pure DB/config reads
// are exercised here — handleSync/handleCreateEvent/handleDeleteEvent all
// shell out to AppleScript against the real Calendar app and are deliberately
// not smoke-tested.
func setupTestDB(t *testing.T) time.Time {
	t.Helper()
	path := filepath.Join(t.TempDir(), "calctl.db")
	config.DBPathOverride = path
	t.Cleanup(func() { config.DBPathOverride = "" })

	config.Active.WorkingHoursFrom = "09:00"
	config.Active.WorkingHoursTo = "18:00"
	config.Active.MinFreeSlot = 30

	s, err := store.New(path, false)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, now.Location())
	ev := &models.Event{
		ID: "1", Title: "Standup", StartTime: start, EndTime: start.Add(30 * time.Minute),
		Calendar: "Work", Source: "apple", CreatedAt: start, UpdatedAt: start,
	}
	if err := s.UpsertEvent(context.Background(), ev); err != nil {
		t.Fatalf("UpsertEvent: %v", err)
	}
	return start
}

func callTool(t *testing.T, handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), args map[string]any) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
	res, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if res.IsError {
		t.Fatalf("handler returned an error result: %+v", res.Content)
	}
	return res
}

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	return tc.Text
}

func TestToolsAreRegisteredWithValidSchema(t *testing.T) {
	for _, tc := range []struct {
		name string
		tool mcp.Tool
	}{
		{"list_events", toolListEvents()},
		{"today", toolToday()},
		{"this_week", toolThisWeek()},
		{"sync", toolSync()},
		{"find_free_slots", toolFreeSlots()},
		{"create_event", toolCreateEvent()},
		{"delete_event", toolDeleteEvent()},
	} {
		if tc.tool.Name != tc.name {
			t.Errorf("expected tool name %q, got %q", tc.name, tc.tool.Name)
		}
		if tc.tool.Description == "" {
			t.Errorf("tool %q has no description", tc.name)
		}
	}
}

func TestHandleToday(t *testing.T) {
	setupTestDB(t)

	res := callTool(t, handleToday, nil)
	text := resultText(t, res)
	if !strings.Contains(text, "Standup") {
		t.Errorf("expected today's event in output, got:\n%s", text)
	}
}

func TestHandleListEvents(t *testing.T) {
	start := setupTestDB(t)

	res := callTool(t, handleListEvents, map[string]any{
		"from": start.Format("2006-01-02"),
		"to":   start.Format("2006-01-02"),
	})
	text := resultText(t, res)
	if !strings.Contains(text, "Standup") {
		t.Errorf("expected event in output, got:\n%s", text)
	}
}

func TestHandleListEventsInvalidDate(t *testing.T) {
	setupTestDB(t)

	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{
		"from": "not-a-date", "to": "2026-07-20",
	}}}
	res, err := handleListEvents(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected an error result for an invalid date")
	}
}

func TestHandleThisWeek(t *testing.T) {
	setupTestDB(t)

	res := callTool(t, handleThisWeek, nil)
	_ = resultText(t, res) // just assert it doesn't error
}

func TestHandleFreeSlots(t *testing.T) {
	start := setupTestDB(t)

	res := callTool(t, handleFreeSlots, map[string]any{
		"from": start.Format("2006-01-02"),
		"to":   start.Format("2006-01-02"),
	})
	text := resultText(t, res)
	if !strings.Contains(text, "Free slots") {
		t.Errorf("expected free-slot output, got:\n%s", text)
	}
}
