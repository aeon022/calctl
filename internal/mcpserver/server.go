package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aeon022/calctl/internal/calendar"
	"github.com/aeon022/calctl/internal/config"
	"github.com/aeon022/calctl/internal/models"
	"github.com/aeon022/calctl/internal/store"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Serve starts the calctl MCP server on stdio.
func Serve() error {
	s := server.NewMCPServer(
		"calctl",
		"0.1.0",
		server.WithToolCapabilities(false),
	)

	s.AddTool(toolListEvents(), handleListEvents)
	s.AddTool(toolToday(), handleToday)
	s.AddTool(toolThisWeek(), handleThisWeek)
	s.AddTool(toolSync(), handleSync)
	s.AddTool(toolFreeSlots(), handleFreeSlots)
	s.AddTool(toolCreateEvent(), handleCreateEvent)
	s.AddTool(toolDeleteEvent(), handleDeleteEvent)

	return server.ServeStdio(s)
}

// ── Tool definitions ──────────────────────────────────────────────────────────

func toolListEvents() mcp.Tool {
	return mcp.NewTool("list_events",
		mcp.WithDescription("List calendar events between two dates. Returns events sorted by start time."),
		mcp.WithString("from",
			mcp.Required(),
			mcp.Description("Start date in YYYY-MM-DD format"),
		),
		mcp.WithString("to",
			mcp.Required(),
			mcp.Description("End date in YYYY-MM-DD format (inclusive)"),
		),
	)
}

func toolToday() mcp.Tool {
	return mcp.NewTool("today",
		mcp.WithDescription("List today's calendar events. Shortcut for list_events with today's date."),
	)
}

func toolThisWeek() mcp.Tool {
	return mcp.NewTool("this_week",
		mcp.WithDescription("List this week's calendar events (Monday to Sunday)."),
	)
}

func toolSync() mcp.Tool {
	return mcp.NewTool("sync",
		mcp.WithDescription("Sync events from Apple Calendar into the local cache. Call this before listing if data might be stale."),
		mcp.WithNumber("days",
			mcp.Description("Number of days to sync ahead (default: 14)"),
		),
	)
}

func toolFreeSlots() mcp.Tool {
	return mcp.NewTool("find_free_slots",
		mcp.WithDescription("Find free time slots within working hours for a given date range. Useful for scheduling meetings."),
		mcp.WithString("from",
			mcp.Required(),
			mcp.Description("Start date in YYYY-MM-DD format"),
		),
		mcp.WithString("to",
			mcp.Required(),
			mcp.Description("End date in YYYY-MM-DD format (inclusive)"),
		),
		mcp.WithNumber("min_minutes",
			mcp.Description("Minimum slot duration in minutes (default: 30)"),
		),
	)
}

func toolCreateEvent() mcp.Tool {
	return mcp.NewTool("create_event",
		mcp.WithDescription("Create a new event in Apple Calendar."),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("Event title"),
		),
		mcp.WithString("start",
			mcp.Required(),
			mcp.Description("Start time in ISO 8601 format: YYYY-MM-DDTHH:MM:SS"),
		),
		mcp.WithString("end",
			mcp.Required(),
			mcp.Description("End time in ISO 8601 format: YYYY-MM-DDTHH:MM:SS"),
		),
		mcp.WithString("calendar",
			mcp.Description("Calendar name (default: system default calendar)"),
		),
		mcp.WithString("location",
			mcp.Description("Event location"),
		),
		mcp.WithString("notes",
			mcp.Description("Event notes or description"),
		),
		mcp.WithBoolean("all_day",
			mcp.Description("Whether this is an all-day event"),
		),
	)
}

func toolDeleteEvent() mcp.Tool {
	return mcp.NewTool("delete_event",
		mcp.WithDescription("Delete a calendar event by title and date. Use list_events first to confirm the exact title and date."),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("Exact event title"),
		),
		mcp.WithString("date",
			mcp.Required(),
			mcp.Description("Event date in YYYY-MM-DD format"),
		),
		mcp.WithString("calendar",
			mcp.Description("Calendar name — speeds up lookup"),
		),
	)
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func handleListEvents(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	from, err := parseDate(req.GetString("from", ""))
	if err != nil {
		return mcp.NewToolResultError("invalid 'from' date: " + err.Error()), nil
	}
	to, err := parseDate(req.GetString("to", ""))
	if err != nil {
		return mcp.NewToolResultError("invalid 'to' date: " + err.Error()), nil
	}
	to = endOfDay(to)

	events, err := loadEvents(from, to)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(renderEvents(events, from, to)), nil
}

