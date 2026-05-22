package cli

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Rasalas/work-cli/internal/db"
)

func TestTodaySummaryTracksWorkPauseAndFirstStart(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "work.sqlite"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	base := time.Date(2026, 5, 22, 8, 0, 0, 0, time.Local)
	startAndEnd := func(start, end time.Time) {
		t.Helper()
		if _, err := store.StartSession(ctx, start, nil); err != nil {
			t.Fatalf("StartSession() error = %v", err)
		}
		if _, err := store.EndRunningSession(ctx, end, ""); err != nil {
			t.Fatalf("EndRunningSession() error = %v", err)
		}
	}
	startAndEnd(base, base.Add(2*time.Hour))
	startAndEnd(base.Add(2*time.Hour+30*time.Minute), base.Add(4*time.Hour))
	if _, err := store.StartSession(ctx, base.Add(4*time.Hour+45*time.Minute), nil); err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}

	summary, err := todaySummary(ctx, store, base.Add(5*time.Hour))
	if err != nil {
		t.Fatalf("todaySummary() error = %v", err)
	}

	if got, want := summary.Work, 3*time.Hour+45*time.Minute; got != want {
		t.Fatalf("Work = %s, want %s", got, want)
	}
	if got, want := summary.Paused, 75*time.Minute; got != want {
		t.Fatalf("Paused = %s, want %s", got, want)
	}
	if !summary.First.Valid || !summary.First.Time.Equal(base) {
		t.Fatalf("First = %v, want %v", summary.First, base)
	}
	if got, want := len(summary.Sessions), 3; got != want {
		t.Fatalf("len(Sessions) = %d, want %d", got, want)
	}
	if !summary.Sessions[0].StartedAt.Equal(base) {
		t.Fatalf("Sessions are not chronological: first = %v, want %v", summary.Sessions[0].StartedAt, base)
	}
}

func TestTodaySummarySessionsCanCollectWholeDayNotes(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "work.sqlite"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	base := time.Date(2026, 5, 22, 8, 0, 0, 0, time.Local)
	if _, err := store.StartSession(ctx, base, nil); err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	if _, err := store.AddNote(ctx, "do", "first session", base.Add(30*time.Minute)); err != nil {
		t.Fatalf("AddNote() error = %v", err)
	}
	if _, err := store.EndRunningSession(ctx, base.Add(time.Hour), ""); err != nil {
		t.Fatalf("EndRunningSession() error = %v", err)
	}
	if _, err := store.StartSession(ctx, base.Add(2*time.Hour), nil); err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	if _, err := store.AddNote(ctx, "doing", "current session", base.Add(2*time.Hour+30*time.Minute)); err != nil {
		t.Fatalf("AddNote() error = %v", err)
	}

	summary, err := todaySummary(ctx, store, base.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("todaySummary() error = %v", err)
	}

	var notes []db.Note
	for _, session := range summary.Sessions {
		sessionNotes, err := store.NotesForSession(ctx, session.ID)
		if err != nil {
			t.Fatalf("NotesForSession() error = %v", err)
		}
		notes = append(notes, sessionNotes...)
	}
	if got, want := len(notes), 2; got != want {
		t.Fatalf("len(notes) = %d, want %d", got, want)
	}
	if got, want := notes[0].Body, "first session"; got != want {
		t.Fatalf("notes[0].Body = %q, want %q", got, want)
	}
	if got, want := notes[1].Body, "current session"; got != want {
		t.Fatalf("notes[1].Body = %q, want %q", got, want)
	}
}

func TestLogSessionHeaderIncludesIDDurationProjectAndTime(t *testing.T) {
	start := time.Date(2026, 5, 22, 12, 38, 0, 0, time.Local)
	end := time.Date(2026, 5, 22, 14, 1, 0, 0, time.Local)
	session := db.Session{
		ID:          2,
		ProjectName: sql.NullString{String: "thk", Valid: true},
		StartedAt:   start,
		EndedAt:     sql.NullTime{Time: end, Valid: true},
	}

	got := logSessionHeader(session, end)
	want := "#2     1h 23m    thk  2026-05-22 12:38 - 2026-05-22 14:01"
	if got != want {
		t.Fatalf("logSessionHeader() = %q, want %q", got, want)
	}
}

func TestLogCommandPrintsOldestSessionFirst(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "work.sqlite")
	t.Setenv("WORK_DB", dbPath)
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))

	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, 5, 22, 8, 0, 0, 0, time.Local)
	if _, err := store.StartSession(ctx, base, nil); err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	if _, err := store.EndRunningSession(ctx, base.Add(time.Hour), ""); err != nil {
		t.Fatalf("EndRunningSession() error = %v", err)
	}
	if _, err := store.StartSession(ctx, base.Add(2*time.Hour), nil); err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	if _, err := store.EndRunningSession(ctx, base.Add(3*time.Hour), ""); err != nil {
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
	cmd.SetArgs([]string{"log"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	first := strings.Index(buf.String(), "#1")
	second := strings.Index(buf.String(), "#2")
	if first < 0 || second < 0 || first > second {
		t.Fatalf("log output order = %q, want #1 before #2", buf.String())
	}
}

func TestLogNoteLineAlignsTimeWithDuration(t *testing.T) {
	note := db.Note{
		Kind:      "do",
		Body:      "test more",
		CreatedAt: time.Date(2026, 5, 22, 12, 40, 0, 0, time.Local),
	}

	got := logNoteLine(2, note)
	want := "     " + noteLine(note)
	if got != want {
		t.Fatalf("logNoteLine() = %q, want %q", got, want)
	}
}
