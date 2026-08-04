package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Rasalas/work-cli/internal/db"
)

func TestAbsenceAddStoresInclusiveRangeAndListsIt(t *testing.T) {
	dbPath := useTempWorkDB(t)

	runWorkCommand(t, "project", "add", "someproject")
	runWorkCommand(t, "project", "set", "someproject", "--weekly", "20h", "--workdays", "mon,tue,thu,fri")

	output := runWorkCommand(t, "absence", "add", "someproject", "--from", "2026-07-17", "--to", "2026-08-03", "--type", "vacation")
	assertOutputContains(t, output, []string{
		"absence",
		"someproject",
		"vacation",
		"2026-07-17",
		"2026-08-03",
	})

	listed := runWorkCommand(t, "absence", "list", "someproject")
	assertOutputContains(t, listed, []string{"vacation", "2026-07-17 - 2026-08-03"})

	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	defer store.Close()
	project, err := store.ProjectByName(context.Background(), "someproject")
	if err != nil {
		t.Fatalf("ProjectByName() error = %v", err)
	}
	from := time.Date(2026, 7, 13, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 8, 10, 0, 0, 0, 0, time.Local)
	reduction, err := store.ProjectAbsenceTargetReduction(context.Background(), project.ID, from, to, 20*time.Hour, "mon,tue,thu,fri")
	if err != nil {
		t.Fatalf("ProjectAbsenceTargetReduction() error = %v", err)
	}
	if got, want := reduction, 50*time.Hour; got != want {
		t.Fatalf("ProjectAbsenceTargetReduction() = %s, want %s", got, want)
	}
}

func TestAbsenceRejectsOverlappingRange(t *testing.T) {
	useTempWorkDB(t)

	runWorkCommand(t, "project", "add", "someproject")
	runWorkCommand(t, "absence", "add", "someproject", "--from", "2026-07-17", "--to", "2026-08-03")

	err := runWorkCommandError("absence", "add", "someproject", "--from", "2026-08-03", "--to", "2026-08-04")
	if err == nil || !strings.Contains(err.Error(), "already overlaps") {
		t.Fatalf("error = %v, want overlap error", err)
	}
}

func TestAbsenceReducesCompletedAndCurrentWeekTargets(t *testing.T) {
	dbPath := useTempWorkDB(t)

	runWorkCommand(t, "project", "add", "someproject")
	runWorkCommand(t, "project", "set", "someproject", "--weekly", "20h", "--workdays", "mon,tue,thu,fri")
	runWorkCommand(t, "project", "balance", "someproject", "--set", "78h", "--date", "2026-07-13")

	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	ctx := context.Background()
	project, err := store.ProjectByName(ctx, "someproject")
	if err != nil {
		t.Fatalf("ProjectByName() error = %v", err)
	}
	monday := time.Date(2026, 7, 13, 8, 0, 0, 0, time.Local)
	addEndedProjectSessionForWeeklyTest(t, store, project.ID, monday, monday.Add(5*time.Hour))
	tuesday := monday.AddDate(0, 0, 1)
	addEndedProjectSessionForWeeklyTest(t, store, project.ID, tuesday, tuesday.Add(5*time.Hour))
	thursday := monday.AddDate(0, 0, 3)
	addEndedProjectSessionForWeeklyTest(t, store, project.ID, thursday, thursday.Add(10*time.Hour))
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() error = %v", err)
	}

	runWorkCommand(t, "absence", "add", "someproject", "--from", "2026-07-17", "--to", "2026-08-03")

	store, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	defer store.Close()
	selected := time.Date(2026, 8, 4, 12, 0, 0, 0, time.Local)
	info, err := loadProjectWeekInfo(ctx, store, project, selected, selected)
	if err != nil {
		t.Fatalf("loadProjectWeekInfo() error = %v", err)
	}
	if got, want := info.Balance, 83*time.Hour; got != want {
		t.Fatalf("Balance = %s, want %s", got, want)
	}
	if got, want := info.Target, 15*time.Hour; got != want {
		t.Fatalf("Target = %s, want %s", got, want)
	}
	if got, want := info.Absence, 5*time.Hour; got != want {
		t.Fatalf("Absence = %s, want %s", got, want)
	}
	if got, want := info.TodayTarget, 5*time.Hour; got != want {
		t.Fatalf("TodayTarget = %s, want %s", got, want)
	}

	vacationMonday := time.Date(2026, 8, 3, 12, 0, 0, 0, time.Local)
	info, err = loadProjectWeekInfo(ctx, store, project, vacationMonday, vacationMonday)
	if err != nil {
		t.Fatalf("loadProjectWeekInfo(vacation) error = %v", err)
	}
	if got := info.TodayTarget; got != 0 {
		t.Fatalf("TodayTarget on vacation = %s, want 0", got)
	}
	if got, want := formatWeekdays(info.RemainingWorkdays), "Tue, Thu, Fri"; got != want {
		t.Fatalf("remaining workdays = %q, want %q", got, want)
	}
}
