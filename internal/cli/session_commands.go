package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
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
	var opts struct {
		sessionID int64
		last      bool
		at        string
	}
	cmd := &cobra.Command{
		Use:   kind + " <note>",
		Short: "Add a " + kind + " note to a work session",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.sessionID < 0 {
				return fmt.Errorf("session id must be positive")
			}
			if opts.sessionID > 0 && opts.last {
				return fmt.Errorf("use either --session or --last")
			}

			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			ctx := context.Background()
			noteBody := strings.Join(args, " ")
			now := time.Now()
			var note db.Note
			if opts.sessionID > 0 || opts.last || opts.at != "" {
				session, err := noteTargetSession(ctx, store, opts.sessionID, opts.last)
				if errors.Is(err, db.ErrNoRunningSession) {
					return fmt.Errorf("no session is running; use `work start`, `%s --last`, or `%s --session <id>`", kind, kind)
				}
				if errors.Is(err, sql.ErrNoRows) {
					return fmt.Errorf("session #%d not found", opts.sessionID)
				}
				if err != nil {
					return err
				}
				if session == nil {
					if opts.last {
						return fmt.Errorf("no ended sessions found; use `work log` to choose a session or `%s --session <id>`", kind)
					}
					return fmt.Errorf("no session is running; use `work start`, `%s --last`, or `%s --session <id>`", kind, kind)
				}
				createdAt, err := parseNoteTime(opts.at, *session, now)
				if err != nil {
					return err
				}
				note, err = store.AddNoteToSession(ctx, session.ID, kind, noteBody, createdAt)
			} else {
				note, err = store.AddNote(ctx, kind, noteBody, now)
			}
			if errors.Is(err, db.ErrNoRunningSession) {
				return fmt.Errorf("no session is running; use `work start`, `%s --last`, or `%s --session <id>`", kind, kind)
			}
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("session #%d not found", opts.sessionID)
			}
			if err != nil {
				return err
			}
			printBlock(noteLine(note))
			return nil
		},
	}
	cmd.Flags().Int64Var(&opts.sessionID, "session", 0, "add note to a specific session id")
	cmd.Flags().BoolVar(&opts.last, "last", false, "add note to the last ended session")
	cmd.Flags().StringVar(&opts.at, "at", "", "note time, or start/end for the target session")
	return cmd
}

func noteTargetSession(ctx context.Context, store *db.Store, sessionID int64, last bool) (*db.Session, error) {
	if sessionID > 0 {
		session, err := store.SessionByID(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		return &session, nil
	}
	if last {
		session, err := store.LastEndedSession(ctx)
		if err != nil {
			return nil, err
		}
		return session, nil
	}
	return store.RunningSession(ctx)
}

func parseNoteTime(input string, session db.Session, fallback time.Time) (time.Time, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		if session.EndedAt.Valid {
			return session.EndedAt.Time, nil
		}
		return fallback, nil
	}

	switch strings.ToLower(input) {
	case "start":
		return session.StartedAt, nil
	case "end":
		if !session.EndedAt.Valid {
			return time.Time{}, fmt.Errorf("session #%d has no end time", session.ID)
		}
		return session.EndedAt.Time, nil
	default:
		return timeparse.ParseStartTime(input, session.StartedAt)
	}
}

func endCmd() *cobra.Command {
	var opts struct {
		at          string
		useOvertime string
	}
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

			ctx := context.Background()
			var overtime time.Duration
			var workedToday time.Duration
			var target time.Duration
			if opts.useOvertime != "" {
				running, err := store.RunningSession(ctx)
				if err != nil {
					return err
				}
				if running == nil {
					return fmt.Errorf("no session is running; use `work start`")
				}
				overtime, workedToday, target, err = overtimeForEnd(ctx, store, *running, endedAt, opts.useOvertime)
				if err != nil {
					return err
				}
			}

			var session db.Session
			if overtime > 0 {
				session, err = store.EndRunningSessionWithOvertime(ctx, endedAt, note, overtime)
			} else {
				session, err = store.EndRunningSession(ctx, endedAt, note)
			}
			if errors.Is(err, db.ErrNoRunningSession) {
				return fmt.Errorf("no session is running; use `work start`")
			}
			if err != nil {
				return err
			}
			lines := []string{
				badgeLine("ended", formatDateTime(session.EndedAt.Time)),
				line("", formatDuration(session.EndedAt.Time.Sub(session.StartedAt))),
			}
			if opts.useOvertime != "" {
				accounted := workedToday + overtime
				lines = append(lines,
					line("worked", formatDuration(workedToday)),
					line("overtime", formatDuration(overtime)),
					line("accounted", formatDuration(accounted)+" / "+formatDuration(target)),
				)
				if session.ProjectID.Valid {
					balance, err := store.ProjectBalanceAt(ctx, session.ProjectID.Int64, endedAt)
					if err != nil {
						return err
					}
					lines = append(lines, line("balance", formatSignedDuration(balance)))
				}
			}
			printBlock(lines...)
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.at, "at", "", "end time")
	cmd.Flags().StringVar(&opts.useOvertime, "use-overtime", "", "use overtime up to today's target, or an explicit duration")
	cmd.Flags().Lookup("use-overtime").NoOptDefVal = "auto"
	return cmd
}

