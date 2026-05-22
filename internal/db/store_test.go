package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestUpdateSessionChangesTimesAndProject(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	project, err := store.AddProject(ctx, "huntreport")
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}

	base := time.Date(2026, 5, 21, 8, 0, 0, 0, time.Local)
	session, err := store.StartSession(ctx, base, nil)
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	if _, err := store.EndRunningSession(ctx, base.Add(2*time.Hour), ""); err != nil {
		t.Fatalf("EndRunningSession() error = %v", err)
	}

	startedAt := base.Add(30 * time.Minute)
	endedAt := base.Add(3 * time.Hour)
	updated, err := store.UpdateSession(ctx, session.ID, SessionUpdate{
		StartedAt: &startedAt,
		EndedAt:   &endedAt,
		ProjectID: &project.ID,
	})
	if err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}

	if !updated.StartedAt.Equal(startedAt) {
		t.Fatalf("StartedAt = %s, want %s", updated.StartedAt, startedAt)
	}
	if !updated.EndedAt.Valid || !updated.EndedAt.Time.Equal(endedAt) {
		t.Fatalf("EndedAt = %v, want %s", updated.EndedAt, endedAt)
	}
	if !updated.ProjectID.Valid || updated.ProjectID.Int64 != project.ID {
		t.Fatalf("ProjectID = %v, want %d", updated.ProjectID, project.ID)
	}
}

func TestUpdateSessionRejectsEndBeforeStart(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 21, 8, 0, 0, 0, time.Local)
	session, err := store.StartSession(ctx, base, nil)
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}

	endedAt := base.Add(-time.Hour)
	if _, err := store.UpdateSession(ctx, session.ID, SessionUpdate{EndedAt: &endedAt}); err == nil {
		t.Fatal("UpdateSession() error = nil, want end-before-start error")
	}
}

func TestDeleteSessionRemovesSessionAndNotes(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 21, 8, 0, 0, 0, time.Local)
	session, err := store.StartSession(ctx, base, nil)
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	if _, err := store.AddNote(ctx, "do", "remove me", base.Add(15*time.Minute)); err != nil {
		t.Fatalf("AddNote() error = %v", err)
	}
	if _, err := store.EndRunningSession(ctx, base.Add(time.Hour), ""); err != nil {
		t.Fatalf("EndRunningSession() error = %v", err)
	}

	deleted, err := store.DeleteSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("DeleteSession() error = %v", err)
	}
	if deleted.ID != session.ID {
		t.Fatalf("deleted.ID = %d, want %d", deleted.ID, session.ID)
	}

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

func newStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "work.sqlite"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	return store
}