func handleToday(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	now := startOfDay(time.Now())
	events, err := loadEvents(now, endOfDay(now))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(renderEvents(events, now, endOfDay(now))), nil
}

func handleThisWeek(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	from, to := thisWeekRange()
	events, err := loadEvents(from, to)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(renderEvents(events, from, to)), nil
}

func handleSync(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	days := int(req.GetFloat("days", 14))
	if days <= 0 || days > 365 {
		days = 14
	}

	from := startOfDay(time.Now())
	to := from.AddDate(0, 0, days)

	events, err := calendar.FetchEvents(from, to)
	if err != nil {
		return mcp.NewToolResultError("sync failed: " + err.Error()), nil
	}

	s, err := store.New(config.DBPath())
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	defer s.Close()

	ctx := context.Background()
	_ = s.DeleteBySource(ctx, "apple", from, to)
	for i := range events {
		_ = s.UpsertEvent(ctx, &events[i])
	}

	return mcp.NewToolResultText(fmt.Sprintf("Synced %d events (%s → %s).",
		len(events),
		from.Format("Jan 02"),
		to.Format("Jan 02 2006"),
	)), nil
}

func handleFreeSlots(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	from, err := parseDate(req.GetString("from", ""))
	if err != nil {
		return mcp.NewToolResultError("invalid 'from' date: " + err.Error()), nil
	}
	to, err := parseDate(req.GetString("to", ""))
	if err != nil {
		return mcp.NewToolResultError("invalid 'to' date: " + err.Error()), nil
	}
	to = endOfDay(to)
	minMin := int(req.GetFloat("min_minutes", float64(config.Active.MinFreeSlot)))
	if minMin <= 0 {
		minMin = 30
	}

	events, err := loadEvents(from, to)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	cfg := config.Active
	slots := calendar.FindFreeSlots(events, from, to, calendar.WorkingHours{
		From: cfg.WorkingHoursFrom,
		To:   cfg.WorkingHoursTo,
	}, minMin)

	if len(slots) == 0 {
		return mcp.NewToolResultText("No free slots found in the given range."), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Free slots (%d+ min) from %s to %s:\n\n",
		minMin, from.Format("Mon Jan 02"), to.Format("Mon Jan 02"))

	var lastDate string
	for _, sl := range slots {
		if sl.Date != lastDate {
			fmt.Fprintf(&b, "%s\n", sl.Start.Format("Mon, Jan 02"))
			lastDate = sl.Date
		}
		fmt.Fprintf(&b, "  %s – %s  (%s)\n",
			sl.Start.Format("15:04"),
			sl.End.Format("15:04"),
			fmtDur(sl.Duration),
		)
	}
	return mcp.NewToolResultText(b.String()), nil
}

