package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

type ProjectSchedule struct {
	ProjectID     int64
	WeeklyTarget  time.Duration
	Workdays      string
	LastUpdatedAt time.Time
}

type ProjectExportSettings struct {
	ProjectID   int64
	ReportStart string
	ReportEnd   string
	UpdatedAt   time.Time
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

type OvertimeUsage struct {
	ID        int64
	ProjectID int64
	SessionID int64
	UsedOn    time.Time
	Duration  time.Duration
	CreatedAt time.Time
}

type ProjectAbsence struct {
	ID        int64
	ProjectID int64
	Kind      string
	StartsOn  time.Time
	EndsOn    time.Time
	CreatedAt time.Time
}

type SessionUpdate struct {
	StartedAt    *time.Time
	EndedAt      *time.Time
	ProjectID    *int64
	ClearProject bool
}

type NoteUpdate struct {
	Kind      *string
	Body      *string
	CreatedAt *time.Time
}

var ErrAlreadyRunning = errors.New("a work session is already running")
var ErrNoRunningSession = errors.New("no work session is running")
var ErrOverlappingAbsence = errors.New("an overlapping absence already exists")

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

func (s *Store) SetProjectSchedule(ctx context.Context, projectID int64, weeklyTarget time.Duration, workdays string) error {
	now := time.Now()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO project_schedules (project_id, weekly_target_minutes, workdays, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(project_id) DO UPDATE SET
	weekly_target_minutes = excluded.weekly_target_minutes,
	workdays = excluded.workdays,
	updated_at = excluded.updated_at
`, projectID, durationMinutes(weeklyTarget), workdays, formatTime(now))
	return err
}

func (s *Store) ProjectSchedule(ctx context.Context, projectID int64) (*ProjectSchedule, error) {
	var schedule ProjectSchedule
	var weeklyMinutes int64
	err := s.db.QueryRowContext(ctx, `
SELECT project_id, weekly_target_minutes, workdays, updated_at
FROM project_schedules
WHERE project_id = ?
`, projectID).Scan(
		&schedule.ProjectID,
		&weeklyMinutes,
		&schedule.Workdays,
		parseScanner(&schedule.LastUpdatedAt),
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	schedule.WeeklyTarget = time.Duration(weeklyMinutes) * time.Minute
	return &schedule, nil
}

func (s *Store) SetProjectExportSettings(ctx context.Context, projectID int64, reportStart, reportEnd string) error {
	now := time.Now()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO project_export_settings (project_id, report_start, report_end, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(project_id) DO UPDATE SET
	report_start = excluded.report_start,
	report_end = excluded.report_end,
	updated_at = excluded.updated_at
`, projectID, reportStart, reportEnd, formatTime(now))
	return err
}

func (s *Store) ProjectExportSettings(ctx context.Context, projectID int64) (*ProjectExportSettings, error) {
	var settings ProjectExportSettings
	err := s.db.QueryRowContext(ctx, `
SELECT project_id, report_start, report_end, updated_at
FROM project_export_settings
WHERE project_id = ?
`, projectID).Scan(
		&settings.ProjectID,
		&settings.ReportStart,
		&settings.ReportEnd,
		parseScanner(&settings.UpdatedAt),
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &settings, nil
}

func (s *Store) SetProjectBalance(ctx context.Context, projectID int64, date time.Time, balance time.Duration) error {
	current, _, err := s.rawProjectBalance(ctx, projectID)
	if err != nil {
		return err
	}
	delta := durationMinutes(balance - current)
	if delta == 0 {
		return nil
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO project_balance_adjustments (project_id, adjustment_date, minutes, note, created_at)
VALUES (?, ?, ?, 'set balance', ?)
`, projectID, date.Local().Format("2006-01-02"), delta, formatTime(time.Now()))
	return err
}

func (s *Store) ProjectBalance(ctx context.Context, projectID int64) (time.Duration, error) {
	return s.ProjectBalanceAt(ctx, projectID, time.Now())
}

func (s *Store) ProjectBalanceAt(ctx context.Context, projectID int64, at time.Time) (time.Duration, error) {
	balance, latestAdjustmentDate, err := s.rawProjectBalance(ctx, projectID)
	if err != nil {
		return 0, err
	}
	schedule, err := s.ProjectSchedule(ctx, projectID)
	if err != nil {
		return 0, err
	}
	start := balanceWeekStart(at)
	if schedule != nil {
		start = balanceWeekStart(schedule.LastUpdatedAt)
	}
	if latestAdjustmentDate.Valid {
		parsed, err := time.ParseInLocation("2006-01-02", latestAdjustmentDate.String, time.Local)
		if err != nil {
			return 0, err
		}
		start = balanceWeekStart(parsed)
	}
	used, err := s.ProjectOvertimeUsed(ctx, projectID, &start, endOfLocalDay(at))
	if err != nil {
		return 0, err
	}
	if schedule == nil {
		return balance - used, nil
	}
	completedBefore := balanceWeekStart(at)
	if !completedBefore.After(start) {
		return balance - used, nil
	}

	delta, err := s.completedWeeklyBalanceDelta(ctx, projectID, start, completedBefore, *schedule)
	if err != nil {
		return 0, err
	}
	return balance - used + delta, nil
}

func (s *Store) rawProjectBalance(ctx context.Context, projectID int64) (time.Duration, sql.NullString, error) {
	var minutes int64
	var latestAdjustmentDate sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(SUM(minutes), 0), MAX(adjustment_date)
FROM project_balance_adjustments
WHERE project_id = ?
`, projectID).Scan(&minutes, &latestAdjustmentDate)
	if err != nil {
		return 0, sql.NullString{}, err
	}
	return time.Duration(minutes) * time.Minute, latestAdjustmentDate, nil
}

func (s *Store) completedWeeklyBalanceDelta(ctx context.Context, projectID int64, start, completedBefore time.Time, schedule ProjectSchedule) (time.Duration, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT started_at, ended_at
FROM sessions
WHERE project_id = ?
  AND started_at >= ?
  AND started_at < ?
  AND ended_at IS NOT NULL
`, projectID, formatTime(start), formatTime(completedBefore))
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	workedByWeek := make(map[time.Time]time.Duration)
	for rows.Next() {
		var startedAt, endedAt time.Time
		if err := rows.Scan(parseScanner(&startedAt), parseScanner(&endedAt)); err != nil {
			return 0, err
		}
		if endedAt.After(startedAt) {
			week := balanceWeekStart(startedAt)
			workedByWeek[week] += endedAt.Sub(startedAt)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	usages, err := s.OvertimeUsages(ctx, projectID, &start, &completedBefore)
	if err != nil {
		return 0, err
	}
	usedByWeek := make(map[time.Time]time.Duration)
	for _, usage := range usages {
		week := balanceWeekStart(usage.UsedOn)
		usedByWeek[week] += usage.Duration
	}

	var delta time.Duration
	for week := start; week.Before(completedBefore); week = week.AddDate(0, 0, 7) {
		delta += workedByWeek[week] + usedByWeek[week] - schedule.WeeklyTarget
	}
	absenceReduction, err := s.ProjectAbsenceTargetReduction(ctx, projectID, start, completedBefore, schedule.WeeklyTarget, schedule.Workdays)
	if err != nil {
		return 0, err
	}
	return delta + absenceReduction, nil
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

func (s *Store) OvertimeUsages(ctx context.Context, projectID int64, from, to *time.Time) ([]OvertimeUsage, error) {
	where := "WHERE project_id = ?"
	args := []any{projectID}
	if from != nil {
		where += " AND used_on >= ?"
		args = append(args, from.Local().Format("2006-01-02"))
	}
	if to != nil {
		where += " AND used_on < ?"
		args = append(args, to.Local().Format("2006-01-02"))
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, project_id, session_id, used_on, minutes, created_at
FROM project_overtime_usages
`+where+`
ORDER BY used_on ASC, id ASC
`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var usages []OvertimeUsage
	for rows.Next() {
		var usage OvertimeUsage
		var usedOn string
		var minutes int64
		if err := rows.Scan(
			&usage.ID,
			&usage.ProjectID,
			&usage.SessionID,
			&usedOn,
			&minutes,
			parseScanner(&usage.CreatedAt),
		); err != nil {
			return nil, err
		}
		usage.UsedOn, err = time.ParseInLocation("2006-01-02", usedOn, time.Local)
		if err != nil {
			return nil, err
		}
		usage.Duration = time.Duration(minutes) * time.Minute
		usages = append(usages, usage)
	}
	return usages, rows.Err()
}

func (s *Store) ProjectOvertimeUsed(ctx context.Context, projectID int64, from, to *time.Time) (time.Duration, error) {
	usages, err := s.OvertimeUsages(ctx, projectID, from, to)
	if err != nil {
		return 0, err
	}
	var total time.Duration
	for _, usage := range usages {
		total += usage.Duration
	}
	return total, nil
}

func (s *Store) AddProjectAbsence(ctx context.Context, projectID int64, startsOn, endsOn time.Time, kind string) (ProjectAbsence, error) {
	startsOn = localDate(startsOn)
	endsOn = localDate(endsOn)
	if endsOn.Before(startsOn) {
		return ProjectAbsence{}, fmt.Errorf("absence end cannot be before start")
	}
	if kind == "" {
		return ProjectAbsence{}, fmt.Errorf("absence type is required")
	}

	var overlap int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM project_absences
WHERE project_id = ?
  AND starts_on <= ?
  AND ends_on >= ?
`, projectID, endsOn.Format("2006-01-02"), startsOn.Format("2006-01-02")).Scan(&overlap)
	if err != nil {
		return ProjectAbsence{}, err
	}
	if overlap > 0 {
		return ProjectAbsence{}, ErrOverlappingAbsence
	}

	now := time.Now()
	result, err := s.db.ExecContext(ctx, `
INSERT INTO project_absences (project_id, kind, starts_on, ends_on, created_at)
VALUES (?, ?, ?, ?, ?)
`, projectID, kind, startsOn.Format("2006-01-02"), endsOn.Format("2006-01-02"), formatTime(now))
	if err != nil {
		return ProjectAbsence{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return ProjectAbsence{}, err
	}
	return ProjectAbsence{
		ID:        id,
		ProjectID: projectID,
		Kind:      kind,
		StartsOn:  startsOn,
		EndsOn:    endsOn,
		CreatedAt: now,
	}, nil
}

func (s *Store) ProjectAbsences(ctx context.Context, projectID int64, from, to *time.Time) ([]ProjectAbsence, error) {
	where := "WHERE project_id = ?"
	args := []any{projectID}
	if from != nil {
		where += " AND ends_on >= ?"
		args = append(args, localDate(*from).Format("2006-01-02"))
	}
	if to != nil {
		where += " AND starts_on < ?"
		args = append(args, localDate(*to).Format("2006-01-02"))
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, project_id, kind, starts_on, ends_on, created_at
FROM project_absences
`+where+`
ORDER BY starts_on ASC, id ASC
`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var absences []ProjectAbsence
	for rows.Next() {
		var absence ProjectAbsence
		var startsOn, endsOn string
		if err := rows.Scan(
			&absence.ID,
			&absence.ProjectID,
			&absence.Kind,
			&startsOn,
			&endsOn,
			parseScanner(&absence.CreatedAt),
		); err != nil {
			return nil, err
		}
		absence.StartsOn, err = time.ParseInLocation("2006-01-02", startsOn, time.Local)
		if err != nil {
			return nil, err
		}
		absence.EndsOn, err = time.ParseInLocation("2006-01-02", endsOn, time.Local)
		if err != nil {
			return nil, err
		}
		absences = append(absences, absence)
	}
	return absences, rows.Err()
}

func (s *Store) ProjectAbsentOn(ctx context.Context, projectID int64, date time.Time) (bool, error) {
	start := localDate(date)
	end := start.AddDate(0, 0, 1)
	absences, err := s.ProjectAbsences(ctx, projectID, &start, &end)
	if err != nil {
		return false, err
	}
	return len(absences) > 0, nil
}

func (s *Store) ProjectAbsenceTargetReduction(ctx context.Context, projectID int64, from, to time.Time, weeklyTarget time.Duration, workdays string) (time.Duration, error) {
	from = localDate(from)
	to = localDate(to)
	if !to.After(from) {
		return 0, nil
	}
	scheduledDays := scheduledWeekdays(workdays)
	if len(scheduledDays) == 0 {
		return 0, nil
	}
	absences, err := s.ProjectAbsences(ctx, projectID, &from, &to)
	if err != nil {
		return 0, err
	}
	absentDates := make(map[string]bool)
	for _, absence := range absences {
		start := absence.StartsOn
		if start.Before(from) {
			start = from
		}
		end := absence.EndsOn.AddDate(0, 0, 1)
		if end.After(to) {
			end = to
		}
		for day := start; day.Before(end); day = day.AddDate(0, 0, 1) {
			if scheduledDays[day.Weekday()] {
				absentDates[day.Format("2006-01-02")] = true
			}
		}
	}
	dailyTarget := weeklyTarget / time.Duration(len(scheduledDays))
	return time.Duration(len(absentDates)) * dailyTarget, nil
}

func (s *Store) AddNote(ctx context.Context, kind, body string, createdAt time.Time) (Note, error) {
	running, err := s.RunningSession(ctx)
	if err != nil {
		return Note{}, err
	}
	if running == nil {
		return Note{}, ErrNoRunningSession
	}
	return s.insertNote(ctx, running.ID, kind, body, createdAt)
}

func (s *Store) AddNoteToSession(ctx context.Context, sessionID int64, kind, body string, createdAt time.Time) (Note, error) {
	if _, err := s.SessionByID(ctx, sessionID); err != nil {
		return Note{}, err
	}
	return s.insertNote(ctx, sessionID, kind, body, createdAt)
}

func (s *Store) insertNote(ctx context.Context, sessionID int64, kind, body string, createdAt time.Time) (Note, error) {
	result, err := s.db.ExecContext(ctx, `
INSERT INTO notes (session_id, kind, body, created_at)
VALUES (?, ?, ?, ?)
`, sessionID, kind, body, formatTime(createdAt))
	if err != nil {
		return Note{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Note{}, err
	}
	return Note{ID: id, SessionID: sessionID, Kind: kind, Body: body, CreatedAt: createdAt}, nil
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

func (s *Store) NoteByID(ctx context.Context, id int64) (Note, error) {
	var note Note
	err := s.db.QueryRowContext(ctx, `
SELECT id, session_id, kind, body, created_at
FROM notes
WHERE id = ?
`, id).Scan(&note.ID, &note.SessionID, &note.Kind, &note.Body, parseScanner(&note.CreatedAt))
	if err != nil {
		return Note{}, err
	}
	return note, nil
}

func (s *Store) UpdateNote(ctx context.Context, id int64, update NoteUpdate) (Note, error) {
	note, err := s.NoteByID(ctx, id)
	if err != nil {
		return Note{}, err
	}

	kind := note.Kind
	if update.Kind != nil {
		kind = *update.Kind
	}
	body := note.Body
	if update.Body != nil {
		body = *update.Body
	}
	createdAt := note.CreatedAt
	if update.CreatedAt != nil {
		createdAt = *update.CreatedAt
	}

	_, err = s.db.ExecContext(ctx, `
UPDATE notes
SET kind = ?, body = ?, created_at = ?
WHERE id = ?
`, kind, body, formatTime(createdAt), id)
	if err != nil {
		return Note{}, err
	}
	return s.NoteByID(ctx, id)
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

func durationMinutes(duration time.Duration) int64 {
	return int64(duration.Round(time.Minute) / time.Minute)
}

func balanceWeekStart(t time.Time) time.Time {
	local := t.Local()
	year, month, day := local.Date()
	start := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
	offset := (int(start.Weekday()) + 6) % 7
	return start.AddDate(0, 0, -offset)
}

func endOfLocalDay(t time.Time) *time.Time {
	local := t.Local()
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	end := start.AddDate(0, 0, 1)
	return &end
}

func localDate(t time.Time) time.Time {
	local := t.Local()
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
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
