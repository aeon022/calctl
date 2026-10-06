package tui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/aeon022/calctl/internal/models"
	"github.com/aeon022/missionctl-core/tuitest"
)

func screenLines(m Model) []string {
	var out []string
	for _, l := range strings.Split(tuitest.Text(m), "\n") {
		out = append(out, strings.TrimRight(l, " "))
	}
	return out
}

var banner = regexp.MustCompile(`^  (TODAY — )?(Mon|Tue|Wed|Thu|Fri|Sat|Sun) \d\d [A-Z][a-z]{2}$`)

// Every day banner except the first on screen has a blank line above it, so
// "(no events)" of one day can't read as belonging to the next day's header.
func TestBlankLineBeforeEveryDayBannerButTheFirst(t *testing.T) {
	m := loaded(t)
	lines := screenLines(m)
	var at []int
	for i, l := range lines {
		if banner.MatchString(l) {
			at = append(at, i)
		}
	}
	if len(at) < 5 {
		t.Fatalf("expected a week of day banners, found %d:\n%s", len(at), strings.Join(lines, "\n"))
	}
	for k, i := range at {
		if k == 0 {
			continue // the first banner follows the header block, no gap needed
		}
		if strings.TrimSpace(lines[i-1]) != "" {
			t.Errorf("banner %q needs a blank line above it, got %q", lines[i], lines[i-1])
		}
		// the order stays banner → divider → events
		if !strings.Contains(lines[i+1], "──") {
			t.Errorf("banner %q must be followed by its divider, got %q", lines[i], lines[i+1])
		}
	}
	// the day before TODAY: "(no events)" sits right above a blank line, then TODAY
	for i, l := range lines {
		if strings.Contains(l, "TODAY") {
			if strings.TrimSpace(lines[i-1]) != "" {
				t.Errorf("TODAY banner is glued to the previous day: %q", lines[i-1])
			}
		}
	}
}

// Clicks land on the right event on both sides of a day gap; banners, dividers
// and the blank gap select nothing.
func TestClickMappingAcrossDayGaps(t *testing.T) {
	m := loaded(t)
	lines := screenLines(m)
	// the "(no events)" placeholders share one title: pair them by order of appearance
	var phRows, phLines []int
	for i, r := range m.rows {
		if r.event != nil && r.event.Title == "(no events)" {
			phRows = append(phRows, i)
		}
	}
	for ly, l := range lines {
		if strings.Contains(l, "(no events)") {
			phLines = append(phLines, ly)
		}
	}
	if len(phRows) != len(phLines) {
		t.Fatalf("%d placeholder rows but %d on screen", len(phRows), len(phLines))
	}
	for k := range phRows {
		if got := m.rowHitTest(phLines[k]); got != phRows[k] {
			t.Errorf("click on the %d. '(no events)' (line %d) hit row %d, want %d", k+1, phLines[k], got, phRows[k])
		}
	}
	for i, r := range m.rows {
		if r.isHeader || r.event == nil || r.event.Title == "(no events)" {
			continue
		}
		y := -1
		for ly, l := range lines {
			if strings.Contains(l, r.event.Title) {
				y = ly
			}
		}
		if y < 0 {
			continue
		}
		if got := m.rowHitTest(y); got != i {
			t.Errorf("click on %q (screen line %d) hit row %d, want %d", r.event.Title, y, got, i)
		}
	}
	for ly, l := range lines {
		isBanner := banner.MatchString(l)
		isDivider := strings.Contains(l, "────")
		isGap := strings.TrimSpace(l) == "" && ly+1 < len(lines) && banner.MatchString(lines[ly+1])
		if (isBanner || isDivider || isGap) && ly > 3 && ly < m.height-3 {
			if got := m.rowHitTest(ly); got != -1 {
				t.Errorf("screen line %d (%q) must not select a row, hit %d", ly, l, got)
			}
		}
	}
}

// With many single-event days and a short window the visible rows never need
// more lines than the room, the cursor stays on screen, and clicks still map.
func TestScrollWindowWithDayGapsFitsAndKeepsCursorVisible(t *testing.T) {
	isolate(t)
	var evs []models.Event
	for d := 0; d < 30; d++ {
		evs = append(evs, models.Event{ID: fmt.Sprintf("e%d", d), Title: fmt.Sprintf("event-%02d", d),
			StartTime: at(d, 10, 0), EndTime: at(d, 11, 0)})
	}
	for _, h := range []int{14, 20, 26} {
		m := New()
		m, _ = send(m, tea.WindowSizeMsg{Width: 100, Height: h}, eventsLoadedMsg{events: evs})
		m, _ = send(m, press("+"), press("+"), press("+"))
		for step := 0; step < 30; step++ {
			m, _ = send(m, press("j"))
			room := m.height - 6
			visible, start := m.visibleRowsWithStart(room)
			used := 0
			for j := range visible {
				used += m.rowLines(start + j)
			}
			if used > room {
				t.Fatalf("h=%d step %d: window needs %d lines, room %d", h, step, used, room)
			}
			if m.cursor < start || m.cursor >= start+len(visible) {
				t.Fatalf("h=%d step %d: cursor %d outside window [%d,%d)", h, step, m.cursor, start, start+len(visible))
			}
			if e := cur(m); e != nil {
				y := -1
				for ly, l := range screenLines(m) {
					if strings.Contains(l, e.Title) {
						y = ly
					}
				}
				if y < 0 || m.rowHitTest(y) != m.cursor {
					t.Fatalf("h=%d step %d: cursor event %q: line %d, hit %d", h, step, e.Title, y, m.rowHitTest(y))
				}
			}
		}
	}
}

// One language, one date format: English weekday names in the week strip, the
// day banners and the header, all as "Tue 06 Oct".
func TestOneLanguageAndOneDateFormat(t *testing.T) {
	m := loaded(t)
	text := tuitest.Text(m)
	for _, de := range []string{" Di ", " Mi ", " Do ", " Fr ", " Sa ", " So ", "Mo 0"} {
		if strings.Contains(text, de) {
			t.Errorf("German weekday %q left over:\n%s", de, text)
		}
	}
	now := time.Now()
	wantHeader := now.Format("Mon 02 Jan")
	if !strings.Contains(strings.Split(text, "\n")[0], wantHeader) {
		t.Errorf("header should show %q:\n%s", wantHeader, strings.Split(text, "\n")[0])
	}
	// the strip: "Mon 05 Tue 06 …" for the visible week
	strip := m.renderWeekNav()
	if !strings.Contains(tuitestStrip(strip), weekStart(0).Format("Mon 02")) {
		t.Errorf("week strip should read like %q: %q", weekStart(0).Format("Mon 02"), tuitestStrip(strip))
	}
	if strings.Contains(text, ", Oct") || regexp.MustCompile(`(Mon|Tue|Wed|Thu|Fri|Sat|Sun), [A-Z][a-z]{2} \d\d`).MatchString(text) {
		t.Errorf("old 'Tue, Oct 06' style still present:\n%s", text)
	}
}

func tuitestStrip(s string) string {
	return regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(s, "")
}
