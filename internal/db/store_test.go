package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
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

func TestAddNoteToSessionAddsNoteToEndedSession(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 25, 8, 0, 0, 0, time.Local)
	session, err := store.StartSession(ctx, base, nil)
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	end := base.Add(2 * time.Hour)
	if _, err := store.EndRunningSession(ctx, end, ""); err != nil {
		t.Fatalf("EndRunningSession() error = %v", err)
	}

	note, err := store.AddNoteToSession(ctx, session.ID, "done", "holiday support", end)
	if err != nil {
		t.Fatalf("AddNoteToSession() error = %v", err)
	}
	if note.SessionID != session.ID {
		t.Fatalf("note.SessionID = %d, want %d", note.SessionID, session.ID)
	}

	notes, err := store.NotesForSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("NotesForSession() error = %v", err)
	}
	if got, want := len(notes), 1; got != want {
		t.Fatalf("len(notes) = %d, want %d", got, want)
	}
	if got, want := notes[0].Body, "holiday support"; got != want {
		t.Fatalf("notes[0].Body = %q, want %q", got, want)
	}
}

func TestLastEndedSessionUsesLatestEndTime(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
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
	if _, err := store.EndRunningSession(ctx, base.Add(3*time.Hour), ""); err != nil {
		t.Fatalf("EndRunningSession() error = %v", err)
	}
	if _, err := store.StartSession(ctx, base.Add(4*time.Hour), nil); err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}

	last, err := store.LastEndedSession(ctx)
	if err != nil {
		t.Fatalf("LastEndedSession() error = %v", err)
	}
	if last == nil {
		t.Fatal("LastEndedSession() = nil, want session")
	}
	if last.ID != second.ID {
		t.Fatalf("LastEndedSession().ID = %d, want %d; first was %d", last.ID, second.ID, first.ID)
	}
}

func TestOvertimeUsageIsNotDeductedTwiceWhenWeekCompletes(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	project, err := store.AddProject(ctx, "someproject")
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	if err := store.SetProjectSchedule(ctx, project.ID, 20*time.Hour, "mon,tue,thu,fri"); err != nil {
		t.Fatalf("SetProjectSchedule() error = %v", err)
	}
	monday := time.Date(2026, 6, 8, 8, 0, 0, 0, time.Local)
	if err := store.SetProjectBalance(ctx, project.ID, monday, 80*time.Hour); err != nil {
		t.Fatalf("SetProjectBalance() error = %v", err)
	}

	if _, err := store.StartSession(ctx, monday, &project.ID); err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	if _, err := store.EndRunningSessionWithOvertime(ctx, monday.Add(3*time.Hour), "", 2*time.Hour); err != nil {
		t.Fatalf("EndRunningSessionWithOvertime() error = %v", err)
	}
	for _, day := range []time.Time{
		monday.AddDate(0, 0, 1),
		monday.AddDate(0, 0, 3),
		monday.AddDate(0, 0, 4),
	} {
		if _, err := store.StartSession(ctx, day, &project.ID); err != nil {
			t.Fatalf("StartSession() error = %v", err)
		}
		if _, err := store.EndRunningSession(ctx, day.Add(5*time.Hour), ""); err != nil {
			t.Fatalf("EndRunningSession() error = %v", err)
		}
	}

	balance, err := store.ProjectBalanceAt(ctx, project.ID, monday.AddDate(0, 0, 7))
	if err != nil {
		t.Fatalf("ProjectBalanceAt() error = %v", err)
	}
	if got, want := balance, 78*time.Hour; got != want {
		t.Fatalf("ProjectBalanceAt() = %s, want %s", got, want)
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

func TestOpenSetsUserVersionToLatestMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "work.sqlite")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	var version int
	if err := store.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("PRAGMA user_version error = %v", err)
	}
	if version != len(migrations) {
		t.Fatalf("user_version = %d, want %d", version, len(migrations))
	}
}

func TestOpenIsIdempotentAndDoesNotBackUpMigratedDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "work.sqlite")

	store, err := Open(path)
	if err != nil {
		t.Fatalf("first Open() error = %v", err)
	}
	if _, err := store.AddProject(context.Background(), "alpha"); err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	store, err = Open(path)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	defer store.Close()

	project, err := store.ProjectByName(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("ProjectByName() error = %v", err)
	}
	if project.Name != "alpha" {
		t.Fatalf("project name = %q, want alpha", project.Name)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".pre-migration-") {
			t.Fatalf("unexpected pre-migration backup %q for already migrated database", entry.Name())
		}
	}
}

func TestOpenBacksUpPreMigrationDatabase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "work.sqlite")

	// Simulate a legacy database: user_version is 0 (untracked), but the
	// schema already matches migration 1. Opening must create a backup
	// snapshot before applying migrations and must not lose data.
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	if _, err := legacy.Exec(migrations[0]); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}
	if _, err := legacy.Exec(`
INSERT INTO projects (name, created_at, updated_at, archived_at)
VALUES ('legacy', '2026-01-02T08:00:00Z', '2026-01-02T08:00:00Z', NULL);
`); err != nil {
		t.Fatalf("seed project row: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	var version int
	if err := store.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("PRAGMA user_version error = %v", err)
	}
	if version != len(migrations) {
		t.Fatalf("user_version = %d, want %d", version, len(migrations))
	}

	project, err := store.ProjectByName(context.Background(), "legacy")
	if err != nil {
		t.Fatalf("ProjectByName() error = %v", err)
	}
	if project.Name != "legacy" {
		t.Fatalf("project name = %q, want legacy", project.Name)
	}

	matches, err := filepath.Glob(filepath.Join(dir, "*.pre-migration-*.bak"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("found %d pre-migration backups, want 1 (%v)", len(matches), matches)
	}
}

func TestBackupWritesConsistentSnapshot(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	if _, err := store.AddProject(ctx, "backed-up"); err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}

	target := filepath.Join(t.TempDir(), "backup.sqlite")
	if err := store.Backup(ctx, target); err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	restored, err := Open(target)
	if err != nil {
		t.Fatalf("Open(backup) error = %v", err)
	}
	defer restored.Close()

	project, err := restored.ProjectByName(ctx, "backed-up")
	if err != nil {
		t.Fatalf("ProjectByName() on backup error = %v", err)
	}
	if project.Name != "backed-up" {
		t.Fatalf("project name = %q, want backed-up", project.Name)
	}
}

func TestBackupRejectsExistingTarget(t *testing.T) {
	store := newStore(t)
	target := filepath.Join(t.TempDir(), "backup.sqlite")
	if err := os.WriteFile(target, []byte(""), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := store.Backup(context.Background(), target); err == nil {
		t.Fatal("Backup() with existing target should fail")
	}
}
