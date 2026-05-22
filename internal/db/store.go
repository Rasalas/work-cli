package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type Project struct {
	ID        int64
	Name      string
	CreatedAt time.Time
	Archived  bool
}

type Session struct {
	ID          int64
	ProjectID   sql.NullInt64
	ProjectName sql.NullString
	StartedAt   time.Time
	EndedAt     sql.NullTime
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Note struct {
	ID        int64
	SessionID int64
	Kind      string
	Body      string
	CreatedAt time.Time
}

type SessionUpdate struct {
	StartedAt    *time.Time
	EndedAt      *time.Time
	ProjectID    *int64
	ClearProject bool
}

var ErrAlreadyRunning = errors.New("a work session is already running")
var ErrNoRunningSession = errors.New("no work session is running")

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
		return nil, err
	}
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	store := &Store{db: conn}
	if err := store.migrate(context.Background()); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
PRAGMA foreign_keys = ON;

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

CREATE INDEX IF NOT EXISTS idx_sessions_open ON sessions(ended_at) WHERE ended_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_sessions_started_at ON sessions(started_at);
CREATE INDEX IF NOT EXISTS idx_notes_session_id ON notes(session_id);
`)
	return err
}

func (s *Store) AddProject(ctx context.Context, name string) (Project, error) {
	now := time.Now()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO projects (name, created_at, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(name) DO UPDATE SET archived_at = NULL, updated_at = excluded.updated_at
`, name, formatTime(now), formatTime(now))
	if err != nil {
		return Project{}, err
	}
	return s.ProjectByName(ctx, name)
}

