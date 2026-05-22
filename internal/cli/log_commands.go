package cli

import (
	"context"
	"database/sql"
	"fmt"
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
			chronologicalSessions(sessions)
			for _, session := range sessions {
				lines := []string{
					logSessionHeader(session, now),
				}
				printBlock(lines...)
				notes, err := store.NotesForSession(ctx, session.ID)
				if err != nil {
					return err
				}
				printLogNotes(session.ID, notes)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&opts.today, "today", false, "show today's sessions")
	cmd.Flags().BoolVar(&opts.week, "week", false, "show this week's sessions")
	cmd.Flags().StringVarP(&opts.project, "project", "p", "", "filter by project")
	return cmd
}

func logSessionHeader(session db.Session, now time.Time) string {
	timing := metaStyle.Render(formatDateTime(session.StartedAt) + " - " + formatEnd(&session))
	duration := logDurationStyle.Render(formatSessionDuration(session, now))
	id := metaStyle.Render(fmt.Sprintf("#%d", session.ID))
	if session.ProjectName.Valid {
		return fmt.Sprintf("%s   %s  %s  %s", id, duration, valueStyle.Render(session.ProjectName.String), timing)
	}
	return fmt.Sprintf("%s   %s  %s", id, duration, timing)
}

func printLogNotes(sessionID int64, notes []db.Note) {
	if len(notes) == 0 {
		return
	}
	for _, note := range notes {
		printLine(logNoteLine(sessionID, note))
	}
	fmt.Fprintln(out)
}

func logNoteLine(sessionID int64, note db.Note) string {
	return fmt.Sprintf("%*s%s", len(fmt.Sprintf("#%d   ", sessionID)), "", noteLine(note))
}

func todayDuration(ctx context.Context, store *db.Store, now time.Time) (time.Duration, error) {
	summary, err := todaySummary(ctx, store, now)
	if err != nil {
		return 0, err
	}
	return summary.Work, nil
}

type daySummaryInfo struct {
	Sessions []db.Session
	Work     time.Duration
	Paused   time.Duration
	First    sql.NullTime
}

func todaySummary(ctx context.Context, store *db.Store, now time.Time) (daySummaryInfo, error) {
	start := dayStart(now)
	end := start.AddDate(0, 0, 1)
	sessions, err := store.LogSessions(ctx, &start, &end, "")
	if err != nil {
		return daySummaryInfo{}, err
	}

	chronologicalSessions(sessions)

	var summary daySummaryInfo
	summary.Sessions = sessions
	for _, session := range sessions {
		if !summary.First.Valid || session.StartedAt.Before(summary.First.Time) {
			summary.First = sql.NullTime{Time: session.StartedAt, Valid: true}
		}
		sessionEnd := now
		if session.EndedAt.Valid {
			sessionEnd = session.EndedAt.Time
		}
		if sessionEnd.After(session.StartedAt) {
			summary.Work += sessionEnd.Sub(session.StartedAt)
		}
	}

	for i := 1; i < len(sessions); i++ {
		previous := sessions[i-1]
		if !previous.EndedAt.Valid {
			continue
		}
		if sessions[i].StartedAt.After(previous.EndedAt.Time) {
			summary.Paused += sessions[i].StartedAt.Sub(previous.EndedAt.Time)
		}
	}
	return summary, nil
}

func chronologicalSessions(sessions []db.Session) {
	for i, j := 0, len(sessions)-1; i < j; i, j = i+1, j-1 {
		sessions[i], sessions[j] = sessions[j], sessions[i]
	}
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
