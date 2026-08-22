package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Session struct {
	ID          int64
	ProjectID   sql.NullInt64
	ProjectName sql.NullString
	StartedAt   time.Time
	EndedAt     sql.NullTime
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type SessionUpdate struct {
	StartedAt    *time.Time
	EndedAt      *time.Time
	ProjectID    *int64
	ClearProject bool
}

var ErrAlreadyRunning = errors.New("a work session is already running")

var ErrNoRunningSession = errors.New("no work session is running")

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
	return s.endRunningSession(ctx, endedAt, note, 0)
}

func (s *Store) EndRunningSessionWithOvertime(ctx context.Context, endedAt time.Time, note string, overtime time.Duration) (Session, error) {
	if durationMinutes(overtime) <= 0 {
		return Session{}, fmt.Errorf("overtime usage must be positive")
	}
	return s.endRunningSession(ctx, endedAt, note, overtime)
}

func (s *Store) endRunningSession(ctx context.Context, endedAt time.Time, note string, overtime time.Duration) (Session, error) {
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
	if overtime > 0 && !running.ProjectID.Valid {
		return Session{}, fmt.Errorf("overtime usage requires a project")
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
	if overtime > 0 {
		_, err = tx.ExecContext(ctx, `
INSERT INTO project_overtime_usages (project_id, session_id, used_on, minutes, created_at)
VALUES (?, ?, ?, ?, ?)
`, running.ProjectID.Int64, running.ID, endedAt.Local().Format("2006-01-02"), durationMinutes(overtime), formatTime(time.Now()))
		if err != nil {
			return Session{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Session{}, err
	}
	return s.SessionByID(ctx, running.ID)
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

func (s *Store) LastEndedSession(ctx context.Context) (*Session, error) {
	var session Session
	err := s.db.QueryRowContext(ctx, `
SELECT s.id, s.project_id, p.name, s.started_at, s.ended_at, s.created_at, s.updated_at
FROM sessions s
LEFT JOIN projects p ON p.id = s.project_id
WHERE s.ended_at IS NOT NULL
ORDER BY s.ended_at DESC, s.id DESC
LIMIT 1
`).Scan(
		&session.ID,
		&session.ProjectID,
		&session.ProjectName,
		parseScanner(&session.StartedAt),
		nullTimeScanner(&session.EndedAt),
		parseScanner(&session.CreatedAt),
		parseScanner(&session.UpdatedAt),
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
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
