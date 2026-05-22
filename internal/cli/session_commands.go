package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Rasalas/work-cli/internal/db"
	"github.com/Rasalas/work-cli/internal/timeparse"
)

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

			lines := []string{badgeLine("started", formatDateTime(session.StartedAt))}
			if projectName != "" {
				lines = append(lines, line("", projectName))
			}
			printBlock(lines...)
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
			printBlock(noteLine(note))
			return nil
		},
	}
}

func endCmd() *cobra.Command {
	var opts options
	cmd := &cobra.Command{
		Use:   "end [time] [note]",
		Short: "End the running work session",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			endedAt, note, err := parseEndArgs(opts.at, args, time.Now())
			if err != nil {
				return err
			}
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			session, err := store.EndRunningSession(context.Background(), endedAt, note)
			if errors.Is(err, db.ErrNoRunningSession) {
				return fmt.Errorf("no session is running; use `work start`")
			}
			if err != nil {
				return err
			}
			printBlock(
				badgeLine("ended", formatDateTime(session.EndedAt.Time)),
				line("", formatDuration(session.EndedAt.Time.Sub(session.StartedAt))),
			)
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.at, "at", "", "end time")
	return cmd
}

func parseEndArgs(at string, args []string, base time.Time) (time.Time, string, error) {
	noteArgs := args
	if at == "" && len(args) > 0 {
		if endedAt, err := timeparse.ParseStartTime(args[0], base); err == nil {
			return endedAt, strings.Join(args[1:], " "), nil
		}
	}

	endedAt, err := timeparse.ParseStartTime(at, base)
	if err != nil {
		return time.Time{}, "", err
	}
	return endedAt, strings.Join(noteArgs, " "), nil
}

func statusCmd() *cobra.Command {
	var opts options
	cmd := &cobra.Command{
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
			today, err := todaySummary(ctx, store, now)
			if err != nil {
				return err
			}
			var target time.Duration
			if opts.target != "" {
				target, err = timeparse.ParseWorkDuration(opts.target)
				if err != nil {
					return err
				}
			}
			if running == nil {
				lines := []string{badgeLine("idle", formatDuration(today.Work)+" today")}
				lines = appendTodaySummaryLines(lines, today, nil, false)
				lines = appendTargetStatusLine(lines, target, today.Work, now, false)
				printBlock(lines...)
				printTodayProjects(today.Sessions, now)
				if opts.timeline {
					printTimeline(today.Sessions, now)
				}
				last, err := store.LastSession(ctx)
				if err != nil {
					return err
				}
				if last != nil {
					printMuted(
						line("last", fmt.Sprintf("%s - %s", formatDateTime(last.StartedAt), formatEnd(last))),
						line("", formatSessionDuration(*last, now)),
					)
				}
				if err := printTodayNotes(ctx, store, today.Sessions); err != nil {
					return err
				}
				return nil
			}

			lines := []string{
				badgeLine("running", formatDuration(now.Sub(running.StartedAt))),
				"",
			}
			if running.ProjectName.Valid {
				lines = append(lines, line("", running.ProjectName.String))
			}
			lines = append(lines,
				line("current", formatDateTime(running.StartedAt)),
			)
			lines = appendTodaySummaryLines(lines, today, running, true)
			lines = appendTargetStatusLine(lines, target, today.Work, now, true)
			printBlock(lines...)
			printTodayProjects(today.Sessions, now)
			if opts.timeline {
				printTimeline(today.Sessions, now)
			}
			return printTodayNotes(ctx, store, today.Sessions)
		},
	}
	cmd.Flags().BoolVar(&opts.timeline, "timeline", false, "show today's session timeline")
	cmd.Flags().BoolVar(&opts.timeline, "detail", false, "show today's session timeline")
	cmd.Flags().StringVar(&opts.target, "target", "", "show when today's work target will be reached")
	return cmd
}

type projectDuration struct {
	Name     string
	Duration time.Duration
}

func todayProjectDurations(sessions []db.Session, now time.Time) []projectDuration {
	indexByName := make(map[string]int)
	var durations []projectDuration
	for _, session := range sessions {
		name := sessionProjectTitle(session)
		index, ok := indexByName[name]
		if !ok {
			index = len(durations)
			indexByName[name] = index
			durations = append(durations, projectDuration{Name: name})
		}

		end := now
		if session.EndedAt.Valid {
			end = session.EndedAt.Time
		}
		if end.After(session.StartedAt) {
			durations[index].Duration += end.Sub(session.StartedAt)
		}
	}
	return durations
}

func printTodayProjects(sessions []db.Session, now time.Time) {
	durations := todayProjectDurations(sessions, now)
	if len(durations) == 0 {
		return
	}
	printSection("projects")
	for _, project := range durations {
		printLine(line(project.Name, formatDuration(project.Duration)))
	}
	fmt.Fprintln(out)
}

func printTodayNotes(ctx context.Context, store *db.Store, sessions []db.Session) error {
	printed := false
	currentProject := ""
	for _, session := range sessions {
		sessionNotes, err := store.NotesForSession(ctx, session.ID)
		if err != nil {
			return err
		}
		if len(sessionNotes) == 0 {
			continue
		}
		if !printed {
			printSection("notes")
			printed = true
		}
		project := sessionProjectTitle(session)
		if project != currentProject {
			if currentProject != "" {
				fmt.Fprintln(out)
			}
			printLine(line("", project))
			currentProject = project
		}
		for _, note := range sessionNotes {
			printLine(noteLine(note))
		}
	}
	if printed {
		fmt.Fprintln(out)
	}
	return nil
}

func sessionProjectTitle(session db.Session) string {
	if session.ProjectName.Valid && session.ProjectName.String != "" {
		return session.ProjectName.String
	}
	return "undefined"
}

func appendTodaySummaryLines(lines []string, summary daySummaryInfo, running *db.Session, includeToday bool) []string {
	if summary.First.Valid && (running == nil || !summary.First.Time.Equal(running.StartedAt)) {
		lines = append(lines, line("first", formatDateTime(summary.First.Time)))
	}
	if includeToday {
		lines = append(lines, line("today", formatDuration(summary.Work)))
	}
	if summary.Paused > 0 {
		lines = append(lines, line("paused", formatDuration(summary.Paused)))
	}
	return lines
}

func appendTargetStatusLine(lines []string, target, worked time.Duration, now time.Time, running bool) []string {
	if target == 0 {
		return lines
	}
	remaining := target - worked
	if remaining <= 0 {
		return append(lines, line("left", "0m"))
	}
	if running {
		return append(lines, line("until", formatClock(now.Add(remaining))))
	}
	return append(lines, line("left", formatDuration(remaining)))
}

func printTimeline(sessions []db.Session, now time.Time) {
	if len(sessions) == 0 {
		return
	}
	printSection("timeline")
	for _, session := range sessions {
		printLine(line("", timelineSessionValue(session, now)))
	}
	fmt.Fprintln(out)
}

func timelineSessionValue(session db.Session, now time.Time) string {
	end := now
	endText := "now"
	if session.EndedAt.Valid {
		end = session.EndedAt.Time
		endText = formatClock(end)
	}
	return fmt.Sprintf("%s - %s  %s", formatClock(session.StartedAt), endText, formatDuration(end.Sub(session.StartedAt)))
}
