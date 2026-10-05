package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aeon022/calctl/internal/config"
	"github.com/aeon022/calctl/internal/models"
	"github.com/aeon022/calctl/internal/store"
	"github.com/aeon022/missionctl-core/dateutil"
)

// setup isolates HOME + data dir and seeds a temp DB with known events.
// PersistentPreRunE calls config.Load on every command, so the (empty) temp
// HOME gives it the defaults: working hours 09:00–18:00, 30 min slots.
func setup(t *testing.T, events ...models.Event) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("CALCTL_DATA_DIR", "")
	config.DBPathOverride = filepath.Join(t.TempDir(), "c.db")
	t.Cleanup(func() {
		config.DBPathOverride = ""
		formatFlag = "human"
		listToday, listWeek, listFrom, listTo, listSync = false, false, "", "", false
		exportWeek, exportFrom, exportTo, exportOutput = false, "", "", ""
		freeDays, freeMin = 7, 0
	})

	s, err := store.New(config.DBPath(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := range events {
		e := events[i]
		e.Source, e.CreatedAt, e.UpdatedAt = "apple", time.Now(), time.Now()
		if err := s.UpsertEvent(context.Background(), &e); err != nil {
			t.Fatal(err)
		}
	}
}

func today(h, m int) time.Time {
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), h, m, 0, 0, time.Local)
}

func ev(id, title string, start time.Time, dur time.Duration) models.Event {
	return models.Event{ID: id, Title: title, StartTime: start, EndTime: start.Add(dur), Calendar: "Arbeit"}
}

func run(t *testing.T, args ...string) (out string, err error) {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	rootCmd.SetArgs(args)
	err = rootCmd.Execute()
	w.Close()
	os.Stdout = old
	b, _ := io.ReadAll(r)
	return string(b), err
}

func mustRun(t *testing.T, args ...string) string {
	t.Helper()
	out, err := run(t, args...)
	if err != nil {
		t.Fatalf("calctl %v: %v\n%s", args, err, out)
	}
	return out
}

