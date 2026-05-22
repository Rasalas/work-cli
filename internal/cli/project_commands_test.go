package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rasalas/work-cli/internal/db"
)

func TestResolveNamedProjectUsesExactProject(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	project := addProject(t, store, "huntreport")

	got, err := resolveNamedProject(ctx, store, "huntreport")
	if err != nil {
		t.Fatalf("resolveNamedProject() error = %v", err)
	}
	if got.ID != project.ID {
		t.Fatalf("project ID = %d, want %d", got.ID, project.ID)
	}
}

func TestResolveNamedProjectUsesSingleFuzzyMatch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	project := addProject(t, store, "huntreport")

	got, err := resolveNamedProject(ctx, store, "hr")
	if err != nil {
		t.Fatalf("resolveNamedProject() error = %v", err)
	}
	if got.ID != project.ID {
		t.Fatalf("project ID = %d, want %d", got.ID, project.ID)
	}
}

func TestResolveNamedProjectRejectsAmbiguousFuzzyMatch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	addProject(t, store, "huntreport")
	addProject(t, store, "hire review")

	_, err := resolveNamedProject(ctx, store, "hr")
	if err == nil {
		t.Fatal("resolveNamedProject() error = nil, want ambiguous match error")
	}
	if !strings.Contains(err.Error(), "matches multiple projects") {
		t.Fatalf("error = %q, want multiple matches", err)
	}
}

func TestResolveNamedProjectRejectsUnknownProject(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	addProject(t, store, "huntreport")

	_, err := resolveNamedProject(ctx, store, "zzz")
	if err == nil {
		t.Fatal("resolveNamedProject() error = nil, want not found error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %q, want not found", err)
	}
}

func newTestStore(t *testing.T) *db.Store {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "work.sqlite"))
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("store.Close() error = %v", err)
		}
	})
	return store
}

func addProject(t *testing.T, store *db.Store, name string) db.Project {
	t.Helper()
	project, err := store.AddProject(context.Background(), name)
	if err != nil {
		t.Fatalf("AddProject(%q) error = %v", name, err)
	}
	return project
}
