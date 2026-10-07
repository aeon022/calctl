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

// a day line: "  Tue 06 Oct  · 2 events", "  TODAY  Wed 07 Oct  · 1 event" or the
// collapsed "  Fri 09 · nothing planned"
var dayLine = regexp.MustCompile(`^  (TODAY +)?(Mon|Tue|Wed|Thu|Fri|Sat|Sun) \d\d( [A-Z][a-z]{2})?( +· \d+ events?| · nothing planned)$`)

// Every day line except the first on screen has a blank line above it, and the
// old header → rule → "(no events)" triple is gone: no rules inside the body.
func TestBlankLineBeforeEveryDayLineButTheFirst(t *testing.T) {
	m := loaded(t)
	lines := screenLines(m)
	var at []int
	for i, l := range lines {
		if dayLine.MatchString(l) {
			at = append(at, i)
		}
	}
	if len(at) < 7 {
		t.Fatalf("expected a line per day of the week, found %d:\n%s", len(at), strings.Join(lines, "\n"))
	}
	for k, i := range at {
		if k > 0 && strings.TrimSpace(lines[i-1]) != "" {
			t.Errorf("day line %q needs a blank line above it, got %q", lines[i], lines[i-1])
		}
	}
	for i, l := range lines {
		if i > 1 && strings.Contains(l, "────") && !strings.Contains(l, "now") {
			t.Errorf("line %d: no rules in the body any more: %q", i, l)
		}
	}
}

// Clicks land on the right event; day lines, the blank gap and the now line
// select nothing. Walks every screen line against rowHitTest.
func TestClickMappingAcrossDayGaps(t *testing.T) {
	m := loaded(t)
	lines := screenLines(m)
	for i, r := range m.rows {
		if r.isHeader || r.event == nil {
			continue
		}
		y := -1
		for ly, l := range lines {
			if strings.Contains(l, r.event.Title) {
				y = ly
			}
		}
		if y < 0 {
			t.Fatalf("event %q not on screen", r.event.Title)
		}
		if got := m.rowHitTest(y); got != i {
			t.Errorf("click on %q (screen line %d) hit row %d, want %d", r.event.Title, y, got, i)
		}
	}
	for ly, l := range lines {
		blankGap := strings.TrimSpace(l) == "" && ly+1 < len(lines) && dayLine.MatchString(lines[ly+1])
		if (dayLine.MatchString(l) || blankGap || strings.Contains(l, "── now")) && ly >= headerLines {
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
			_, _, _, lh := m.listArea()
			room := lh - m.barLines()
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
			if e := m.selectedEvent(); e != nil {
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
// day lines and the header, all as "Tue 06 Oct".
func TestOneLanguageAndOneDateFormat(t *testing.T) {
	m := loaded(t)
	text := tuitest.Text(m)
	for _, de := range []string{" Di ", " Mi ", " Do ", " Fr ", " Sa ", " So ", "Mo 0"} {
		if strings.Contains(text, de) {
			t.Errorf("German weekday %q left over:\n%s", de, text)
		}
	}
	lines := strings.Split(text, "\n")
	if want := time.Now().Format("Mon 02 Jan"); !strings.Contains(lines[0], want) {
		t.Errorf("header should show %q:\n%s", want, lines[0])
	}
	if want := weekStart(0).Format("Mon 02"); !strings.Contains(lines[stripRow], want) {
		t.Errorf("week strip should read like %q: %q", want, lines[stripRow])
	}
	if strings.Contains(text, ", Oct") || regexp.MustCompile(`(Mon|Tue|Wed|Thu|Fri|Sat|Sun), [A-Z][a-z]{2} \d\d`).MatchString(text) {
		t.Errorf("old 'Tue, Oct 06' style still present:\n%s", text)
	}
}
