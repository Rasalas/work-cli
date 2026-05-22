package work

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tbuck/work-cli/internal/db"
	"github.com/tbuck/work-cli/internal/timeparse"
	"github.com/tbuck/work-cli/internal/tui"
)

var out io.Writer = os.Stdout

type options struct {
	project   string
	noProject bool
	at        string
	today     bool
	week      bool
}

func Execute() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "work",
		Short:         "Track local work sessions",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(startCmd(), noteCmd("do"), noteCmd("doing"), noteCmd("done"), endCmd(), statusCmd(), logCmd(), projectCmd())
	return cmd
}

func startCmd() *cobra.Command {
	var opts options
	cmd := &cobra.Command{
		Use:   "start [time]",
		Short: "Start a work session",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input := opts.at
			if len(args) == 1 {
				input = args[0]
			}
			startedAt, err := timeparse.ParseStartTime(input, time.Now())
			if err != nil {
				return err
			}

			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			ctx := context.Background()
			projectID, projectName, err := resolveProject(ctx, store, opts)
			if err != nil {
				return err
			}

			session, err := store.StartSession(ctx, startedAt, projectID)
			if errors.Is(err, db.ErrAlreadyRunning) {
				return fmt.Errorf("a session is already running; use `work status`")
			}
			if err != nil {
				return err
			}
			if projectName == "" && session.ProjectName.Valid {
				projectName = session.ProjectName.String
			}

			fmt.Fprintf(out, "Started: %s\n", formatDateTime(session.StartedAt))
			if projectName != "" {
				fmt.Fprintf(out, "Project: %s\n", projectName)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&opts.project, "project", "p", "", "project name")
	cmd.Flags().BoolVar(&opts.noProject, "no-project", false, "start without a project")
	cmd.Flags().StringVar(&opts.at, "at", "", "start time")
	return cmd
}

func noteCmd(kind string) *cobra.Command {
	return &cobra.Command{
		Use:   kind + " <note>",
		Short: "Add a " + kind + " note to the running session",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			note, err := store.AddNote(context.Background(), kind, strings.Join(args, " "), time.Now())
			if errors.Is(err, db.ErrNoRunningSession) {
				return fmt.Errorf("no session is running; use `work start`")
			}
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "%s %-5s %s\n", formatClock(note.CreatedAt), note.Kind, note.Body)
			return nil
		},
	}
}

func endCmd() *cobra.Command {
	var opts options
	cmd := &cobra.Command{
		Use:   "end [note]",
		Short: "End the running work session",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			endedAt, err := timeparse.ParseStartTime(opts.at, time.Now())
			if err != nil {
				return err
			}
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			session, err := store.EndRunningSession(context.Background(), endedAt, strings.Join(args, " "))
			if errors.Is(err, db.ErrNoRunningSession) {
				return fmt.Errorf("no session is running; use `work start`")
			}
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Ended: %s\n", formatDateTime(session.EndedAt.Time))
			fmt.Fprintf(out, "Duration: %s\n", formatDuration(session.EndedAt.Time.Sub(session.StartedAt)))
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.at, "at", "", "end time")
	return cmd
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current state and notes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			ctx := context.Background()
			running, err := store.RunningSession(ctx)
			if err != nil {
				return err
			}
			now := time.Now()
			today, err := todayDuration(ctx, store, now)
			if err != nil {
				return err
			}
			if running == nil {
				fmt.Fprintln(out, "Status: idle")
				fmt.Fprintf(out, "Today: %s\n", formatDuration(today))
				last, err := store.LastSession(ctx)
				if err != nil {
					return err
				}
				if last != nil {
					fmt.Fprintf(out, "Last session: %s - %s, %s\n", formatDateTime(last.StartedAt), formatEnd(last), formatSessionDuration(*last, now))
				}
				return nil
			}

			fmt.Fprintln(out, "Status: running")
			if running.ProjectName.Valid {
				fmt.Fprintf(out, "Project: %s\n", running.ProjectName.String)
			}
			fmt.Fprintf(out, "Started: %s\n", formatDateTime(running.StartedAt))
			fmt.Fprintf(out, "Duration: %s\n", formatDuration(now.Sub(running.StartedAt)))
			fmt.Fprintf(out, "Today: %s\n", formatDuration(today))

			notes, err := store.NotesForSession(ctx, running.ID)
			if err != nil {
				return err
			}
			if len(notes) == 0 {
				return nil
			}
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Notes:")
			printNotes(notes)
			return nil
		},
	}
}

