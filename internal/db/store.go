package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db   *sql.DB
	path string
}

func DefaultPath() (string, error) {
	if path := os.Getenv("WORK_DB"); path != "" {
		return path, nil
	}
	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
		return filepath.Join(dataHome, "work-cli", "work.sqlite"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "work-cli", "work.sqlite"), nil
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory %s: %w", filepath.Dir(path), err)
	}
	dsn := (&url.URL{
		Scheme:   "file",
		Path:     path,
		RawQuery: "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)",
	}).String()
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}
	conn.SetMaxOpenConns(1)
	store := &Store{db: conn, path: path}
	if err := store.migrate(context.Background()); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// Backup writes a consistent snapshot of the database to target using
// SQLite's VACUUM INTO. The target must not exist yet.

func (s *Store) Backup(ctx context.Context, target string) error {
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("backup target already exists: %s", target)
	}
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, target); err != nil {
		return fmt.Errorf("backup to %s: %w", target, err)
	}
	return nil
}

var ErrNoMigrationBackup = errors.New("database file does not exist yet; nothing to back up")

// backupBeforeMigration copies the current database contents to a timestamped
// snapshot next to the database file. It is called before schema migrations
// are applied so that a failed migration never loses data.

func (s *Store) backupBeforeMigration(ctx context.Context, at time.Time) (string, error) {
	target := fmt.Sprintf("%s.pre-migration-%s.bak", s.path, at.Format("20060102-150405"))
	if _, err := os.Stat(target); err == nil {
		return "", fmt.Errorf("backup target already exists: %s", target)
	}
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, target); err != nil {
		return "", fmt.Errorf("pre-migration backup to %s: %w", target, err)
	}
	return target, nil
}

// migrations holds the ordered schema migrations. Entry i upgrades the
// database from version i to version i+1 (tracked via PRAGMA user_version).
// Every statement must be idempotent enough for databases that predate
// version tracking (user_version == 0 with an existing v1 schema).

var migrations = []string{
	// v1: initial schema.
	`
CREATE TABLE IF NOT EXISTS projects (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	archived_at TEXT
);

CREATE TABLE IF NOT EXISTS sessions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER REFERENCES projects(id),
	started_at TEXT NOT NULL,
	ended_at TEXT,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS notes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id INTEGER NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
	kind TEXT NOT NULL,
	body TEXT NOT NULL,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS project_schedules (
	project_id INTEGER PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
	weekly_target_minutes INTEGER NOT NULL,
	workdays TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS project_balance_adjustments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	adjustment_date TEXT NOT NULL,
	minutes INTEGER NOT NULL,
	note TEXT NOT NULL,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS project_export_settings (
	project_id INTEGER PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
	report_start TEXT NOT NULL,
	report_end TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS project_overtime_usages (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	session_id INTEGER NOT NULL UNIQUE REFERENCES sessions(id) ON DELETE CASCADE,
	used_on TEXT NOT NULL,
	minutes INTEGER NOT NULL CHECK (minutes > 0),
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS project_absences (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	kind TEXT NOT NULL,
	starts_on TEXT NOT NULL,
	ends_on TEXT NOT NULL,
	created_at TEXT NOT NULL,
	CHECK (ends_on >= starts_on)
);

CREATE INDEX IF NOT EXISTS idx_sessions_open ON sessions(ended_at) WHERE ended_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_sessions_started_at ON sessions(started_at);
CREATE INDEX IF NOT EXISTS idx_notes_session_id ON notes(session_id);
CREATE INDEX IF NOT EXISTS idx_project_balance_adjustments_project_id ON project_balance_adjustments(project_id);
CREATE INDEX IF NOT EXISTS idx_project_overtime_usages_project_date ON project_overtime_usages(project_id, used_on);
CREATE INDEX IF NOT EXISTS idx_project_absences_project_dates ON project_absences(project_id, starts_on, ends_on);
`,
}

func (s *Store) migrate(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	if version < len(migrations) {
		if info, err := os.Stat(s.path); err == nil && info.Size() > 0 {
			if _, err := s.backupBeforeMigration(ctx, time.Now()); err != nil {
				return err
			}
		}
	}
	for i, stmt := range migrations {
		v := i + 1
		if v <= version {
			continue
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d: %w", v, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, v)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d: set user_version: %w", v, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migration %d: commit: %w", v, err)
		}
	}
	return nil
}

func formatTime(t time.Time) string {
	return t.Format(time.RFC3339)
}

func durationMinutes(duration time.Duration) int64 {
	return int64(duration.Round(time.Minute) / time.Minute)
}

func endOfLocalDay(t time.Time) *time.Time {
	local := t.Local()
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	end := start.AddDate(0, 0, 1)
	return &end
}

func scheduledWeekdays(workdays string) map[time.Weekday]bool {
	days := make(map[time.Weekday]bool)
	for _, value := range strings.Split(workdays, ",") {
		switch strings.TrimSpace(strings.ToLower(value)) {
		case "mon":
			days[time.Monday] = true
		case "tue":
			days[time.Tuesday] = true
		case "wed":
			days[time.Wednesday] = true
		case "thu":
			days[time.Thursday] = true
		case "fri":
			days[time.Friday] = true
		case "sat":
			days[time.Saturday] = true
		case "sun":
			days[time.Sunday] = true
		}
	}
	return days
}

type timeScanner struct {
	dest *time.Time
}

func parseScanner(dest *time.Time) sql.Scanner {
	return timeScanner{dest: dest}
}

func (s timeScanner) Scan(value any) error {
	switch v := value.(type) {
	case string:
		parsed, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return err
		}
		*s.dest = parsed
		return nil
	case []byte:
		parsed, err := time.Parse(time.RFC3339, string(v))
		if err != nil {
			return err
		}
		*s.dest = parsed
		return nil
	default:
		return fmt.Errorf("unsupported time value %T", value)
	}
}

type nullableTimeScanner struct {
	dest *sql.NullTime
}

func nullTimeScanner(dest *sql.NullTime) sql.Scanner {
	return nullableTimeScanner{dest: dest}
}

func (s nullableTimeScanner) Scan(value any) error {
	if value == nil {
		*s.dest = sql.NullTime{}
		return nil
	}
	var parsed time.Time
	if err := (timeScanner{dest: &parsed}).Scan(value); err != nil {
		return err
	}
	*s.dest = sql.NullTime{Time: parsed, Valid: true}
	return nil
}