func handleCreateEvent(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title := req.GetString("title", "")
	if title == "" {
		return mcp.NewToolResultError("title is required"), nil
	}

	startStr := req.GetString("start", "")
	endStr := req.GetString("end", "")

	start, err := time.ParseInLocation("2006-01-02T15:04:05", startStr, time.Local)
	if err != nil {
		return mcp.NewToolResultError("invalid start time (use YYYY-MM-DDTHH:MM:SS): " + err.Error()), nil
	}
	end, err := time.ParseInLocation("2006-01-02T15:04:05", endStr, time.Local)
	if err != nil {
		return mcp.NewToolResultError("invalid end time (use YYYY-MM-DDTHH:MM:SS): " + err.Error()), nil
	}

	e := &models.Event{
		Title:     title,
		StartTime: start,
		EndTime:   end,
		Calendar:  req.GetString("calendar", config.Active.DefaultCalendar),
		Location:  req.GetString("location", ""),
		Notes:     req.GetString("notes", ""),
		AllDay:    req.GetBool("all_day", false),
		Source:    "calctl",
	}

	if err := calendar.CreateEvent(e); err != nil {
		return mcp.NewToolResultError("create failed: " + err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf(
		"Created: %s on %s %s–%s%s",
		title,
		start.Format("Mon, Jan 02 2006"),
		start.Format("15:04"),
		end.Format("15:04"),
		calendarSuffix(e.Calendar),
	)), nil
}

func handleDeleteEvent(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title := req.GetString("title", "")
	dateStr := req.GetString("date", "")
	calName := req.GetString("calendar", "")

	if title == "" || dateStr == "" {
		return mcp.NewToolResultError("title and date are required"), nil
	}

	date, err := parseDate(dateStr)
	if err != nil {
		return mcp.NewToolResultError("invalid date: " + err.Error()), nil
	}

	// find the event in cache to get its full data (start time, calendar)
	s, err := store.New(config.DBPath())
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	defer s.Close()

	from := startOfDay(date)
	to := endOfDay(date)
	events, err := s.ListEvents(context.Background(), from, to)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var target *models.Event
	for i := range events {
		if events[i].Title == title && (calName == "" || events[i].Calendar == calName) {
			target = &events[i]
			break
		}
	}
	if target == nil {
		return mcp.NewToolResultError(fmt.Sprintf("event %q on %s not found in cache — run sync first", title, dateStr)), nil
	}

	if err := calendar.DeleteEvent(target); err != nil {
		return mcp.NewToolResultError("delete failed: " + err.Error()), nil
	}
	_ = s.DeleteByID(context.Background(), target.ID)

	return mcp.NewToolResultText(fmt.Sprintf("Deleted: %s on %s", title, date.Format("Mon, Jan 02 2006"))), nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func loadEvents(from, to time.Time) ([]models.Event, error) {
	s, err := store.New(config.DBPath())
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	defer s.Close()
	return s.ListEvents(context.Background(), from, to)
}

func renderEvents(events []models.Event, from, to time.Time) string {
	if len(events) == 0 {
		return fmt.Sprintf("No events between %s and %s.", from.Format("Mon Jan 02"), to.Format("Mon Jan 02"))
	}

	// group by day
	type evJSON struct {
		Title    string `json:"title"`
		Start    string `json:"start"`
		End      string `json:"end"`
		Calendar string `json:"calendar"`
		Location string `json:"location,omitempty"`
		AllDay   bool   `json:"all_day,omitempty"`
		Notes    string `json:"notes,omitempty"`
	}
	type dayJSON struct {
		Date   string   `json:"date"`
		Events []evJSON `json:"events"`
	}

	var days []dayJSON
	var lastDate string
	var cur *dayJSON

	for _, e := range events {
		d := e.StartTime.Format("2006-01-02")
		if d != lastDate {
			if cur != nil {
				days = append(days, *cur)
			}
			cur = &dayJSON{Date: e.StartTime.Format("Mon, Jan 02 2006")}
			lastDate = d
		}
		timeStr := e.StartTime.Format("15:04") + "–" + e.EndTime.Format("15:04")
		if e.AllDay {
			timeStr = "all day"
		}
		cur.Events = append(cur.Events, evJSON{
			Title:    e.Title,
			Start:    timeStr,
			End:      e.EndTime.Format("15:04"),
			Calendar: e.Calendar,
			Location: e.Location,
			AllDay:   e.AllDay,
			Notes:    e.Notes,
		})
	}
	if cur != nil {
		days = append(days, *cur)
	}

	b, _ := json.MarshalIndent(days, "", "  ")
	return string(b)
}

func parseDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	return time.ParseInLocation("2006-01-02", s, time.Local)
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func endOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, t.Location())
}

func thisWeekRange() (time.Time, time.Time) {
	now := startOfDay(time.Now())
	wd := int(now.Weekday())
	if wd == 0 {
		wd = 7
	}
	monday := now.AddDate(0, 0, -(wd - 1))
	sunday := monday.AddDate(0, 0, 6)
	return monday, endOfDay(sunday)
}

func fmtDur(d time.Duration) string {
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

func calendarSuffix(cal string) string {
	if cal == "" {
		return ""
	}
	return " [" + cal + "]"
}