func TestListTodayTextAndJSON(t *testing.T) {
	setup(t,
		ev("1", "Standup", today(9, 30), 30*time.Minute),
		ev("2", "Zahnarzt", today(14, 0), time.Hour),
		ev("3", "Tomorrow", today(10, 0).AddDate(0, 0, 1), time.Hour),
	)
	text := mustRun(t, "list", "--today")
	for _, want := range []string{"Standup", "09:30–10:00", "Zahnarzt", "[Arbeit]"} {
		if !strings.Contains(text, want) {
			t.Errorf("text lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Tomorrow") {
		t.Errorf("--today must exclude tomorrow:\n%s", text)
	}

	var resp listResponse
	if err := json.Unmarshal([]byte(mustRun(t, "list", "--today", "--format", "json")), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Tool != "calctl" || resp.Command != "list" || resp.Count != 2 || len(resp.Data) != 2 || resp.From != resp.To {
		t.Errorf("envelope = %+v", resp)
	}
}

func TestListDefaultsToTodayAndRangeFlags(t *testing.T) {
	setup(t, ev("1", "Now", today(12, 0), time.Hour), ev("2", "Later", today(12, 0).AddDate(0, 0, 3), time.Hour))
	if out := mustRun(t, "list"); !strings.Contains(out, "Now") || strings.Contains(out, "Later") {
		t.Errorf("default range is today:\n%s", out)
	}
	to := today(0, 0).AddDate(0, 0, 5).Format("2006-01-02")
	from := today(0, 0).Format("2006-01-02")
	out := mustRun(t, "list", "--from", from, "--to", to)
	if !strings.Contains(out, "Now") || !strings.Contains(out, "Later") {
		t.Errorf("--from/--to must include both:\n%s", out)
	}
	if _, err := run(t, "list", "--from", "garbage", "--to", to); err == nil || !strings.Contains(err.Error(), "--from") {
		t.Errorf("bad --from must be reported, got %v", err)
	}
}

func TestListEmpty(t *testing.T) {
	setup(t)
	if out := mustRun(t, "list"); !strings.Contains(out, "No events between") {
		t.Errorf("out = %q", out)
	}
}

func TestExportToFileAndStdout(t *testing.T) {
	setup(t, ev("1", "Export me", today(11, 0), time.Hour))
	path := filepath.Join(t.TempDir(), "out.json")
	_ = mustRun(t, "export", "--week", "--format", "json", "-o", path)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var resp listResponse
	if err := json.Unmarshal(b, &resp); err != nil || resp.Command != "export" || resp.Count != 1 {
		t.Errorf("file = %s (%v)", b, err)
	}
	exportOutput = ""
	if out := mustRun(t, "export", "--week"); !strings.Contains(out, "Export me") {
		t.Errorf("text export to stdout:\n%s", out)
	}
	exportOutput = filepath.Join(t.TempDir(), "no", "such", "dir", "x")
	if _, err := run(t, "export", "--week", "--output", exportOutput); err == nil {
		t.Error("an unwritable output path must be an error")
	}
}

func TestFreeSlotsAroundEvents(t *testing.T) {
	setup(t, ev("1", "Block", today(10, 0), 2*time.Hour)) // 10:00–12:00
	var resp freeResponse
	if err := json.Unmarshal([]byte(mustRun(t, "free", "--next", "1", "--format", "json")), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.MinMinutes != 30 || resp.WorkingFrom != "09:00" || resp.WorkingTo != "18:00" {
		t.Errorf("defaults not applied: %+v", resp)
	}
	var got []string
	for _, s := range resp.Data {
		got = append(got, s.Start[11:16]+"-"+s.End[11:16])
	}
	if strings.Join(got, ",") != "09:00-10:00,12:00-18:00" {
		t.Errorf("slots = %v, want 09:00-10:00 and 12:00-18:00", got)
	}

	// --min filters out the short gap
	freeMin = 0
	_ = json.Unmarshal([]byte(mustRun(t, "free", "--next", "1", "--format", "json", "--min", "90")), &resp)
	if resp.MinMinutes != 90 || len(resp.Data) != 1 || resp.Data[0].DurationMinutes != 360 {
		t.Errorf("--min 90: %+v", resp)
	}

	formatFlag, freeMin = "human", 0
	text := mustRun(t, "free", "--next", "1")
	if !strings.Contains(text, "09:00 – 10:00  (1h)") || !strings.Contains(text, "Free slots (min 30min, 09:00–18:00)") {
		t.Errorf("text:\n%s", text)
	}
}

func TestVersionCommand(t *testing.T) {
	setup(t)
	if out := mustRun(t, "version"); !strings.HasPrefix(out, "calctl ") {
		t.Errorf("out = %q", out)
	}
}

func TestLicenseStatusWithoutKey(t *testing.T) {
	setup(t)
	out := mustRun(t, "license", "status")
	if !strings.Contains(out, "CORE (free)") {
		t.Errorf("out = %q", out)
	}
}

func TestSummarizeIsGatedWithoutLicense(t *testing.T) {
	setup(t)
	if _, err := run(t, "summarize"); err == nil || !strings.Contains(err.Error(), "Bundle feature") {
		t.Errorf("err = %v, want the Bundle gate", err)
	}
}

func TestResolveRange(t *testing.T) {
	now := time.Now()
	from, to, err := resolveRange(true, false, "", "")
	if err != nil || !from.Equal(dateutil.StartOfDay(now)) || !to.Equal(dateutil.EndOfDay(now)) {
		t.Errorf("today: %v %v %v", from, to, err)
	}
	mon, sun, _ := resolveRange(false, true, "", "")
	if mon.Weekday() != time.Monday || sun.Weekday() != time.Sunday {
		t.Errorf("week = %v..%v", mon, sun)
	}
	from, to, err = resolveRange(false, false, "2026-10-01", "2026-10-31")
	if err != nil || from.Day() != 1 || to.Day() != 31 || to.Hour() != 23 {
		t.Errorf("explicit range must end at the end of the last day: %v %v %v", from, to, err)
	}
	if _, _, err = resolveRange(false, false, "2026-10-01", "nope"); err == nil || !strings.Contains(err.Error(), "--to") {
		t.Errorf("bad --to: %v", err)
	}
	// only one bound given → falls back to today rather than half a range
	from, _, _ = resolveRange(false, false, "2026-10-01", "")
	if !from.Equal(dateutil.StartOfDay(now)) {
		t.Errorf("a lone --from must be ignored, got %v", from)
	}
}

func TestPrintEventsToFormatting(t *testing.T) {
	d := time.Date(2026, 10, 6, 9, 0, 0, 0, time.Local)
	var b bytes.Buffer
	printEventsTo(&b, []models.Event{
		{Title: "Timed", StartTime: d, EndTime: d.Add(time.Hour), Calendar: "Work", Location: "Büro 3"},
		{Title: "Holiday", StartTime: d, EndTime: d.Add(24 * time.Hour), AllDay: true, Location: "missing value"},
		{Title: "Next day", StartTime: d.AddDate(0, 0, 1), EndTime: d.AddDate(0, 0, 1).Add(time.Hour)},
	}, d, d.AddDate(0, 0, 2))
	out := b.String()
	for _, want := range []string{"09:00–10:00  Timed  [Work]", "@ Büro 3", "all day  Holiday", "Tue, Oct 06", "Wed, Oct 07"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "missing value") {
		t.Error("AppleScript's \"missing value\" placeholder must never be shown as a location")
	}
	if strings.Count(out, "Tue, Oct 06") != 1 {
		t.Error("same-day events share one date header")
	}
}

func TestBuildRecurrenceRule(t *testing.T) {
	for _, c := range []struct {
		repeat string
		count  int
		until  string
		want   string
		err    string
	}{
		{"", 0, "", "", ""},
		{"weekly", 0, "", "FREQ=WEEKLY", ""},
		{"DAILY", 5, "", "FREQ=DAILY;COUNT=5", ""},
		{"monthly", 0, "", "FREQ=MONTHLY", ""},
		{"yearly", 2, "", "FREQ=YEARLY;COUNT=2", ""},
		{"weekly", 3, "2026-12-01", "", "mutually exclusive"},
		{"hourly", 0, "", "", "invalid --repeat"},
		{"weekly", 0, "soon", "", "invalid --until"},
	} {
		got, err := buildRecurrenceRule(c.repeat, c.count, c.until)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%+v: err = %v, want %q", c, err, c.err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%+v = %q, %v; want %q", c, got, err, c.want)
		}
	}
}

// UNTIL has to reach the END of the last day: at 00:00 the final day's own
// occurrence (e.g. 09:00) would fall after it and be dropped.
func TestRecurrenceUntilIncludesTheLastDay(t *testing.T) {
	rule, err := buildRecurrenceRule("daily", 0, "2026-12-24")
	if err != nil {
		t.Fatal(err)
	}
	until := strings.TrimPrefix(rule[strings.Index(rule, "UNTIL="):], "UNTIL=")
	got, err := time.Parse("20060102T150405Z", until)
	if err != nil {
		t.Fatal(err)
	}
	lastOccurrence := time.Date(2026, 12, 24, 9, 0, 0, 0, time.Local)
	if lastOccurrence.After(got) {
		t.Errorf("UNTIL=%s is before the last day's 09:00 occurrence (%v) — it would be excluded", until, lastOccurrence.UTC())
	}
	if dayAfter := time.Date(2026, 12, 25, 0, 0, 0, 0, time.Local); got.After(dayAfter) {
		t.Errorf("UNTIL=%s runs past the last day", until)
	}
}

func TestFindBestMatch(t *testing.T) {
	events := []models.Event{
		{Title: "Holiday", AllDay: true},
		{Title: "Sprint Planning"},
		{Title: "Design Review"},
		{Title: "Weekly Standup"},
	}
	if findBestMatch(nil, "x") != nil {
		t.Error("no events → nil")
	}
	if got := findBestMatch(events, ""); got.Title != "Sprint Planning" {
		t.Errorf("empty query picks the first timed event, got %q", got.Title)
	}
	if got := findBestMatch(events[:1], ""); got.Title != "Holiday" {
		t.Errorf("only an all-day event: got %q", got.Title)
	}
	if got := findBestMatch(events, "design"); got.Title != "Design Review" {
		t.Errorf("substring match: %q", got.Title)
	}
	if got := findBestMatch(events, "planning sprint"); got.Title != "Sprint Planning" {
		t.Errorf("word-level match: %q", got.Title)
	}
	if got := findBestMatch(events, "zzz"); got.Title != "Holiday" {
		t.Errorf("no match falls back to the first event, got %q", got.Title)
	}
}

func TestWriteDraftFileKeepsFrontmatterValid(t *testing.T) {
	e := &models.Event{Title: `Q3 "Kickoff" \ Review`, Attendees: []string{"a@x.test", "b@x.test"}}
	path, err := writeDraftFile(e, "Summary body")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(path) })
	b, _ := os.ReadFile(path)
	text := string(b)
	if !strings.HasPrefix(text, "---\nto:\n  - a@x.test\n  - b@x.test\nsubject: ") {
		t.Errorf("frontmatter head:\n%s", text)
	}
	// the subject line must be a single properly quoted scalar
	line := strings.Split(text, "\n")[4]
	want := `subject: "Meeting Summary: Q3 \"Kickoff\" \\ Review"`
	if line != want {
		t.Errorf("subject line = %s\nwant           %s", line, want)
	}
	if !strings.HasSuffix(text, "\n---\n\nSummary body\n") {
		t.Errorf("body missing:\n%s", text)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, drafts hold meeting content and must be private", fi.Mode().Perm())
	}
}

