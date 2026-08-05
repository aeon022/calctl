package calendar

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// Regression test for the DST bug: DeleteEvent/CreateEvent used to build the
// AppleScript target date as "(current date) + (targetEpoch - nowEpoch)",
// which silently drifted by the DST offset whenever "now" and the target
// date sat on opposite sides of a daylight-saving transition. Property
// assignment must produce the exact requested wall-clock fields regardless
// of what date the test happens to run on.
func TestAppleScriptSetDateUsesExactFields(t *testing.T) {
	cases := []struct {
		name string
		t    time.Time
	}{
		{"pre-DST-end", time.Date(2026, 9, 4, 0, 0, 0, 0, time.Local)},
		{"post-DST-end", time.Date(2026, 11, 6, 0, 0, 0, 0, time.Local)},
		{"with-time", time.Date(2026, 11, 18, 19, 0, 0, 0, time.Local)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			script := appleScriptSetDate("targetDate", c.t)

			if strings.Contains(script, "do shell script") {
				t.Fatalf("script still shells out for epoch math, want pure property assignment:\n%s", script)
			}

			wantSeconds := c.t.Hour()*3600 + c.t.Minute()*60 + c.t.Second()
			checks := []string{
				"set year of targetDate to 2026",
				"set day of targetDate to " + strconv.Itoa(c.t.Day()),
				"set time of targetDate to " + strconv.Itoa(wantSeconds),
			}
			for _, want := range checks {
				if !strings.Contains(script, want) {
					t.Errorf("script missing %q:\n%s", want, script)
				}
			}
		})
	}
}