func overtimeForEnd(ctx context.Context, store *db.Store, running db.Session, endedAt time.Time, input string) (time.Duration, time.Duration, time.Duration, error) {
	if !running.ProjectID.Valid || !running.ProjectName.Valid {
		return 0, 0, 0, fmt.Errorf("--use-overtime requires a project on the running session")
	}
	schedule, err := store.ProjectSchedule(ctx, running.ProjectID.Int64)
	if err != nil {
		return 0, 0, 0, err
	}
	if schedule == nil {
		return 0, 0, 0, fmt.Errorf("project %q has no weekly target; configure it with `work project set`", running.ProjectName.String)
	}
	workdays, err := parseWorkdays(schedule.Workdays)
	if err != nil {
		return 0, 0, 0, err
	}
	target := todayTarget(schedule.WeeklyTarget, endedAt, workdays)
	if target == 0 {
		return 0, 0, 0, fmt.Errorf("project %q has no target on %s", running.ProjectName.String, workdayLabel(endedAt.Weekday()))
	}

	start := dayStart(endedAt)
	end := start.AddDate(0, 0, 1)
	sessions, err := store.LogSessions(ctx, &start, &end, running.ProjectName.String)
	if err != nil {
		return 0, 0, 0, err
	}
	worked := totalSessionDuration(sessions, endedAt)
	alreadyUsed, err := store.ProjectOvertimeUsed(ctx, running.ProjectID.Int64, &start, &end)
	if err != nil {
		return 0, 0, 0, err
	}
	remaining := target - worked - alreadyUsed
	if remaining < 0 {
		remaining = 0
	}

	if input == "auto" {
		return remaining, worked, target, nil
	}
	overtime, err := timeparse.ParseWorkDuration(input)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid overtime duration: %w", err)
	}
	if overtime > remaining {
		return 0, 0, 0, fmt.Errorf("overtime duration %s exceeds today's remaining %s", formatDuration(overtime), formatDuration(remaining))
	}
	return overtime, worked, target, nil
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
				if err := printStatusWeek(ctx, store, today, nil, now); err != nil {
					return err
				}
				if len(today.Sessions) == 0 {
					last, err := store.LastSession(ctx)
					if err != nil {
						return err
					}
					if last != nil {
						printMuted(
							line("last", fmt.Sprintf("%s - %s  %s", formatDateTime(last.StartedAt), formatEnd(last), formatSessionDuration(*last, now))),
						)
					}
				}
				if err := printTodayNotes(ctx, store, today.Sessions); err != nil {
					return err
				}
				return nil
			}

			lines := []string{
				badgeLine("running", formatDuration(now.Sub(running.StartedAt))),
			}
			lines = appendTargetStatusLine(lines, target, today.Work, now, true)
			printBlock(lines...)
			printTodayProjects(today.Sessions, now)
			if err := printStatusWeek(ctx, store, today, running, now); err != nil {
				return err
			}
			return printTodayNotes(ctx, store, today.Sessions)
		},
	}
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
	nameWidth := projectDurationNameWidth(durations)
	for _, project := range durations {
		printLine(projectDurationLine(project, nameWidth))
	}
	fmt.Fprintln(out)
}

func projectDurationNameWidth(durations []projectDuration) int {
	width := 0
	for _, project := range durations {
		if len(project.Name) > width {
			width = len(project.Name)
		}
	}
	return width
}

func projectDurationLine(project projectDuration, nameWidth int) string {
	return lineWithLeftKeyWidth(project.Name, formatDuration(project.Duration), nameWidth)
}

func printStatusWeek(ctx context.Context, store *db.Store, today daySummaryInfo, running *db.Session, now time.Time) error {
	projects, err := statusWeekProjects(ctx, store, today, running)
	if err != nil {
		return err
	}
	var groups [][]string
	for _, project := range projects {
		schedule, err := store.ProjectSchedule(ctx, project.ID)
		if err != nil {
			return err
		}
		if schedule == nil {
			continue
		}
		info, err := loadProjectWeekInfo(ctx, store, project, now, now)
		if err != nil {
			return err
		}
		groups = append(groups, statusProjectWeekLines(info, now))
	}
	if len(groups) == 0 {
		return nil
	}

	printSection("weekly")
	for i, group := range groups {
		if i > 0 {
			fmt.Fprintln(out)
		}
		for _, text := range group {
			printLine(text)
		}
	}
	fmt.Fprintln(out)
	return nil
}

