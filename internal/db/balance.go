package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/Rasalas/work-cli/internal/calendar"
)

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
	start := calendar.WeekStart(at)
	if schedule != nil {
		start = calendar.WeekStart(schedule.LastUpdatedAt)
	}
	if latestAdjustmentDate.Valid {
		parsed, err := time.ParseInLocation("2006-01-02", latestAdjustmentDate.String, time.Local)
		if err != nil {
			return 0, err
		}
		start = calendar.WeekStart(parsed)
	}
	used, err := s.ProjectOvertimeUsed(ctx, projectID, &start, endOfLocalDay(at))
	if err != nil {
		return 0, err
	}
	if schedule == nil {
		return balance - used, nil
	}
	completedBefore := calendar.WeekStart(at)
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
			week := calendar.WeekStart(startedAt)
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
		week := calendar.WeekStart(usage.UsedOn)
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
