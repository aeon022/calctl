package tui

import (
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/aeon022/calctl/internal/models"
)

func browsingModel() Model {
	m := New()
	m.width, m.height = 100, 30
	m.loading = false
	start := time.Now().Add(time.Hour)
	m.events = []models.Event{
		{ID: "e1", Title: "A", StartTime: start, EndTime: start.Add(time.Hour)},
		{ID: "e2", Title: "B", StartTime: start.Add(2 * time.Hour), EndTime: start.Add(3 * time.Hour)},
	}
	m.rows = buildRows(m.events, m.weekOffset, m.daysAhead, "")
	m.lastLoad = time.Now().Add(-time.Minute) // stale
	return m
}

func eventRowIndex(m Model, id string) int {
	for i, r := range m.rows {
		if r.event != nil && r.event.ID == id {
			return i
		}
	}
	return -1
}

func TestFocusReloadsWhenBrowsingAndStale(t *testing.T) {
	mi, cmd := browsingModel().Update(tea.FocusMsg{})
	if cmd == nil || !mi.(Model).focusLoading {
		t.Errorf("stale browse state must reload on focus: cmd=%v focusLoading=%v", cmd != nil, mi.(Model).focusLoading)
	}
}

func TestFocusDoesNotReloadWhenFreshOrBusy(t *testing.T) {
	cases := map[string]func(*Model){
		"fresh":        func(m *Model) { m.lastLoad = time.Now() },
		"form":         func(m *Model) { m.view = viewCreate },
		"detail":       func(m *Model) { m.view = viewDetail },
		"search":       func(m *Model) { m.searching = true },
		"palette":      func(m *Model) { m.inPalette = true },
		"delete":       func(m *Model) { m.deleteTarget = &m.events[0] },
		"syncing":      func(m *Model) { m.syncing = true },
		"already busy": func(m *Model) { m.focusLoading = true },
		"first load":   func(m *Model) { m.loading = true },
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			m := browsingModel()
			setup(&m)
			if _, cmd := m.Update(tea.FocusMsg{}); cmd != nil {
				t.Errorf("%s: focus must not trigger a reload", name)
			}
		})
	}
}

func TestFocusReloadKeepsCursorOnSameEvent(t *testing.T) {
	m := browsingModel()
	m.cursor = eventRowIndex(m, "e2")
	mi, _ := m.Update(tea.FocusMsg{})
	// the reloaded list arrives with a new event in front, shifting rows
	start := time.Now().Add(30 * time.Minute)
	fresh := append([]models.Event{{ID: "e0", Title: "new", StartTime: start, EndTime: start.Add(time.Minute)}}, m.events...)
	mi, _ = mi.Update(eventsLoadedMsg{events: fresh})
	m = mi.(Model)
	if r := m.rows[m.cursor]; r.event == nil || r.event.ID != "e2" {
		t.Errorf("cursor left event e2 after focus reload: %+v", r)
	}
	if m.focusLoading || time.Since(m.lastLoad) > time.Second {
		t.Errorf("load bookkeeping not reset: focusLoading=%v lastLoad=%v", m.focusLoading, m.lastLoad)
	}
}

func TestViewReportsFocus(t *testing.T) {
	if !browsingModel().View().ReportFocus {
		t.Error("View must set ReportFocus so FocusMsg is delivered")
	}
}

func TestCopyUsesOSC52AndPbcopy(t *testing.T) {
	msg := copyToClipboardCmd("hello")()
	batch, ok := msg.(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("want a 2-command batch (OSC 52 + pbcopy), got %T %v", msg, msg)
	}
	var osc bool
	for _, c := range batch {
		if strings.Contains(runtime.FuncForPC(reflect.ValueOf(c).Pointer()).Name(), "SetClipboard") {
			osc = true // identified by name — executing the pbcopy half would touch the real clipboard
		}
	}
	if !osc {
		t.Error("batch must contain tea.SetClipboard so copying works over SSH/tmux")
	}
}
