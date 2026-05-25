package cli

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Rasalas/work-cli/internal/db"
)

func TestParseEndArgsUsesLeadingTimeArgument(t *testing.T) {
	base := time.Date(2026, 5, 22, 14, 30, 0, 0, time.Local)

	endedAt, note, err := parseEndArgs("", []string{"8", "wrapped", "up"}, base)
	if err != nil {
		t.Fatalf("parseEndArgs() error = %v", err)
	}
	if got, want := endedAt.Format("15:04"), "08:00"; got != want {
		t.Fatalf("endedAt = %s, want %s", got, want)
	}
	if note != "wrapped up" {
		t.Fatalf("note = %q, want %q", note, "wrapped up")
	}
}

func TestParseEndArgsUsesAtFlagBeforeLeadingArgument(t *testing.T) {
	base := time.Date(2026, 5, 22, 14, 30, 0, 0, time.Local)

	endedAt, note, err := parseEndArgs("1402", []string{"8", "wrapped", "up"}, base)
	if err != nil {
		t.Fatalf("parseEndArgs() error = %v", err)
	}
	if got, want := endedAt.Format("15:04"), "14:02"; got != want {
		t.Fatalf("endedAt = %s, want %s", got, want)
	}
	if note != "8 wrapped up" {
		t.Fatalf("note = %q, want %q", note, "8 wrapped up")
	}
}

func TestParseEndArgsKeepsNonTimeLeadingArgumentAsNote(t *testing.T) {
	base := time.Date(2026, 5, 22, 14, 30, 0, 0, time.Local)

	endedAt, note, err := parseEndArgs("", []string{"wrapped", "up"}, base)
	if err != nil {
		t.Fatalf("parseEndArgs() error = %v", err)
	}
	if !endedAt.Equal(base) {
		t.Fatalf("endedAt = %s, want %s", endedAt, base)
	}
	if note != "wrapped up" {
		t.Fatalf("note = %q, want %q", note, "wrapped up")
	}
}

func TestTodayProjectDurationsGroupsByProject(t *testing.T) {
	base := time.Date(2026, 5, 22, 8, 0, 0, 0, time.Local)
	sessions := []db.Session{
		{
			ProjectName: sql.NullString{String: "huntreport", Valid: true},
			StartedAt:   base,
			EndedAt:     sql.NullTime{Time: base.Add(time.Hour), Valid: true},
		},
		{
			ProjectName: sql.NullString{String: "admin", Valid: true},
			StartedAt:   base.Add(90 * time.Minute),
			EndedAt:     sql.NullTime{Time: base.Add(2 * time.Hour), Valid: true},
		},
		{
			ProjectName: sql.NullString{String: "huntreport", Valid: true},
			StartedAt:   base.Add(3 * time.Hour),
		},
	}

	durations := todayProjectDurations(sessions, base.Add(4*time.Hour))

	if got, want := len(durations), 2; got != want {
		t.Fatalf("len(durations) = %d, want %d", got, want)
	}
	if got, want := durations[0].Name, "huntreport"; got != want {
		t.Fatalf("durations[0].Name = %q, want %q", got, want)
	}
	if got, want := durations[0].Duration, 2*time.Hour; got != want {
		t.Fatalf("durations[0].Duration = %s, want %s", got, want)
	}
	if got, want := durations[1].Name, "admin"; got != want {
		t.Fatalf("durations[1].Name = %q, want %q", got, want)
	}
	if got, want := durations[1].Duration, 30*time.Minute; got != want {
		t.Fatalf("durations[1].Duration = %s, want %s", got, want)
	}
}

func TestPrintTodayNotesPrintsProjectTitleOnProjectChange(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 22, 8, 0, 0, 0, time.Local)
	huntreport := addProject(t, store, "huntreport")
	admin := addProject(t, store, "admin")

	startSessionWithProject(t, store, base, huntreport.ID)
	addNote(t, store, "do", "first hunt note", base.Add(15*time.Minute))
	endSession(t, store, base.Add(time.Hour))
	startSessionWithProject(t, store, base.Add(2*time.Hour), admin.ID)
	addNote(t, store, "do", "admin note", base.Add(2*time.Hour+15*time.Minute))
	endSession(t, store, base.Add(3*time.Hour))
	startSessionWithProject(t, store, base.Add(4*time.Hour), huntreport.ID)
	addNote(t, store, "do", "second hunt note", base.Add(4*time.Hour+15*time.Minute))

	summary, err := todaySummary(ctx, store, base.Add(5*time.Hour))
	if err != nil {
		t.Fatalf("todaySummary() error = %v", err)
	}
	var buf bytes.Buffer
	oldOut := out
	out = &buf
	t.Cleanup(func() {
		out = oldOut
	})

	if err := printTodayNotes(ctx, store, summary.Sessions); err != nil {
		t.Fatalf("printTodayNotes() error = %v", err)
	}

	output := buf.String()
	if got, want := strings.Count(output, "  huntreport  \n"), 2; got != want {
		t.Fatalf("huntreport title count = %d, want %d; output = %q", got, want, output)
	}
	if got, want := strings.Count(output, "  admin  \n"), 1; got != want {
		t.Fatalf("admin title count = %d, want %d; output = %q", got, want, output)
	}
	if !strings.Contains(output, "first hunt note  \n\n  admin") {
		t.Fatalf("output does not separate project titles with a blank line: %q", output)
	}
}