func statusWeekProjects(ctx context.Context, store *db.Store, today daySummaryInfo, running *db.Session) ([]db.Project, error) {
	seen := make(map[int64]bool)
	var projects []db.Project
	add := func(session db.Session) {
		if !session.ProjectID.Valid || seen[session.ProjectID.Int64] {
			return
		}
		seen[session.ProjectID.Int64] = true
		name := ""
		if session.ProjectName.Valid {
			name = session.ProjectName.String
		}
		projects = append(projects, db.Project{ID: session.ProjectID.Int64, Name: name})
	}
	if running != nil {
		add(*running)
	}
	for _, session := range today.Sessions {
		add(session)
	}
	if len(projects) > 0 {
		return projects, nil
	}

	active, err := store.ActiveProjects(ctx)
	if err != nil {
		return nil, err
	}
	var scheduled []db.Project
	for _, project := range active {
		schedule, err := store.ProjectSchedule(ctx, project.ID)
		if err != nil {
			return nil, err
		}
		if schedule != nil {
			scheduled = append(scheduled, project)
		}
	}
	if len(scheduled) == 1 {
		return scheduled, nil
	}
	return nil, nil
}

type projectNotes struct {
	Project string
	Events  []noteEvent
}

type noteEvent struct {
	At    time.Time
	Kind  string
	Body  string
	Order int
}

func todayNotes(ctx context.Context, store *db.Store, sessions []db.Session) ([]projectNotes, error) {
	var groups []projectNotes
	for _, session := range sessions {
		events := []noteEvent{
			{At: session.StartedAt, Kind: "start", Order: 0},
		}
		sessionNotes, err := store.NotesForSession(ctx, session.ID)
		if err != nil {
			return nil, err
		}
		for _, note := range sessionNotes {
			events = append(events, noteEvent{
				At:    note.CreatedAt,
				Kind:  note.Kind,
				Body:  note.Body,
				Order: 1,
			})
		}
		if session.EndedAt.Valid {
			events = append(events, noteEvent{
				At:    session.EndedAt.Time,
				Kind:  "stop",
				Order: 2,
			})
		}
		sort.SliceStable(events, func(i, j int) bool {
			if events[i].At.Equal(events[j].At) {
				return events[i].Order < events[j].Order
			}
			return events[i].At.Before(events[j].At)
		})
		project := sessionProjectTitle(session)
		if len(groups) > 0 && groups[len(groups)-1].Project == project {
			groups[len(groups)-1].Events = append(groups[len(groups)-1].Events, events...)
			continue
		}
		groups = append(groups, projectNotes{Project: project, Events: events})
	}
	return groups, nil
}

func printTodayNotes(ctx context.Context, store *db.Store, sessions []db.Session) error {
	groups, err := todayNotes(ctx, store, sessions)
	if err != nil {
		return err
	}
	if len(groups) == 0 {
		return nil
	}

	printSection("notes")
	showProjects := hasMultipleNoteProjects(groups)
	for i, group := range groups {
		if showProjects {
			if i > 0 {
				fmt.Fprintln(out)
			}
			printLine(line("", group.Project))
		}
		for _, event := range group.Events {
			printLine(noteEventLine(event))
		}
	}
	fmt.Fprintln(out)
	return nil
}

func hasMultipleNoteProjects(groups []projectNotes) bool {
	seen := make(map[string]bool)
	for _, group := range groups {
		seen[group.Project] = true
		if len(seen) > 1 {
			return true
		}
	}
	return false
}

func noteEventLine(event noteEvent) string {
	prefix := metaStyle.Render(formatClock(event.At)) + "  "
	if event.Body == "" {
		return prefix + boundaryNoteKind(event.Kind)
	}
	return prefix +
		noteKindStyle.Render(event.Kind) +
		"  " +
		valueStyle.Render(event.Body)
}

func sessionProjectTitle(session db.Session) string {
	if session.ProjectName.Valid && session.ProjectName.String != "" {
		return session.ProjectName.String
	}
	return "undefined"
}

func appendTodaySummaryLines(lines []string, summary daySummaryInfo, running *db.Session, includeToday bool) []string {
	if len(summary.Sessions) > 1 && summary.First.Valid && (running == nil || !summary.First.Time.Equal(running.StartedAt)) {
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
