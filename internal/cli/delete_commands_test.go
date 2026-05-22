package cli

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Rasalas/work-cli/internal/db"
)

func TestDeleteCommandDeletesSession(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "work.sqlite")
	t.Setenv("WORK_DB", dbPath)
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))

	ctx := context.Background()
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	base := time.Date(2026, 5, 21, 8, 0, 0, 0, time.Local)
	session, err := store.StartSession(ctx, base, nil)
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	if _, err := store.AddNote(ctx, "do", "ship delete command", base.Add(15*time.Minute)); err != nil {
		t.Fatalf("AddNote() error = %v", err)
	}
	if _, err := store.EndRunningSession(ctx, base.Add(time.Hour), ""); err != nil {
		t.Fatalf("EndRunningSession() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	var buf bytes.Buffer
	oldOut := out
	oldConfirm := confirmWithContent
	out = &buf
	confirmWithContent = func(title, content, question string) (bool, error) {
		wantTitle := "DELETE Session #" + strconv.FormatInt(session.ID, 10)
		if title != wantTitle {
			t.Fatalf("title = %q, want %q", title, wantTitle)
		}
		if question != "Delete this session?" {
			t.Fatalf("question = %q, want %q", question, "Delete this session?")
		}
		if strings.Contains(content, "Session #"+strconv.FormatInt(session.ID, 10)) {
			t.Fatalf("content = %q, want session id only in title", content)
		}
		if !strings.Contains(content, "ship delete command") {
			t.Fatalf("content = %q, want note body", content)
		}
		return true, nil
	}
	t.Cleanup(func() {
		out = oldOut
		confirmWithContent = oldConfirm
	})

	cmd := rootCmd()
	cmd.SetArgs([]string{"delete", strconv.FormatInt(session.ID, 10)})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	store, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	defer store.Close()
	if _, err := store.SessionByID(ctx, session.ID); err != sql.ErrNoRows {
		t.Fatalf("SessionByID() error = %v, want sql.ErrNoRows", err)
	}
	notes, err := store.NotesForSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("NotesForSession() error = %v", err)
	}
	if len(notes) != 0 {
		t.Fatalf("len(notes) = %d, want 0", len(notes))
	}
}

func TestDeleteCommandCancelsWithoutConfirmation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "work.sqlite")
	t.Setenv("WORK_DB", dbPath)
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))

	ctx := context.Background()
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	base := time.Date(2026, 5, 21, 8, 0, 0, 0, time.Local)
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
	oldConfirm := confirmWithContent
	out = &buf
	confirmWithContent = func(title, content, question string) (bool, error) {
		return false, nil
	}
	t.Cleanup(func() {
		out = oldOut
		confirmWithContent = oldConfirm
	})

	cmd := rootCmd()
	cmd.SetArgs([]string{"rm", strconv.FormatInt(session.ID, 10)})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	store, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	defer store.Close()
	if _, err := store.SessionByID(ctx, session.ID); err != nil {
		t.Fatalf("SessionByID() error = %v, want nil", err)
	}
}

func TestDeleteCommandReturnsConfirmationError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "work.sqlite")
	t.Setenv("WORK_DB", dbPath)
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))

	ctx := context.Background()
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	base := time.Date(2026, 5, 21, 8, 0, 0, 0, time.Local)
	session, err := store.StartSession(ctx, base, nil)
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	confirmErr := errors.New("confirm failed")
	oldConfirm := confirmWithContent
	confirmWithContent = func(title, content, question string) (bool, error) {
		return false, confirmErr
	}
	t.Cleanup(func() {
		confirmWithContent = oldConfirm
	})

	cmd := rootCmd()
	cmd.SetArgs([]string{"delete", strconv.FormatInt(session.ID, 10)})
	if err := cmd.Execute(); !errors.Is(err, confirmErr) {
		t.Fatalf("Execute() error = %v, want %v", err, confirmErr)
	}
}
