# calctl — Calendar from your Terminal

`calctl` is a lightweight, local-first CLI and TUI tool written in Go to read and write calendar events directly from Markdown files. Part of the [missionctl](../README.md) suite — built for developer workflows and AI-assisted scheduling.

---

## Features

- **Markdown-First**: Write events as `.md` files with YAML frontmatter — import them into Apple Calendar with one command
- **Terminal UI**: Interactive week view with keyboard navigation, event detail, and free-slot finder
- **Apple Calendar sync**: Pulls your events into a local SQLite cache via AppleScript — no API keys needed
- **Free Slot Finder**: Finds gaps in your calendar within your configured working hours
- **AI-ready**: All read commands output clean JSON, piping directly into Claude or any LLM
- **Local-first**: All data stored in `~/.config/calctl/calctl.db` — nothing leaves your machine

> Google Calendar support is planned for v0.5.

---

## Installation

### Requirements

- macOS (Apple Calendar integration uses AppleScript)
- Go 1.23+ — install via `brew install go`

### Build from source

```bash
git clone https://github.com/aeon022/calctl
cd calctl
chmod +x setup.sh
./setup.sh
```

The setup script builds the binary and optionally installs it to `/usr/local/bin`.

### Manual build

```bash
go build -o calctl .
./calctl --help
```

---

## Quick Start

```bash
# 1. Sync your Apple Calendar (macOS will ask for permission once)
calctl sync

# 2. See today's events
calctl list --today

# 3. Open the TUI
calctl
```

---

## Commands

### `calctl` (no arguments)

Opens the interactive TUI.

```bash
calctl
```

---

### `calctl sync`

Pulls events from Apple Calendar into the local SQLite cache.

```bash
calctl sync              # sync next 30 days (default)
calctl sync --days 60   # sync next 60 days
calctl sync --format json
```

> On first run, macOS will show a permission dialog: **"calctl wants access to your calendars"** — click Allow.

---

### `calctl list`

Lists events from the local cache.

```bash
calctl list                                   # today (default)
calctl list --today                           # today
calctl list --week                            # current week (Mon–Sun)
calctl list --from 2026-10-01 --to 2026-10-31
calctl list --today --format json             # machine-readable output
calctl list --week --sync                     # sync first, then list
```

**JSON output** (for AI pipelines):

```json
{
  "tool": "calctl",
  "command": "list",
  "from": "2026-10-01",
  "to": "2026-10-07",
  "count": 3,
  "data": [
    {
      "id": "apple-abc123",
      "title": "Team Standup",
      "start_time": "2026-10-01T09:00:00+02:00",
      "end_time": "2026-10-01T09:30:00+02:00",
      "all_day": false,
      "calendar": "Work",
      "location": "",
      "notes": "",
      "attendees": [],
      "source": "apple"
    }
  ]
}
```

---

### `calctl free`

Finds free time slots within your configured working hours.

```bash
calctl free                      # next 7 days, min 30min slots
calctl free --next 14            # next 14 days
calctl free --min 60             # only slots ≥ 60 minutes
calctl free --next 7 --format json
```

