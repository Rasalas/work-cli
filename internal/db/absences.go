package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Rasalas/work-cli/internal/calendar"
)

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

var ErrOverlappingAbsence = errors.New("an overlapping absence already exists")

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
	startsOn = calendar.DayStart(startsOn)
	endsOn = calendar.DayStart(endsOn)
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
		args = append(args, calendar.DayStart(*from).Format("2006-01-02"))
	}
	if to != nil {
		where += " AND starts_on < ?"
		args = append(args, calendar.DayStart(*to).Format("2006-01-02"))
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
	start := calendar.DayStart(date)
	end := start.AddDate(0, 0, 1)
	absences, err := s.ProjectAbsences(ctx, projectID, &start, &end)
	if err != nil {
		return false, err
	}
	return len(absences) > 0, nil
}

func (s *Store) ProjectAbsenceTargetReduction(ctx context.Context, projectID int64, from, to time.Time, weeklyTarget time.Duration, workdays string) (time.Duration, error) {
	from = calendar.DayStart(from)
	to = calendar.DayStart(to)
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
