package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aeon022/calctl/internal/models"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_journal=WAL&_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id          TEXT PRIMARY KEY,
			title       TEXT NOT NULL,
			start_time  TEXT NOT NULL,
			end_time    TEXT NOT NULL,
			all_day     INTEGER NOT NULL DEFAULT 0,
			calendar    TEXT NOT NULL DEFAULT '',
			location    TEXT NOT NULL DEFAULT '',
			notes       TEXT NOT NULL DEFAULT '',
			attendees   TEXT NOT NULL DEFAULT '[]',
			source      TEXT NOT NULL DEFAULT 'apple',
			external_id TEXT NOT NULL DEFAULT '',
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_events_start ON events(start_time);
		CREATE INDEX IF NOT EXISTS idx_events_source ON events(source);
	`)
	return err
}

func (s *Store) UpsertEvent(ctx context.Context, e *models.Event) error {
	attendees, _ := json.Marshal(e.Attendees)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO events
			(id, title, start_time, end_time, all_day, calendar, location, notes, attendees, source, external_id, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			title=excluded.title, start_time=excluded.start_time, end_time=excluded.end_time,
			all_day=excluded.all_day, calendar=excluded.calendar, location=excluded.location,
			notes=excluded.notes, attendees=excluded.attendees, updated_at=excluded.updated_at
	`,
		e.ID, e.Title,
		e.StartTime.UTC().Format(time.RFC3339),
		e.EndTime.UTC().Format(time.RFC3339),
		boolToInt(e.AllDay),
		e.Calendar, e.Location, e.Notes,
		string(attendees),
		e.Source, e.ExternalID,
		e.CreatedAt.UTC().Format(time.RFC3339),
		e.UpdatedAt.UTC().Format(time.RFC3339),
	)
	return err
}

func (s *Store) ListEvents(ctx context.Context, from, to time.Time) ([]models.Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, start_time, end_time, all_day, calendar, location, notes, attendees, source, external_id, created_at, updated_at
		FROM events
		WHERE start_time >= ? AND start_time <= ?
		ORDER BY start_time ASC
	`, from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

func (s *Store) DeleteByID(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE id = ?`, id)
	return err
}

func (s *Store) DeleteBySource(ctx context.Context, source string, from, to time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM events WHERE source = ? AND start_time >= ? AND start_time <= ?
	`, source, from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339))
	return err
}

func scanEvents(rows *sql.Rows) ([]models.Event, error) {
	var events []models.Event
	for rows.Next() {
		var e models.Event
		var startStr, endStr, createdStr, updatedStr string
		var allDayInt int
		var attendeesJSON string

		if err := rows.Scan(
			&e.ID, &e.Title, &startStr, &endStr, &allDayInt,
			&e.Calendar, &e.Location, &e.Notes, &attendeesJSON,
			&e.Source, &e.ExternalID, &createdStr, &updatedStr,
		); err != nil {
			return nil, err
		}

		e.StartTime = parseLocalTime(startStr)
		e.EndTime = parseLocalTime(endStr)
		e.CreatedAt = parseLocalTime(createdStr)
		e.UpdatedAt = parseLocalTime(updatedStr)
		e.AllDay = allDayInt == 1
		_ = json.Unmarshal([]byte(attendeesJSON), &e.Attendees)

		events = append(events, e)
	}
	return events, rows.Err()
}

// parseLocalTime parses an RFC3339 timestamp and converts to local timezone.
func parseLocalTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t.Local()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
