package cli

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/sahilm/fuzzy"
	"github.com/spf13/cobra"

	"github.com/Rasalas/work-cli/internal/db"
	"github.com/Rasalas/work-cli/internal/timeparse"
	"github.com/Rasalas/work-cli/internal/tui"
)

func projectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "add <name>",
		Short: "Add or reactivate a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			project, err := store.AddProject(context.Background(), args[0])
			if err != nil {
				return err
			}
			printBlock(badgeLine("project", project.Name))
			return nil
		},
	})
	cmd.AddCommand(projectSetCmd())
	cmd.AddCommand(projectBalanceCmd())
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List active projects",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			ctx := context.Background()
			projects, err := store.ActiveProjects(ctx)
			if err != nil {
				return err
			}
			if len(projects) == 0 {
				printMuted(line("projects", "none"))
				return nil
			}
			printSection("projects")
			for _, project := range projects {
				schedule, err := store.ProjectSchedule(ctx, project.ID)
				if err != nil {
					return err
				}
				printLine(line("", projectListLine(project, schedule)))
			}
			fmt.Fprintln(out)
			return nil
		},
	})
	return cmd
}

func projectSetCmd() *cobra.Command {
	var opts struct {
		weekly   string
		workdays string
	}
	cmd := &cobra.Command{
		Use:   "set <name>",
		Short: "Set project planning defaults",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			ctx := context.Background()
			project, err := resolveProjectCommandProject(ctx, store, args, "work project set <projectname> --weekly <duration> --workdays <days>")
			if err != nil {
				return err
			}
			if opts.weekly == "" {
				return fmt.Errorf("--weekly is required")
			}
			if opts.workdays == "" {
				return fmt.Errorf("--workdays is required")
			}
			weeklyTarget, err := timeparse.ParseWorkDuration(opts.weekly)
			if err != nil {
				return err
			}
			workdays, err := parseWorkdays(opts.workdays)
			if err != nil {
				return err
			}
			if err := store.SetProjectSchedule(ctx, project.ID, weeklyTarget, formatWorkdayKeys(workdays)); err != nil {
				return err
			}
			printBlock(
				badgeLine("project", project.Name),
				line("weekly", formatDuration(weeklyTarget)+"/week"),
				line("workdays", formatWorkdayLabels(workdays)),
			)
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.weekly, "weekly", "", "weekly target duration")
	cmd.Flags().StringVar(&opts.workdays, "workdays", "", "comma-separated workdays")
	return cmd
}

func projectBalanceCmd() *cobra.Command {
	var opts struct {
		set  string
		date string
	}
	cmd := &cobra.Command{
		Use:   "balance <name>",
		Short: "Show or set project overtime balance",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			ctx := context.Background()
			project, err := resolveProjectCommandProject(ctx, store, args, "work project balance <projectname> [--set <duration>]")
			if err != nil {
				return err
			}
			if opts.set != "" {
				balance, err := timeparse.ParseWorkDuration(opts.set)
				if err != nil {
					return err
				}
				date := dayStart(time.Now())
				if opts.date != "" {
					date, err = parseLogDate(opts.date, time.Local)
					if err != nil {
						return err
					}
				}
				if err := store.SetProjectBalance(ctx, project.ID, date, balance); err != nil {
					return err
				}
			}
			balance, err := store.ProjectBalance(ctx, project.ID)
			if err != nil {
				return err
			}
			printBlock(
				badgeLine("project", project.Name),
				line("balance", formatSignedDuration(balance)),
			)
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.set, "set", "", "set project overtime balance")
	cmd.Flags().StringVar(&opts.date, "date", "", "balance adjustment date YYYY-MM-DD")
	return cmd
}

func projectListLine(project db.Project, schedule *db.ProjectSchedule) string {
	if schedule == nil {
		return project.Name
	}
	workdays, err := parseWorkdays(schedule.Workdays)
	if err != nil {
		return fmt.Sprintf("%s  %s/week", project.Name, formatDuration(schedule.WeeklyTarget))
	}
	return fmt.Sprintf("%s  %s/week  %s", project.Name, formatDuration(schedule.WeeklyTarget), formatWorkdayLabels(workdays))
}

func resolveProjectCommandProject(ctx context.Context, store *db.Store, args []string, usage string) (db.Project, error) {
	if len(args) > 0 {
		return resolveNamedProject(ctx, store, args[0])
	}
	projects, err := store.ActiveProjects(ctx)
	if err != nil {
		return db.Project{}, err
	}
	switch len(projects) {
	case 0:
		return db.Project{}, fmt.Errorf("missing project name; use `%s`", usage)
	case 1:
		return projects[0], nil
	default:
		picked, err := tui.PickProject(projects)
		if err != nil {
			return db.Project{}, err
		}
		if picked == nil {
			return db.Project{}, fmt.Errorf("project selection cancelled")
		}
		return *picked, nil
	}
}