func logCmd() *cobra.Command {
	var opts options
	cmd := &cobra.Command{
		Use:   "log",
		Short: "List logged sessions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			var from, to *time.Time
			now := time.Now()
			if opts.today {
				start := dayStart(now)
				end := start.AddDate(0, 0, 1)
				from, to = &start, &end
			} else if opts.week {
				start := weekStart(now)
				end := start.AddDate(0, 0, 7)
				from, to = &start, &end
			}

			ctx := context.Background()
			sessions, err := store.LogSessions(ctx, from, to, opts.project)
			if err != nil {
				return err
			}
			for _, session := range sessions {
				project := ""
				if session.ProjectName.Valid {
					project = "  " + session.ProjectName.String
				}
				fmt.Fprintf(out, "%s - %s  %s%s\n", formatDateTime(session.StartedAt), formatEnd(&session), formatSessionDuration(session, now), project)
				notes, err := store.NotesForSession(ctx, session.ID)
				if err != nil {
					return err
				}
				printNotes(notes)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&opts.today, "today", false, "show today's sessions")
	cmd.Flags().BoolVar(&opts.week, "week", false, "show this week's sessions")
	cmd.Flags().StringVarP(&opts.project, "project", "p", "", "filter by project")
	return cmd
}

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
			fmt.Fprintf(out, "Project: %s\n", project.Name)
			return nil
		},
	})
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

			projects, err := store.ActiveProjects(context.Background())
			if err != nil {
				return err
			}
			for _, project := range projects {
				fmt.Fprintln(out, project.Name)
			}
			return nil
		},
	})
	return cmd
}

func resolveProject(ctx context.Context, store *db.Store, opts options) (*int64, string, error) {
	if opts.project != "" && opts.noProject {
		return nil, "", fmt.Errorf("use either --project or --no-project")
	}
	if opts.noProject {
		return nil, "", nil
	}
	if opts.project != "" {
		project, err := store.AddProject(ctx, opts.project)
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

func openStore() (*db.Store, error) {
	path, err := db.DefaultPath()
	if err != nil {
		return nil, err
	}
	return db.Open(path)
}

func printNotes(notes []db.Note) {
	for _, note := range notes {
		fmt.Fprintf(out, "  %s %-5s %s\n", formatClock(note.CreatedAt), note.Kind, note.Body)
	}
}

func todayDuration(ctx context.Context, store *db.Store, now time.Time) (time.Duration, error) {
	start := dayStart(now)
	end := start.AddDate(0, 0, 1)
	sessions, err := store.LogSessions(ctx, &start, &end, "")
	if err != nil {
		return 0, err
	}

	var total time.Duration
	for _, session := range sessions {
		sessionEnd := now
		if session.EndedAt.Valid {
			sessionEnd = session.EndedAt.Time
		}
		if sessionEnd.After(session.StartedAt) {
			total += sessionEnd.Sub(session.StartedAt)
		}
	}
	return total, nil
}

func formatDateTime(t time.Time) string {
	return t.Local().Format("2006-01-02 15:04")
}

func formatClock(t time.Time) string {
	return t.Local().Format("15:04")
}

func formatEnd(session *db.Session) string {
	if session.EndedAt.Valid {
		return formatDateTime(session.EndedAt.Time)
	}
	return "running"
}

func formatSessionDuration(session db.Session, now time.Time) string {
	end := now
	if session.EndedAt.Valid {
		end = session.EndedAt.Time
	}
	return formatDuration(end.Sub(session.StartedAt))
}

func formatDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	minutes := int(duration.Round(time.Minute).Minutes())
	hours := minutes / 60
	minutes = minutes % 60
	if hours == 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

func dayStart(t time.Time) time.Time {
	local := t.Local()
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
}

func weekStart(t time.Time) time.Time {
	start := dayStart(t)
	offset := int(start.Weekday() - time.Monday)
	if offset < 0 {
		offset += 7
	}
	return start.AddDate(0, 0, -offset)
}
