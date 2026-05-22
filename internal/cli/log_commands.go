package cli

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/Rasalas/work-cli/internal/db"
)

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
				project := []string{}
				if session.ProjectName.Valid {
					project = append(project, line("", session.ProjectName.String))
				}
				lines := []string{
					badgeLine(formatSessionDuration(session, now), formatDateTime(session.StartedAt)+" - "+formatEnd(&session)),
				}
				lines = append(lines, project...)
				printBlock(lines...)
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