func resolveProject(ctx context.Context, store *db.Store, opts options) (*int64, string, error) {
	if opts.project != "" && opts.noProject {
		return nil, "", fmt.Errorf("use either --project or --no-project")
	}
	if opts.noProject {
		return nil, "", nil
	}
	if opts.project != "" {
		project, err := resolveNamedProject(ctx, store, opts.project)
		if err != nil {
			return nil, "", err
		}
		return &project.ID, project.Name, nil
	}

	projects, err := store.ActiveProjects(ctx)
	if err != nil {
		return nil, "", err
	}
	switch len(projects) {
	case 0:
		return nil, "", nil
	case 1:
		return &projects[0].ID, projects[0].Name, nil
	default:
		picked, err := tui.PickProject(projects)
		if err != nil {
			return nil, "", err
		}
		if picked == nil {
			return nil, "", fmt.Errorf("project selection cancelled")
		}
		return &picked.ID, picked.Name, nil
	}
}

func resolveNamedProject(ctx context.Context, store *db.Store, name string) (db.Project, error) {
	project, err := store.ProjectByName(ctx, name)
	if err == nil {
		if project.Archived {
			return store.AddProject(ctx, project.Name)
		}
		return project, nil
	}
	if err != sql.ErrNoRows {
		return db.Project{}, err
	}

	projects, err := store.ActiveProjects(ctx)
	if err != nil {
		return db.Project{}, err
	}
	matches := fuzzy.FindFrom(name, projectSource(projects))
	switch len(matches) {
	case 0:
		return db.Project{}, fmt.Errorf("project %q not found; use `work project add %s` to create it", name, name)
	case 1:
		return projects[matches[0].Index], nil
	default:
		names := make([]string, 0, len(matches))
		for _, match := range matches {
			names = append(names, projects[match.Index].Name)
		}
		return db.Project{}, fmt.Errorf("project %q matches multiple projects: %s", name, strings.Join(names, ", "))
	}
}

type projectSource []db.Project

func (p projectSource) String(i int) string {
	return p[i].Name
}

func (p projectSource) Len() int {
	return len(p)
}

func parseWorkdays(input string) ([]time.Weekday, error) {
	parts := strings.Split(input, ",")
	seen := make(map[time.Weekday]bool)
	var workdays []time.Weekday
	for _, part := range parts {
		day, ok := parseWorkday(strings.TrimSpace(part))
		if !ok {
			return nil, fmt.Errorf("invalid workday %q; use mon,tue,wed,thu,fri,sat,sun", part)
		}
		if seen[day] {
			return nil, fmt.Errorf("duplicate workday %q", part)
		}
		seen[day] = true
		workdays = append(workdays, day)
	}
	if len(workdays) == 0 {
		return nil, fmt.Errorf("at least one workday is required")
	}
	return workdays, nil
}

func parseWorkday(input string) (time.Weekday, bool) {
	switch strings.ToLower(input) {
	case "mon", "monday":
		return time.Monday, true
	case "tue", "tues", "tuesday":
		return time.Tuesday, true
	case "wed", "wednesday":
		return time.Wednesday, true
	case "thu", "thur", "thurs", "thursday":
		return time.Thursday, true
	case "fri", "friday":
		return time.Friday, true
	case "sat", "saturday":
		return time.Saturday, true
	case "sun", "sunday":
		return time.Sunday, true
	default:
		return time.Sunday, false
	}
}

func formatWorkdayKeys(workdays []time.Weekday) string {
	parts := make([]string, 0, len(workdays))
	for _, day := range workdays {
		parts = append(parts, workdayKey(day))
	}
	return strings.Join(parts, ",")
}

func formatWorkdayLabels(workdays []time.Weekday) string {
	parts := make([]string, 0, len(workdays))
	for _, day := range workdays {
		parts = append(parts, workdayLabel(day))
	}
	return strings.Join(parts, ", ")
}

func workdayKey(day time.Weekday) string {
	switch day {
	case time.Monday:
		return "mon"
	case time.Tuesday:
		return "tue"
	case time.Wednesday:
		return "wed"
	case time.Thursday:
		return "thu"
	case time.Friday:
		return "fri"
	case time.Saturday:
		return "sat"
	default:
		return "sun"
	}
}

func workdayLabel(day time.Weekday) string {
	switch day {
	case time.Monday:
		return "Mon"
	case time.Tuesday:
		return "Tue"
	case time.Wednesday:
		return "Wed"
	case time.Thursday:
		return "Thu"
	case time.Friday:
		return "Fri"
	case time.Saturday:
		return "Sat"
	default:
		return "Sun"
	}
}

func formatSignedDuration(duration time.Duration) string {
	sign := "+"
	if duration < 0 {
		sign = "-"
		duration = -duration
	}
	return sign + formatDuration(duration)
}