func TestSessionProjectTitleUsesUndefinedForSessionWithoutProject(t *testing.T) {
	if got, want := sessionProjectTitle(db.Session{}), "undefined"; got != want {
		t.Fatalf("sessionProjectTitle() = %q, want %q", got, want)
	}
}

func TestNoteCommandAddsNoteToLastEndedSession(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "work.sqlite")
	t.Setenv("WORK_DB", dbPath)
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))

	ctx := context.Background()
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	base := time.Date(2026, 5, 25, 8, 0, 0, 0, time.Local)
	first, err := store.StartSession(ctx, base, nil)
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	if _, err := store.EndRunningSession(ctx, base.Add(time.Hour), ""); err != nil {
		t.Fatalf("EndRunningSession() error = %v", err)
	}
	second, err := store.StartSession(ctx, base.Add(2*time.Hour), nil)
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	secondEnd := base.Add(3 * time.Hour)
	if _, err := store.EndRunningSession(ctx, secondEnd, ""); err != nil {
		t.Fatalf("EndRunningSession() error = %v", err)
	}
	if _, err := store.StartSession(ctx, base.Add(4*time.Hour), nil); err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	var buf bytes.Buffer
	oldOut := out
	out = &buf
	t.Cleanup(func() {
		out = oldOut
	})

	cmd := rootCmd()
	cmd.SetArgs([]string{"done", "--last", "holiday support"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	store, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	defer store.Close()
	firstNotes, err := store.NotesForSession(ctx, first.ID)
	if err != nil {
		t.Fatalf("NotesForSession(first) error = %v", err)
	}
	if len(firstNotes) != 0 {
		t.Fatalf("len(firstNotes) = %d, want 0", len(firstNotes))
	}
	secondNotes, err := store.NotesForSession(ctx, second.ID)
	if err != nil {
		t.Fatalf("NotesForSession(second) error = %v", err)
	}
	if got, want := len(secondNotes), 1; got != want {
		t.Fatalf("len(secondNotes) = %d, want %d", got, want)
	}
	if got, want := secondNotes[0].Body, "holiday support"; got != want {
		t.Fatalf("secondNotes[0].Body = %q, want %q", got, want)
	}
	if !secondNotes[0].CreatedAt.Equal(secondEnd) {
		t.Fatalf("CreatedAt = %s, want %s", secondNotes[0].CreatedAt, secondEnd)
	}
}

func TestNoteCommandAddsNoteToExplicitSessionAtStart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "work.sqlite")
	t.Setenv("WORK_DB", dbPath)
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))

	ctx := context.Background()
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	base := time.Date(2026, 5, 25, 8, 0, 0, 0, time.Local)
	session, err := store.StartSession(ctx, base, nil)
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	if _, err := store.EndRunningSession(ctx, base.Add(time.Hour), ""); err != nil {
		t.Fatalf("EndRunningSession() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	var buf bytes.Buffer
	oldOut := out
	out = &buf
	t.Cleanup(func() {
		out = oldOut
	})

	cmd := rootCmd()
	cmd.SetArgs([]string{"doing", "--session", strconv.FormatInt(session.ID, 10), "--at", "start", "holiday reason"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	store, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	defer store.Close()
	notes, err := store.NotesForSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("NotesForSession() error = %v", err)
	}
	if got, want := len(notes), 1; got != want {
		t.Fatalf("len(notes) = %d, want %d", got, want)
	}
	if !notes[0].CreatedAt.Equal(base) {
		t.Fatalf("CreatedAt = %s, want %s", notes[0].CreatedAt, base)
	}
}

func startSessionWithProject(t *testing.T, store *db.Store, start time.Time, projectID int64) {
	t.Helper()
	if _, err := store.StartSession(context.Background(), start, &projectID); err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
}

func addNote(t *testing.T, store *db.Store, kind, body string, createdAt time.Time) {
	t.Helper()
	if _, err := store.AddNote(context.Background(), kind, body, createdAt); err != nil {
		t.Fatalf("AddNote() error = %v", err)
	}
}

func endSession(t *testing.T, store *db.Store, end time.Time) {
	t.Helper()
	if _, err := store.EndRunningSession(context.Background(), end, ""); err != nil {
		t.Fatalf("EndRunningSession() error = %v", err)
	}
}
