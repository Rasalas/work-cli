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
	Worked            time.Duration
	Left              time.Duration
	TodayWorked       time.Duration
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

			printBlock(
				badgeLine("project", project.Name),
				line("week", formatDuration(info.Worked)+" / "+formatDuration(info.Schedule.WeeklyTarget)),
				line("left", formatDuration(info.Left)),
				line("schedule", formatWorkdayLabels(info.Workdays)),
				line("remaining", formatWeekdays(info.RemainingWorkdays)),
				line("per day", formatDuration(perDayTarget(info.Left, info.RemainingWorkdays))),
				line("deadline", formatDeadline(info.RemainingWorkdays)),
				line("balance", formatSignedDuration(info.Balance)),
				line("projected", formatSignedDuration(info.Projected)),
			)
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
	sessions, err := store.LogSessions(ctx, &start, &end, project.Name)
	if err != nil {
		return projectWeekInfo{}, err
	}
	worked := totalSessionDuration(sessions, now)
	left := schedule.WeeklyTarget - worked
	if left < 0 {
		left = 0
	}
	remainingWorkdays := remainingWeekWorkdays(selected, end, workdays)
	balance, err := store.ProjectBalance(ctx, project.ID)
	if err != nil {
		return projectWeekInfo{}, err
	}
	todayWorked := totalSessionDuration(todayProjectSessions(sessions, selected), now)
	todayTarget := todayTarget(schedule.WeeklyTarget, sessions, selected, remainingWorkdays, now)
	todayLeft := todayTarget - todayWorked
	if todayLeft < 0 {
		todayLeft = 0
	}
	todayOvertime := todayWorked - todayTarget
	if todayOvertime < 0 {
		todayOvertime = 0
	}

	return projectWeekInfo{
		Project:           project,
		Schedule:          schedule,
		Workdays:          workdays,
		Worked:            worked,
		Left:              left,
		TodayWorked:       todayWorked,
		TodayTarget:       todayTarget,
		TodayLeft:         todayLeft,
		TodayOvertime:     todayOvertime,
		RemainingWorkdays: remainingWorkdays,
		Balance:           balance,
		Projected:         balance + worked - schedule.WeeklyTarget,
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

func todayTarget(weeklyTarget time.Duration, sessions []db.Session, selected time.Time, remainingWorkdays []time.Time, now time.Time) time.Duration {
	for _, day := range remainingWorkdays {
		if dayStart(day).Equal(dayStart(selected)) {
			leftAtStartOfDay := weeklyTarget - totalSessionDuration(sessionsBeforeDay(sessions, selected), now)
			if leftAtStartOfDay < 0 {
				return 0
			}
			return perDayTarget(leftAtStartOfDay, remainingWorkdays)
		}
	}
	return 0
}

func sessionsBeforeDay(sessions []db.Session, selected time.Time) []db.Session {
	start := dayStart(selected)
	var before []db.Session
	for _, session := range sessions {
		if session.StartedAt.Before(start) {
			before = append(before, session)
		}
	}
	return before
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
		line("week", formatDuration(info.Worked)+" / "+formatDuration(info.Schedule.WeeklyTarget)),
		line("today", formatDuration(info.TodayWorked)+" / "+formatDuration(info.TodayTarget)),
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
