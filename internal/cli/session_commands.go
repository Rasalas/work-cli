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
				printBlock(badgeLine("idle", formatDuration(today)+" today"))
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
				line("started", formatDateTime(running.StartedAt)),
				line("today", formatDuration(today)),
			)
			printBlock(lines...)

			notes, err := store.NotesForSession(ctx, running.ID)
			if err != nil {
				return err
			}
			if len(notes) == 0 {
				return nil
			}
			printSection("notes")
			printNotes(notes)
			return nil
		},
	}
}
