package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/aeon022/missionctl-core/tuitest"
	"github.com/charmbracelet/x/ansi"
)

// Secondary views share the header (calctl · section), a titled panel and a
// one-line footer, at constant height and never wider than the terminal.
func TestSecondaryViewsShareTheChrome(t *testing.T) {
	for name, tc := range map[string]struct {
		keys   []string
		title  string
		footer string
	}{
		"detail": {[]string{"enter"}, "Event", "esc back"},
		"free":   {[]string{"f"}, "Free Slots", "esc back"},
		"create": {[]string{"n"}, "New Event", "ctrl+s"},
		"edit":   {[]string{"e"}, "Edit Event", "ctrl+s"},
		"help":   {[]string{"?"}, "Help", ""},
		"cals":   {[]string{"c"}, "Default Calendar", ""},
	} {
		for _, w := range []int{40, 60, 100, 170} {
			t.Run(fmt.Sprintf("%s/%d", name, w), func(t *testing.T) {
				m, _ := tuitest.Send(loaded(t), tuitest.Resize(w, 30))
				m, _ = tuitest.Keys(m, tc.keys...)
				lines := strings.Split(ansi.Strip(tuitest.Text(m)), "\n")
				if len(lines) != 29 { // Update keeps one row of slack (bubbletea#304)
					t.Errorf("height = %d, want 29", len(lines))
				}
				for _, l := range lines {
					if lipgloss.Width(l) > w {
						t.Fatalf("line wider than %d: %q", w, l)
					}
				}
				all := strings.Join(lines, "\n")
				if !strings.Contains(all, tc.title) || !strings.Contains(all, "╭") {
					t.Errorf("missing panel title %q:\n%s", tc.title, all)
				}
				if tc.footer != "" && w >= 60 && !strings.Contains(lines[len(lines)-1], tc.footer) {
					t.Errorf("footer lacks %q: %q", tc.footer, lines[len(lines)-1])
				}
			})
		}
	}
}