Working hours default to `09:00–18:00`, Mon–Fri. Change them in your [config file](#configuration).

**Example output:**

```
Free slots (min 30min, 09:00–18:00):

Mon, Jun 29
  09:00 – 11:00  (2h)
  14:30 – 18:00  (3h30m)

Tue, Jun 30
  09:00 – 18:00  (9h)
```

---

### `calctl import`

Creates calendar events in Apple Calendar from Markdown files.

```bash
calctl import event.md              # import single file
calctl import ./events/             # import all .md files in folder
calctl import event.md --dry-run    # validate without creating
calctl import --format json         # JSON output
```

#### Markdown frontmatter format

```markdown
---
title: Product Launch Call
date: 2026-10-15
time: 14:00
duration: 60min
calendar: Work
location: Zoom
attendees:
  - jan@example.com
  - lisa@example.com
---

Optional notes or agenda go here as Markdown body.
They are added to the event's Notes field.
```

**Frontmatter fields:**

| Field | Required | Format | Example |
|-------|----------|--------|---------|
| `title` | Yes | string | `Product Launch Call` |
| `date` | Yes | `YYYY-MM-DD` | `2026-10-15` |
| `time` | No | `HH:MM` (24h) | `14:00` — defaults to `09:00` |
| `duration` | No | `60min`, `1h30m`, `90` | `60min` — defaults to `60min` |
| `calendar` | No | string | `Work` — defaults to your primary calendar |
| `location` | No | string | `Zoom`, `Office`, `Berlin` |
| `attendees` | No | list of emails | `[jan@example.com]` |
| `all_day` | No | `true` / `false` | `false` |
| `notes` | No | string | overrides Markdown body if set |

**Duration formats** — all of these work:
```yaml
duration: 60       # bare number = minutes
duration: 60min    # minutes with "min"
duration: 1h       # hours
duration: 1h30m    # hours and minutes
duration: 90m      # minutes with "m"
```

**Example files** are in the `examples/` folder:

```bash
calctl import examples/product-launch-call.md --dry-run
calctl import examples/all-day-event.md --dry-run
```

---

## TUI

Launch the TUI with `calctl` (no arguments):

```
  calctl  ·  Sun, Jun 29 2026

  TODAY — Sun, Jun 29
  ────────────────────────────────────────
  09:00–10:00  Team Standup              [Work]
  14:00–15:00  Product Launch Call       [Work]

  Mon, Jun 30
  ────────────────────────────────────────
  (no events)

  Tue, Jul 01
  ────────────────────────────────────────
  All day      Team Offsite Berlin       [Work]

  ↑↓ navigate  enter detail  s sync  f free slots  +/- next 7 days  q quit
```

### Keyboard shortcuts

| Key | Action |
|-----|--------|
| `↑` / `k` | Navigate up |
| `↓` / `j` | Navigate down |
| `←` / `h` | Previous week |
| `→` / `l` | Next week |
| `Enter` | Open event detail |
| `s` | Sync Apple Calendar (current week) |
| `f` | Show free slots view |
| `+` or `]` | Show 7 more days |
| `-` or `[` | Show 7 fewer days |
| `Esc` / `Backspace` | Back to list |
| `q` | Quit |

The week navigation header (`◀ KW27 Mo Di Mi Do Fr Sa So ▶`) shows the current calendar week. Today is highlighted in blue, days with events in cyan.

### Event detail view

Press `Enter` on any event to see the full detail:

```
  Product Launch Call

  Date      Wed, Oct 15 2026
  Time      14:00 – 15:00  (1h)
  Calendar  Work
  Location  Zoom
  Attendees jan@example.com, lisa@example.com

  Discuss Q4 launch strategy and assign ownership.

  Agenda:
  - Launch date confirmation
  - Marketing plan review
  - Support readiness check
```

### Free slots view

Press `f` from the list to see free time slots:

```
  Free Slots

  Mon, Jun 30
    09:00 – 11:00  (2h)
    14:30 – 18:00  (3h30m)

  Tue, Jul 01
    09:00 – 18:00  (9h)
```

---

## Configuration

calctl reads its config from `~/.config/calctl/config.yaml`. The file is created automatically on first run with defaults.

```yaml
# ~/.config/calctl/config.yaml

# Calendar to use when none is specified in the Markdown frontmatter
default_calendar: ""

# Free slot search window (24h format)
working_hours_from: "09:00"
working_hours_to: "18:00"

# Working days (used for free slot finder)
working_days: [Mon, Tue, Wed, Thu, Fri]

# Minimum free slot duration in minutes
min_free_slot_min: 30

# Calendars to skip during sync (birthday/holiday calendars are slow and rarely useful)
excluded_calendars:
  - Geburtstage
  - Birthdays
  - Feiertage in Österreich
  - Holidays
  - Siri Suggestions

# Google Calendar (v0.5 — coming soon)
google:
  client_id: ""
  client_secret: ""
```

**Where things are stored:**

| Path | Content |
|------|---------|
| `~/.config/calctl/config.yaml` | Configuration |
| `~/.config/calctl/calctl.db` | Local SQLite event cache |
| `~/.config/calctl/google_token.json` | Google OAuth token (v0.5) |

---

## AI Integration

calctl is designed to work seamlessly with Claude and other AI agents.

### Shell pipeline (no MCP needed)

```bash
# Give Claude your week overview
calctl list --week --format json | claude "Summarize my week and flag any conflicts"

# Let Claude find the best slot for a meeting
calctl free --next 7 --format json | claude "Find the best 2h morning slot for a deep work session"

# AI-planned events → import
# Claude writes the Markdown, calctl creates the events
calctl import ~/ai-generated-events/ --dry-run
calctl import ~/ai-generated-events/
```

### Claude system prompt snippet

Add this to your Claude system prompt to let it use calctl directly:

```
You have access to calctl, a calendar CLI tool on this machine.

Read commands (always safe to run):
  calctl list --today --format json      → today's events
  calctl list --week --format json       → this week's events
  calctl free --next 7 --format json     → free time slots
  calctl list --from DATE --to DATE --format json

Write commands (confirm with user first):
  calctl import <file.md>               → create event from Markdown
  calctl sync                           → sync Apple Calendar

Event Markdown format:
  ---
  title: <title>
  date: YYYY-MM-DD
  time: HH:MM
  duration: 60min
  calendar: Work
  location: <optional>
  attendees: [email1, email2]
  ---
  Optional notes here.
```

### MCP server — Claude Code integration

`calctl mcp` starts a local MCP server so Claude can call your calendar directly without copy-pasting JSON.

#### 1. Install the binary

```bash
# Build and copy to your user bin
go build -o calctl . && cp calctl ~/.local/bin/calctl
# or if you have write access to /usr/local/bin:
sudo cp calctl /usr/local/bin/calctl
```

#### 2. Register in Claude Code

Add to `~/.claude.json` (create if it doesn't exist):

```json
{
  "mcpServers": {
    "calctl": {
      "command": "/Users/YOU/.local/bin/calctl",
      "args": ["mcp"]
    }
  }
}
```

Replace `/Users/YOU` with your actual home directory (`echo $HOME`).

Then **restart Claude Code**. You'll see calctl appear in the MCP tools list.

#### 3. What Claude can now do

| MCP Tool | What Claude calls |
|----------|------------------|
| `today` | "What's on today?" |
| `this_week` | "Give me my week overview" |
| `list_events(from, to)` | "What do I have from Oct 1–15?" |
| `sync(days?)` | "Sync my calendar" |
| `find_free_slots(from, to)` | "When am I free this week for a 1h meeting?" |
| `create_event(title, start, end, ...)` | "Book team lunch Thursday 12:00–13:00" |

#### Example prompts

```
"Was habe ich nächste Woche?"
"Wann bin ich Mittwoch frei für ein 2h Deep-Work-Block?"
"Erstell einen Termin: Zahnarzt, Montag 10:00–11:00, Kalender Privat"
"Sync meinen Kalender und gib mir eine Übersicht der nächsten 3 Tage"
```

> **First sync**: Claude will call `sync` automatically when the cache is empty.  
> macOS will ask for Calendar permission once — click **Allow**.

---

## Troubleshooting

### "permission denied" running setup.sh

```bash
chmod +x setup.sh
./setup.sh
```

### Apple Calendar permission denied

macOS requires explicit permission for apps to access Calendar.

1. Go to **System Settings → Privacy & Security → Calendars**
2. Enable **calctl** (or **Terminal**, depending on how you run it)

Or trigger the permission dialog by running:
```bash
calctl sync
```
macOS will prompt automatically.

### "no events" after sync

1. Check that calctl has Calendar access (see above)
2. Run `calctl sync --days 60` to sync more days ahead
3. Verify your calendar has events: open Apple Calendar and check

### Events not appearing in Apple Calendar after import

1. Run with `--dry-run` first to validate:
   ```bash
   calctl import event.md --dry-run
   ```
2. Check the calendar name matches exactly:
   ```bash
   osascript -e 'tell application "Calendar" to get name of every calendar'
   ```
3. Make sure the `calendar:` field in your Markdown matches one of the names above

### Build fails: "go: command not found"

```bash
brew install go
```

---

## Data & Privacy

- All data is stored locally in `~/.config/calctl/`
- calctl never makes network requests except when you explicitly add Google Calendar
- Apple Calendar is accessed read-only for `sync` and read/write for `import`
- No telemetry, no analytics, no external services

---

## Roadmap

| Version | Features | Status |
|---------|----------|--------|
| v0.1 | Apple Calendar sync, list, free slots, import, TUI | ✅ done |
| v0.2 | MCP server (`calctl mcp`), fast EventKit sync via Swift, week navigation | ✅ done |
| v0.5 | Google Calendar OAuth2, `calctl calendars` command | planned |
| v1.0 | Recurring events, attendee invites, timezone config | planned |

See [missionctl ROADMAP](../ROADMAP.md) for full timeline.

---

## Part of missionctl

calctl is part of the [missionctl](../README.md) bundle — a suite of small, local-first CLI tools that give AI agents hands:

| Tool | What it does |
|------|-------------|
| **calctl** | Calendar |
| postctl | Social media scheduling |
| mailctl | Email |
| budgetctl | Budget tracking |
| notectl | Notes (Obsidian) |
| taskctl | Tasks (Apple Reminders) |

Available on [polar.sh](https://polar.sh) — one-time purchase, no subscription.
