package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/Rasalas/work-cli/internal/db"
)

type projectWeekInfo struct {
	Project           db.Project
	Schedule          *db.ProjectSchedule
	Workdays          []time.Weekday
	Target            time.Duration
	Absence           time.Duration
	Worked            time.Duration
	OvertimeUsed      time.Duration
	Accounted         time.Duration
	Left              time.Duration
	TodayWorked       time.Duration
	TodayOvertimeUsed time.Duration
	TodayAccounted    time.Duration
	TodayTarget       time.Duration
	TodayLeft         time.Duration
	TodayOvertime     time.Duration
	RemainingWorkdays []time.Time
	Balance           time.Duration
	Projected         time.Duration
}

func weekCmd() *cobra.Command {
	var opts struct {
		project string
		date    string
	}
	cmd := &cobra.Command{
		Use:   "week [project]",
		Short: "Show project weekly progress",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.project != "" && len(args) > 0 {
				return fmt.Errorf("use either --project or positional project")
			}
			selected := dayStart(time.Now())
			var err error
			if opts.date != "" {
				selected, err = parseLogDate(opts.date, time.Local)
				if err != nil {
					return err
				}
			}

			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			ctx := context.Background()
			projectArgs := args
			if opts.project != "" {
				projectArgs = []string{opts.project}
			}
			project, err := resolveProjectCommandProject(ctx, store, projectArgs, "work week <projectname>")
			if err != nil {
				return err
			}
			info, err := loadProjectWeekInfo(ctx, store, project, selected, time.Now())
			if err != nil {
				return err
			}

			lines := []string{
				badgeLine("project", project.Name),
				line("week", formatDuration(info.Worked)+" / "+formatDuration(info.Target)),
			}
			if info.Absence > 0 {
				lines = append(lines, line("absence", formatDuration(info.Absence)))
			}
			if info.OvertimeUsed > 0 {
				lines = append(lines,
					line("overtime", formatDuration(info.OvertimeUsed)),
					line("accounted", formatDuration(info.Accounted)+" / "+formatDuration(info.Target)),
				)
			}
			lines = append(lines,
				line("left", formatDuration(info.Left)),
				line("schedule", formatWorkdayLabels(info.Workdays)),
				line("remaining", formatWeekdays(info.RemainingWorkdays)),
				line("per day", formatDuration(perDayTarget(info.Left, info.RemainingWorkdays))),
				line("deadline", formatDeadline(info.RemainingWorkdays)),
				line("balance", formatSignedDuration(info.Balance)),
				line("projected", formatSignedDuration(info.Projected)),
			)
			printBlock(lines...)
			return nil
		},
	}
	cmd.Flags().StringVarP(&opts.project, "project", "p", "", "project name")
	cmd.Flags().StringVar(&opts.date, "date", "", "week date YYYY-MM-DD")
	return cmd
}

func loadProjectWeekInfo(ctx context.Context, store *db.Store, project db.Project, selected, now time.Time) (projectWeekInfo, error) {
	schedule, err := store.ProjectSchedule(ctx, project.ID)
	if err != nil {
		return projectWeekInfo{}, err
	}
	if schedule == nil {
		return projectWeekInfo{}, fmt.Errorf("project %q has no weekly target; use `work project set %s --weekly <duration> --workdays <days>`", project.Name, project.Name)
	}
	workdays, err := parseWorkdays(schedule.Workdays)
	if err != nil {
		return projectWeekInfo{}, err
	}

	start := weekStart(selected)
	end := start.AddDate(0, 0, 7)
	absenceReduction, err := store.ProjectAbsenceTargetReduction(ctx, project.ID, start, end, schedule.WeeklyTarget, schedule.Workdays)
	if err != nil {
		return projectWeekInfo{}, err
	}
	target := schedule.WeeklyTarget - absenceReduction
	if target < 0 {
		target = 0
	}
	absences, err := store.ProjectAbsences(ctx, project.ID, &start, &end)
	if err != nil {
		return projectWeekInfo{}, err
	}
	absentDates := projectAbsentDateSet(absences, start, end)
	sessions, err := store.LogSessions(ctx, &start, &end, project.Name)
	if err != nil {
		return projectWeekInfo{}, err
	}
	worked := totalSessionDuration(sessions, now)
	overtimeUsed, err := store.ProjectOvertimeUsed(ctx, project.ID, &start, &end)
	if err != nil {
		return projectWeekInfo{}, err
	}
	accounted := worked + overtimeUsed
	left := target - accounted
	if left < 0 {
		left = 0
	}
	remainingWorkdays := remainingWeekWorkdays(selected, end, workdays)
	remainingWorkdays = filterAbsentDates(remainingWorkdays, absentDates)
	balance, err := store.ProjectBalanceAt(ctx, project.ID, selected)
	if err != nil {
		return projectWeekInfo{}, err
	}
	todayWorked := totalSessionDuration(todayProjectSessions(sessions, selected), now)
	todayStart := dayStart(selected)
	todayEnd := todayStart.AddDate(0, 0, 1)
	todayOvertimeUsed, err := store.ProjectOvertimeUsed(ctx, project.ID, &todayStart, &todayEnd)
	if err != nil {
		return projectWeekInfo{}, err
	}
	todayAccounted := todayWorked + todayOvertimeUsed
	todayTarget := todayTarget(schedule.WeeklyTarget, selected, workdays)
	if absentDates[dayStart(selected).Format("2006-01-02")] {
		todayTarget = 0
	}
	todayLeft := todayTarget - todayAccounted
	if todayLeft < 0 {
		todayLeft = 0
	}
	todayOvertime := todayAccounted - todayTarget
	if todayOvertime < 0 {
		todayOvertime = 0
	}

	return projectWeekInfo{
		Project:           project,
		Schedule:          schedule,
		Workdays:          workdays,
		Target:            target,
		Absence:           absenceReduction,
		Worked:            worked,
		OvertimeUsed:      overtimeUsed,
		Accounted:         accounted,
		Left:              left,
		TodayWorked:       todayWorked,
		TodayOvertimeUsed: todayOvertimeUsed,
		TodayAccounted:    todayAccounted,
		TodayTarget:       todayTarget,
		TodayLeft:         todayLeft,
		TodayOvertime:     todayOvertime,
		RemainingWorkdays: remainingWorkdays,
		Balance:           balance,
		Projected:         balance + accounted - target,
	}, nil
}

