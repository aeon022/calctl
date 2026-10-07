package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/aeon022/missionctl-core/tuitest"
	"github.com/charmbracelet/x/ansi"
)

// Every view/mode calctl has, each left again via esc. Commands returned by
// Update are never run (no DB, no Calendar.app).
var smokeKeys = []string{
	"j", "k", "down", "up", "right", "left", // list, week navigation
	"enter", "esc", // detail
	"f", "esc", // free slots
	"n", "tab", "tab", "esc", // create form
	"e", "esc", // edit form
	"?", "j", "esc", // help popup, scrolled
	"c", "down", "esc", // calendar picker
	"/", "s", "esc", // filter
	":", "esc", // command palette
	"d", "n", // delete confirm → cancel
	"u", "+", "-", "r",
}

func TestSmokeWideAndPopulated(t *testing.T) { tuitest.Smoke(t, loaded(t), smokeKeys...) }

func TestSmokeEmptyDataAndNoMatchState(t *testing.T) {
	isolate(t)
	m, _ := tuitest.Send(New(), tuitest.Resize(100, 30), eventsLoadedMsg{})
	tuitest.Smoke(t, m, smokeKeys...)
	// An empty week still lists its days; the empty state shows when a search matches nothing.
	m, _ = tuitest.Keys(m, "/", "z", "z", "z")
	if out := tuitest.Text(m); !strings.Contains(out, "No events match your search") {
		t.Errorf("no-match search should show the empty state:\n%s", out)
	}
}

// Narrow and short terminals: no panics, never an empty frame.
func TestSmokeSmallTerminal(t *testing.T) {
	mi, _ := tuitest.Send(loaded(t), tuitest.Resize(60, 15))
	for _, k := range smokeKeys {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic at 60x15 after key %q: %v", k, r)
				}
			}()
			mi, _ = tuitest.Keys(mi, k)
			if strings.TrimSpace(tuitest.Text(mi)) == "" {
				t.Fatalf("empty view at 60x15 after key %q", k)
			}
		}()
	}
}

func TestFooterAndHeaderNeverWiderThanTerminal(t *testing.T) {
	for _, w := range []int{40, 60, 80, 100, 140} {
		m, _ := tuitest.Send(loaded(t), tuitest.Resize(w, 30))
		mm := m.(Model)
		lines := map[string]string{"footer": mm.renderStatusBar()}
		for i, l := range strings.Split(mm.headerBlock(), "\n") {
			lines[fmt.Sprintf("header line %d", i)] = l
		}
		for name, line := range lines {
			if lw := lipgloss.Width(line); lw > w {
				t.Errorf("width %d: %s is %d cells wide: %q", w, name, lw, ansi.Strip(line))
			}
		}
	}
}

func TestFooterKeepsMostImportantHintsWhenNarrow(t *testing.T) {
	m, _ := tuitest.Send(loaded(t), tuitest.Resize(40, 30))
	out := ansi.Strip(m.(Model).renderStatusBar())
	// "? help" and "q quit" sit among the first hints so they are dropped last
	if !strings.Contains(out, "? help") || !strings.Contains(out, "q quit") || strings.Contains(out, "copy") || strings.Contains(out, "free") {
		t.Errorf("narrow footer should keep the first hints and drop the last: %q", out)
	}
}

func TestEmptyDayShowsNoBogusTimeRange(t *testing.T) {
	m, _ := tuitest.Send(loaded(t), tuitest.Resize(110, 28))
	text := tuitest.Text(m)
	if !strings.Contains(text, "nothing planned") {
		t.Fatalf("test data should contain a collapsed empty day:\n%s", text)
	}
	if strings.Contains(text, "00:00–00:00") || strings.Contains(text, "(no events)") {
		t.Errorf("an empty day collapses to one 'nothing planned' line, no bogus range or placeholder:\n%s", text)
	}
}
