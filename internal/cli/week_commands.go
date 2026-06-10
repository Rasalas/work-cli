package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/Rasalas/work-cli/internal/db"
)

func weekCmd() *cobra.Command {
	var opts struct {
		project string
		date    string
	}
	cmd := &cobra.Command{
		Use:   "week",
		Short: "Show project weekly progress",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.project == "" {
				return fmt.Errorf("--project is required")
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
			project, err := resolveNamedProject(ctx, store, opts.project)
			if err != nil {
				return err
			}
			schedule, err := store.ProjectSchedule(ctx, project.ID)
			if err != nil {
				return err
			}
			if schedule == nil {
				return fmt.Errorf("project %q has no weekly target; use `work project set %s --weekly <duration> --workdays <days>`", project.Name, project.Name)
			}
			workdays, err := parseWorkdays(schedule.Workdays)
			if err != nil {
				return err
			}

			start := weekStart(selected)
			end := start.AddDate(0, 0, 7)
			sessions, err := store.LogSessions(ctx, &start, &end, project.Name)
			if err != nil {
				return err
			}
			worked := totalSessionDuration(sessions, time.Now())
			left := schedule.WeeklyTarget - worked
			if left < 0 {
				left = 0
			}
			remainingWorkdays := remainingWeekWorkdays(selected, end, workdays)
			perDay := left
			if len(remainingWorkdays) > 0 {
				perDay = left / time.Duration(len(remainingWorkdays))
			}
			balance, err := store.ProjectBalance(ctx, project.ID)
			if err != nil {
				return err
			}
			projected := balance + worked - schedule.WeeklyTarget

			printBlock(
				badgeLine("project", project.Name),
				line("week", formatDuration(worked)+" / "+formatDuration(schedule.WeeklyTarget)),
				line("left", formatDuration(left)),
				line("workdays", formatWeekdays(remainingWorkdays)),
				line("per day", formatDuration(perDay)),
				line("deadline", formatDeadline(remainingWorkdays)),
				line("balance", formatSignedDuration(balance)),
				line("projected", formatSignedDuration(projected)),
			)
			return nil
		},
	}
	cmd.Flags().StringVarP(&opts.project, "project", "p", "", "project name")
	cmd.Flags().StringVar(&opts.date, "date", "", "week date YYYY-MM-DD")
	return cmd
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