func totalSessionDuration(sessions []db.Session, now time.Time) time.Duration {
	var total time.Duration
	for _, session := range sessions {
		end := now
		if session.EndedAt.Valid {
			end = session.EndedAt.Time
		}
		if end.After(session.StartedAt) {
			total += end.Sub(session.StartedAt)
		}
	}
	return total
}

func todayProjectSessions(sessions []db.Session, selected time.Time) []db.Session {
	start := dayStart(selected)
	end := start.AddDate(0, 0, 1)
	var today []db.Session
	for _, session := range sessions {
		if !session.StartedAt.Before(start) && session.StartedAt.Before(end) {
			today = append(today, session)
		}
	}
	return today
}

func remainingWeekWorkdays(selected, weekEnd time.Time, workdays []time.Weekday) []time.Time {
	workdaySet := make(map[time.Weekday]bool)
	for _, day := range workdays {
		workdaySet[day] = true
	}
	start := dayStart(selected)
	var remaining []time.Time
	for day := start; day.Before(weekEnd); day = day.AddDate(0, 0, 1) {
		if workdaySet[day.Weekday()] {
			remaining = append(remaining, day)
		}
	}
	return remaining
}

func projectAbsentDateSet(absences []db.ProjectAbsence, from, to time.Time) map[string]bool {
	dates := make(map[string]bool)
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
			dates[day.Format("2006-01-02")] = true
		}
	}
	return dates
}

func filterAbsentDates(days []time.Time, absentDates map[string]bool) []time.Time {
	var filtered []time.Time
	for _, day := range days {
		if !absentDates[day.Format("2006-01-02")] {
			filtered = append(filtered, day)
		}
	}
	return filtered
}

func todayTarget(weeklyTarget time.Duration, selected time.Time, workdays []time.Weekday) time.Duration {
	if len(workdays) == 0 {
		return 0
	}
	for _, day := range workdays {
		if day == selected.Weekday() {
			return weeklyTarget / time.Duration(len(workdays))
		}
	}
	return 0
}

func perDayTarget(left time.Duration, remainingWorkdays []time.Time) time.Duration {
	if len(remainingWorkdays) == 0 {
		return left
	}
	return left / time.Duration(len(remainingWorkdays))
}

func formatWeekdays(days []time.Time) string {
	if len(days) == 0 {
		return "none"
	}
	workdays := make([]time.Weekday, 0, len(days))
	for _, day := range days {
		workdays = append(workdays, day.Weekday())
	}
	return formatWorkdayLabels(workdays)
}

func formatDeadline(days []time.Time) string {
	if len(days) == 0 {
		return "none"
	}
	return days[len(days)-1].Format("2006-01-02")
}

func statusProjectWeekLines(info projectWeekInfo, now time.Time) []string {
	lines := []string{
		line("", info.Project.Name),
		line("week", formatDuration(info.Worked)+" / "+formatDuration(info.Target)),
		line("today", formatDuration(info.TodayWorked)+" / "+formatDuration(info.TodayTarget)),
	}
	if info.Absence > 0 {
		lines = append(lines, line("absence", formatDuration(info.Absence)))
	}
	if info.TodayOvertimeUsed > 0 {
		lines = append(lines,
			line("overtime", formatDuration(info.TodayOvertimeUsed)),
			line("accounted", formatDuration(info.TodayAccounted)+" / "+formatDuration(info.TodayTarget)),
		)
	}
	if info.TodayLeft > 0 {
		lines = append(lines,
			line("left today", formatDuration(info.TodayLeft)),
			line("until", formatClock(now.Add(info.TodayLeft))),
		)
	}
	if info.TodayOvertime > 0 {
		lines = append(lines, line("over today", formatSignedDuration(info.TodayOvertime)))
	}
	lines = append(lines,
		line("balance", formatSignedDuration(info.Balance)),
	)
	return lines
}