func TestCollectMarkdownFiles(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"a.md", "b.markdown", "c.txt", "d.MD"} {
		os.WriteFile(filepath.Join(dir, f), nil, 0o644)
	}
	os.Mkdir(filepath.Join(dir, "sub.md"), 0o755) // a directory named like a file
	got, err := collectMarkdownFiles(dir)
	if err != nil || len(got) != 2 {
		t.Fatalf("files = %v, %v; want a.md and b.markdown only", got, err)
	}
	single, _ := collectMarkdownFiles(filepath.Join(dir, "c.txt"))
	if len(single) != 1 {
		t.Error("a single file path is returned as is")
	}
	if _, err := collectMarkdownFiles(filepath.Join(dir, "missing")); err == nil {
		t.Error("a missing path must be an error")
	}
}

func TestFormatSlotsAndLabel(t *testing.T) {
	s := time.Date(2026, 10, 6, 9, 0, 0, 0, time.Local)
	out := formatSlots([]models.FreeSlot{{Start: s, End: s.Add(90 * time.Minute), Duration: 90 * time.Minute, Date: "2026-10-06"}})
	if len(out) != 1 || out[0].DurationMinutes != 90 || out[0].Start != "2026-10-06T09:00:00" || out[0].End != "2026-10-06T10:30:00" {
		t.Errorf("slots = %+v", out)
	}
	if formatDateLabel(s) != "Tue, Oct 06" {
		t.Errorf("label = %q", formatDateLabel(s))
	}
}

func TestWriteDraftFileUsesFreshPrivateFile(t *testing.T) {
	e := &models.Event{Title: "Standup"}
	a, err := writeDraftFile(e, "x")
	if err != nil {
		t.Fatal(err)
	}
	b, err := writeDraftFile(e, "y")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(a); os.Remove(b) })
	if a == b {
		t.Errorf("same title must not reuse a predictable path: %s", a)
	}
	if fi, _ := os.Stat(a); fi == nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("draft must be 0600, got %v", fi)
	}
}
