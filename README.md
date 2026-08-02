# calctl

A terminal calendar client for Apple Calendar. Sync, browse, and manage events from the command line or a full TUI. Ships with an MCP server for AI assistant integration.

Part of the [missionctl](../README.md) suite.

---

## Quick Start

1. Clone and install:
   ```bash
   git clone https://github.com/aeon022/calctl && cd calctl
   ./setup.sh
   ```

2. Sync events from Apple Calendar (macOS will ask for permission once):
   ```bash
   calctl sync
   ```

3. Open the TUI:
   ```bash
   calctl
   ```

4. Or list today's events directly:
   ```bash
   calctl list --today
   ```

5. To expose calctl as an MCP tool for Claude Desktop, add the config shown in [MCP — AI Integration](#mcp--ai-integration) and restart Claude.

---

## Cheatsheet

| Command | What it does |
|---|---|
| `calctl` | Open TUI |
| `calctl sync` | Sync from Apple Calendar |
| `calctl list --today` | List today's events |
| `calctl list --week` | List this week's events |
| `calctl add "Meeting" --start 2026-07-07T10:00:00 --end 2026-07-07T11:00:00` | Create an event |
| `calctl free --today --duration 30` | Find 30-minute free slots today |
| `calctl import event.md` | Create event from Markdown file |
| `calctl calendars` | List all calendar names |
| `calctl mcp` | Start MCP server |

---

## CLI Reference

### `calctl`

Open the TUI. This is the default command; no subcommand is required.

```bash
calctl
```

---

### `calctl sync`

Sync events from Apple Calendar into the local SQLite cache.

```bash
calctl sync [--days N]
```

| Flag | Default | Description |
|---|---|---|
| `--days N` | 30 | Number of days ahead to sync |

Examples:

```bash
calctl sync
calctl sync --days 90
```

---

### `calctl list`

List events from the local cache.

```bash
calctl list [--from DATE] [--to DATE] [--today] [--week] [--calendar NAME] [--format human|json]
```

| Flag | Description |
|---|---|
| `--from DATE` | Start of range (`YYYY-MM-DD`) |
| `--to DATE` | End of range (`YYYY-MM-DD`) |
| `--today` | Shorthand for today only |
| `--week` | Shorthand for the current Monday–Sunday week |
| `--calendar NAME` | Filter by calendar name |
| `--format human\|json` | Output format (default: `human`) |

Examples:

```bash
calctl list --today
calctl list --week --calendar Work
calctl list --from 2026-07-01 --to 2026-07-31
calctl list --week --format json
```

---

### `calctl add`

Create a new event in Apple Calendar.

```bash
calctl add TITLE --start DATETIME --end DATETIME [--calendar NAME] [--notes TEXT] [--format human|json]
```

| Flag | Description |
|---|---|
| `--start DATETIME` | Event start (`YYYY-MM-DDTHH:MM:SS`) |
| `--end DATETIME` | Event end (`YYYY-MM-DDTHH:MM:SS`) |
| `--calendar NAME` | Target calendar (defaults to default calendar) |
| `--notes TEXT` | Notes or description |
| `--format human\|json` | Output format (default: `human`) |

Examples:

```bash
calctl add "Team standup" --start 2026-07-07T09:00:00 --end 2026-07-07T09:30:00
calctl add "Planning" --start 2026-07-07T14:00:00 --end 2026-07-07T15:00:00 --calendar Work --notes "Q3 goals"
calctl add "1:1" --start 2026-07-08T10:00:00 --end 2026-07-08T10:30:00 --format json
```

---

### `calctl free`

Find free time slots within a date range.

```bash
calctl free [--from DATE] [--to DATE] [--hours-from H] [--hours-to H] [--duration N] [--format human|json]
```

| Flag | Default | Description |
|---|---|---|
| `--from DATE` | Today | Start of search range (`YYYY-MM-DD`) |
| `--to DATE` | Today | End of search range (`YYYY-MM-DD`) |
| `--hours-from H` | 9 | Start of working hours |
| `--hours-to H` | 18 | End of working hours |
| `--duration N` | 30 | Minimum slot duration in minutes |
| `--format human\|json` | Output format (default: `human`) |

Examples:

```bash
calctl free --today --duration 60
calctl free --from 2026-07-07 --to 2026-07-11 --duration 30 --hours-from 8 --hours-to 17
calctl free --week --duration 90 --format json
```

---

### `calctl import`

Create a new event from a Markdown file. See [Event Markdown Format](#event-markdown-format) for the required structure.

```bash
calctl import FILE.md
```

Example:

```bash
calctl import standup.md
```

---

### `calctl calendars`

List all Apple Calendar names visible to calctl.

```bash
calctl calendars [--format human|json]
```

Examples:

```bash
calctl calendars
calctl calendars --format json
```

---

### `calctl mcp`

Start the MCP server on stdio. Used by Claude Desktop and other MCP-compatible clients.

```bash
calctl mcp
```

---

## Event Markdown Format

Events can be written as Markdown files and created with `calctl import`. The frontmatter block is required; the body is optional and treated as additional description.

```markdown
---
title: Team standup
start: 2026-07-07T09:00:00
end: 2026-07-07T09:30:00
calendar: Work
notes: Weekly sync
---

Optional description body here.
```

| Field | Required | Description |
|---|---|---|
| `title` | Yes | Event title |
| `start` | Yes | Start datetime (`YYYY-MM-DDTHH:MM:SS`, local time) |
| `end` | Yes | End datetime (`YYYY-MM-DDTHH:MM:SS`, local time) |
| `calendar` | No | Target calendar name |
| `notes` | No | Short notes line |

Datetime format: `YYYY-MM-DDTHH:MM:SS` (local time, no timezone suffix).

---

## TUI Keys

Open the TUI with `calctl` (no arguments).

### List View

| Key | Action |
|---|---|
| `j` / `k` or `↓` / `↑` | Navigate events (skips date headers) |
| `PgDn` / `PgUp` | Page down / page up |
| `Enter` | Open event detail |
| `n` | New event (opens create form) |
| `e` | Edit selected event |
| `d` | Delete selected event |
| `s` | Sync from Apple Calendar |
| `f` | Open free slot finder |
| `+` or `]` | Next week |
| `-` or `[` | Previous week |
| `←` / `→` or `h` / `l` | Week navigation |
| `q` | Quit |

### Detail View

| Key | Action |
|---|---|
| `Esc` | Back to list |

### Create / Edit Form

| Key | Action |
|---|---|
| `Tab` / `Shift+Tab` or `↓` / `↑` | Move between fields |
| `Ctrl+S` | Save event |
| `Esc` | Cancel |
| `y` | Confirm delete |

### Free Slot View

| Key | Action |
|---|---|
| `Esc` | Back to list |

---

## MCP — AI Integration

calctl ships an MCP server that exposes calendar operations to any MCP-compatible AI client, including Claude Desktop.

### Claude Desktop Configuration

Add the following to `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "calctl": {
      "command": "calctl",
      "args": ["mcp"]
    }
  }
}
```

Restart Claude Desktop after saving. `calctl` must be on your `PATH`.

### MCP Tools

| Tool | Parameters | Description |
|---|---|---|
| `list_events` | `from`, `to` | List events between two dates |
| `today` | — | List today's events |
| `this_week` | — | List this week's events (Monday–Sunday) |
| `sync` | `days` | Sync N days ahead from Apple Calendar |
| `find_free_slots` | `from`, `to`, `min_duration_minutes`, `work_hours_from`, `work_hours_to` | Find free time slots in a range |
| `create_event` | `title`, `start`, `end`, `calendar`, `notes` | Create a new calendar event |
| `delete_event` | `title`, `date` | Delete an event by title and date |

### AI Workflow Examples

**"What is on my calendar this week?"**

Prompt Claude: *"What's on my calendar this week?"*

Claude calls `this_week` and returns a structured list of all events for the current Monday–Sunday range.

---

**"Find a 30-minute slot for a meeting."**

Prompt Claude: *"Find a free 30-minute slot for a meeting sometime this week, between 9am and 5pm."*

Claude calls `find_free_slots` with `from` and `to` set to the current week, `min_duration_minutes: 30`, `work_hours_from: 9`, `work_hours_to: 17`, and returns available windows.

---

**"Create an event from a natural language description."**

Prompt Claude: *"Schedule a product review on Thursday July 10th from 2pm to 3pm on my Work calendar."*

Claude calls `create_event` with the extracted title, start, end, and calendar values. The event is created directly in Apple Calendar.

---

## Architecture

```
Apple Calendar (AppleScript)
        |
     calctl sync
        |
  SQLite cache (~/.config/calctl/calctl.db)
        |
   +----+----+
   |         |
  TUI      MCP server (stdio)
             |
         Claude Desktop / any MCP client
```

Events are read from Apple Calendar via AppleScript and cached locally in SQLite. The TUI and MCP server both read from this cache. Write operations (`add`, `delete`) go through AppleScript directly to Apple Calendar, then a sync updates the cache.

---

## Syncing across devices

By default calctl's cache lives at `~/Library/Application Support/calctl/calctl.db`, local to this machine. To share it across devices, set `data_dir` (in `~/Library/Application Support/calctl/config.yaml`) or the `CALCTL_DATA_DIR` env var to a folder you already sync yourself — iCloud Drive, Dropbox, Syncthing, etc:

```bash
export CALCTL_DATA_DIR="$HOME/Library/Mobile Documents/com~apple~CloudDocs/calctl"
```

Once set, calctl automatically switches its SQLite journal mode from WAL to rollback-journal — WAL splits the database across multiple files that a folder-sync client can't update atomically together, so this switch keeps the directory down to a single consistent file whenever calctl isn't actively writing. A same-machine lock also prevents two calctl processes from opening the cache at once (run `calctl doctor` to see the current mode and path). This only protects against the same-machine and stale-snapshot failure modes, not two machines editing at the exact same instant; an undownloaded iCloud file is reported explicitly rather than as a bare error.

---

## Requirements

- macOS with Apple Calendar
- Go 1.21+

---

## License

See [LICENSE](./LICENSE).
