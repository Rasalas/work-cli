package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Rasalas/work-cli/internal/db"
)

func TestNoteListShowsNoteIDs(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "work.sqlite")
	t.Setenv("WORK_DB", dbPath)
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))

	ctx := context.Background()
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	base := time.Date(2026, 6, 11, 8, 6, 0, 0, time.Local)
	session, err := store.StartSession(ctx, base, nil)
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	note, err := store.AddNote(ctx, "doing", "marker detection/replacement", base.Add(6*time.Hour+24*time.Minute))
	if err != nil {
		t.Fatalf("AddNote() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	output := runNoteCommand(t, "note", "list", strconv.FormatInt(session.ID, 10))

	for _, want := range []string{
		strconv.FormatInt(note.ID, 10),
		"14:30",
		"doing",
		"marker detection/replacement",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q: %q", want, output)
		}
	}
}

func TestNoteLsAliasShowsNoteIDs(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "work.sqlite")
	t.Setenv("WORK_DB", dbPath)
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))

	ctx := context.Background()
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	base := time.Date(2026, 6, 11, 8, 6, 0, 0, time.Local)
	session, err := store.StartSession(ctx, base, nil)
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	note, err := store.AddNote(ctx, "doing", "marker detection/replacement", base.Add(6*time.Hour+24*time.Minute))
	if err != nil {
		t.Fatalf("AddNote() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	output := runNoteCommand(t, "note", "ls", strconv.FormatInt(session.ID, 10))

	if !strings.Contains(output, strconv.FormatInt(note.ID, 10)) {
		t.Fatalf("output missing note id: %q", output)
	}
}

func TestNoteEditUpdatesTime(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "work.sqlite")
	t.Setenv("WORK_DB", dbPath)
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))

	ctx := context.Background()
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	base := time.Date(2026, 6, 11, 8, 6, 0, 0, time.Local)
	if _, err := store.StartSession(ctx, base, nil); err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	note, err := store.AddNote(ctx, "doing", "marker detection/replacement", base.Add(6*time.Hour+24*time.Minute))
	if err != nil {
		t.Fatalf("AddNote() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	output := runNoteCommand(t, "note", "edit", strconv.FormatInt(note.ID, 10), "--at", "1330")
	if !strings.Contains(output, "13:30") {
		t.Fatalf("output missing edited time: %q", output)
	}

	store, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	defer store.Close()
	updated, err := store.NoteByID(ctx, note.ID)
	if err != nil {
		t.Fatalf("NoteByID() error = %v", err)
	}
	if got, want := updated.CreatedAt.Format("15:04"), "13:30"; got != want {
		t.Fatalf("CreatedAt = %s, want %s", got, want)
	}
}

func runNoteCommand(t *testing.T, args ...string) string {
	t.Helper()
	var buf bytes.Buffer
	oldOut := out
	out = &buf
	t.Cleanup(func() {
		out = oldOut
	})

	cmd := rootCmd()
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	return buf.String()
}
