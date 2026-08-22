package db

import (
	"context"
	"database/sql"
	"time"
)

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