func (s *Store) ProjectByName(ctx context.Context, name string) (Project, error) {
	var project Project
	var archivedAt sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT id, name, created_at, archived_at FROM projects WHERE name = ?
`, name).Scan(&project.ID, &project.Name, parseScanner(&project.CreatedAt), &archivedAt)
	if err != nil {
		return Project{}, err
	}
	project.Archived = archivedAt.Valid
	return project, nil
}

func (s *Store) ActiveProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, created_at, archived_at
FROM projects
WHERE archived_at IS NULL
ORDER BY lower(name)
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var project Project
		var archivedAt sql.NullString
		if err := rows.Scan(&project.ID, &project.Name, parseScanner(&project.CreatedAt), &archivedAt); err != nil {
			return nil, err
		}
		project.Archived = archivedAt.Valid
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

func (s *Store) StartSession(ctx context.Context, startedAt time.Time, projectID *int64) (Session, error) {
	running, err := s.RunningSession(ctx)
	if err != nil {
		return Session{}, err
	}
	if running != nil {
		return Session{}, ErrAlreadyRunning
	}

	now := time.Now()
	var result sql.Result
	if projectID == nil {
		result, err = s.db.ExecContext(ctx, `
INSERT INTO sessions (started_at, created_at, updated_at)
VALUES (?, ?, ?)
`, formatTime(startedAt), formatTime(now), formatTime(now))
	} else {
		result, err = s.db.ExecContext(ctx, `
INSERT INTO sessions (project_id, started_at, created_at, updated_at)
VALUES (?, ?, ?, ?)
`, *projectID, formatTime(startedAt), formatTime(now), formatTime(now))
	}
	if err != nil {
		return Session{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Session{}, err
	}
	return s.SessionByID(ctx, id)
}

func (s *Store) EndRunningSession(ctx context.Context, endedAt time.Time, note string) (Session, error) {
	running, err := s.RunningSession(ctx)
	if err != nil {
		return Session{}, err
	}
	if running == nil {
		return Session{}, ErrNoRunningSession
	}
	if endedAt.Before(running.StartedAt) {
		return Session{}, fmt.Errorf("end time cannot be before start time")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
UPDATE sessions SET ended_at = ?, updated_at = ? WHERE id = ?
`, formatTime(endedAt), formatTime(time.Now()), running.ID)
	if err != nil {
		return Session{}, err
	}
	if note != "" {
		_, err = tx.ExecContext(ctx, `
INSERT INTO notes (session_id, kind, body, created_at)
VALUES (?, 'done', ?, ?)
`, running.ID, note, formatTime(endedAt))
		if err != nil {
			return Session{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Session{}, err
	}
	return s.SessionByID(ctx, running.ID)
}

func (s *Store) AddNote(ctx context.Context, kind, body string, createdAt time.Time) (Note, error) {
	running, err := s.RunningSession(ctx)
	if err != nil {
		return Note{}, err
	}
	if running == nil {
		return Note{}, ErrNoRunningSession
	}
	result, err := s.db.ExecContext(ctx, `
INSERT INTO notes (session_id, kind, body, created_at)
VALUES (?, ?, ?, ?)
`, running.ID, kind, body, formatTime(createdAt))
	if err != nil {
		return Note{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Note{}, err
	}
	return Note{ID: id, SessionID: running.ID, Kind: kind, Body: body, CreatedAt: createdAt}, nil
}

func (s *Store) RunningSession(ctx context.Context) (*Session, error) {
	rows, err := s.sessions(ctx, "WHERE s.ended_at IS NULL", nil)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func (s *Store) LastSession(ctx context.Context) (*Session, error) {
	rows, err := s.sessions(ctx, "", []any{})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func (s *Store) SessionByID(ctx context.Context, id int64) (Session, error) {
	rows, err := s.sessions(ctx, "WHERE s.id = ?", []any{id})
	if err != nil {
		return Session{}, err
	}
	if len(rows) == 0 {
		return Session{}, sql.ErrNoRows
	}
	return rows[0], nil
}

func (s *Store) UpdateSession(ctx context.Context, id int64, update SessionUpdate) (Session, error) {
	session, err := s.SessionByID(ctx, id)
	if err != nil {
		return Session{}, err
	}

	startedAt := session.StartedAt
	if update.StartedAt != nil {
		startedAt = *update.StartedAt
	}
	endedAt := session.EndedAt
	if update.EndedAt != nil {
		endedAt = sql.NullTime{Time: *update.EndedAt, Valid: true}
	}
	if endedAt.Valid && endedAt.Time.Before(startedAt) {
		return Session{}, fmt.Errorf("end time cannot be before start time")
	}

	projectID := session.ProjectID
	if update.ClearProject {
		projectID = sql.NullInt64{}
	}
	if update.ProjectID != nil {
		projectID = sql.NullInt64{Int64: *update.ProjectID, Valid: true}
	}

	var projectValue any
	if projectID.Valid {
		projectValue = projectID.Int64
	}
	var endValue any
	if endedAt.Valid {
		endValue = formatTime(endedAt.Time)
	}

	_, err = s.db.ExecContext(ctx, `
UPDATE sessions
SET project_id = ?, started_at = ?, ended_at = ?, updated_at = ?
WHERE id = ?
`, projectValue, formatTime(startedAt), endValue, formatTime(time.Now()), id)
	if err != nil {
		return Session{}, err
	}
	return s.SessionByID(ctx, id)
}

func (s *Store) DeleteSession(ctx context.Context, id int64) (Session, error) {
	session, err := s.SessionByID(ctx, id)
	if err != nil {
		return Session{}, err
	}

	_, err = s.db.ExecContext(ctx, `
DELETE FROM sessions WHERE id = ?
`, id)
	if err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *Store) LogSessions(ctx context.Context, from, to *time.Time, project string) ([]Session, error) {
	where := "WHERE 1=1"
	var args []any
	if from != nil {
		where += " AND s.started_at >= ?"
		args = append(args, formatTime(*from))
	}
	if to != nil {
		where += " AND s.started_at < ?"
		args = append(args, formatTime(*to))
	}
	if project != "" {
		where += " AND p.name = ?"
		args = append(args, project)
	}
	return s.sessions(ctx, where, args)
}

func (s *Store) NotesForSession(ctx context.Context, sessionID int64) ([]Note, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, kind, body, created_at
FROM notes
WHERE session_id = ?
ORDER BY created_at ASC, id ASC
`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []Note
	for rows.Next() {
		var note Note
		if err := rows.Scan(&note.ID, &note.SessionID, &note.Kind, &note.Body, parseScanner(&note.CreatedAt)); err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}
	return notes, rows.Err()
}

func (s *Store) sessions(ctx context.Context, where string, args []any) ([]Session, error) {
	query := `
SELECT s.id, s.project_id, p.name, s.started_at, s.ended_at, s.created_at, s.updated_at
FROM sessions s
LEFT JOIN projects p ON p.id = s.project_id
` + where + `
ORDER BY s.started_at DESC, s.id DESC
`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var session Session
		if err := rows.Scan(
			&session.ID,
			&session.ProjectID,
			&session.ProjectName,
			parseScanner(&session.StartedAt),
			nullTimeScanner(&session.EndedAt),
			parseScanner(&session.CreatedAt),
			parseScanner(&session.UpdatedAt),
		); err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func formatTime(t time.Time) string {
	return t.Format(time.RFC3339)
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
