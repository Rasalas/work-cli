package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Rasalas/work-cli/internal/db"
)

func TestEditCommandUsesEditedStartDateForBareEndTime(t *testing.T) {
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
	if _, err := store.EndRunningSession(ctx, base.Add(2*time.Hour), ""); err != nil {
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
	cmd.SetArgs([]string{
		"edit",
		strconv.FormatInt(session.ID, 10),
		"--start",
		"2026-05-20 0800",
		"--end",
		"1402",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	store, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	defer store.Close()
	updated, err := store.SessionByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("SessionByID() error = %v", err)
	}

	wantStart := time.Date(2026, 5, 20, 8, 0, 0, 0, time.Local)
	if !updated.StartedAt.Equal(wantStart) {
		t.Fatalf("StartedAt = %s, want %s", updated.StartedAt, wantStart)
	}
	wantEnd := time.Date(2026, 5, 20, 14, 2, 0, 0, time.Local)
	if !updated.EndedAt.Valid || !updated.EndedAt.Time.Equal(wantEnd) {
		t.Fatalf("EndedAt = %v, want %s", updated.EndedAt, wantEnd)
	}
}
