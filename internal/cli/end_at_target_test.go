package cli

import (
	"context"
	"testing"
	"time"

	"github.com/Rasalas/work-cli/internal/db"
)

func TestEndAtTargetUsesOnlyRemainingDailyWork(t *testing.T) {
	dbPath := useTempWorkDB(t)

	runWorkCommand(t, "project", "add", "someproject")
	runWorkCommand(t, "project", "set", "someproject", "--weekly", "25h", "--workdays", "mon,tue,wed,thu,fri")
	runWorkCommand(t, "start", "2026-08-04 08:00")
	runWorkCommand(t, "end", "--at", "2026-08-04 10:00")
	runWorkCommand(t, "start", "2026-08-04 10:30")

	output := runWorkCommand(t, "end", "--at", "2026-08-04 15:30", "--at-target")

	assertOutputContains(t, output, []string{
		"2026-08-04 13:30",
		"worked",
		"5h",
		"accounted",
		"5h / 5h",
		"ignored",
		"2h",
	})

	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	defer store.Close()
	last, err := store.LastEndedSession(context.Background())
	if err != nil {
		t.Fatalf("LastEndedSession() error = %v", err)
	}
	want := time.Date(2026, 8, 4, 13, 30, 0, 0, time.Local)
	if last == nil || !last.EndedAt.Valid || !last.EndedAt.Time.Equal(want) {
		t.Fatalf("last ended session = %#v, want end %s", last, want)
	}
}

func TestEndAtTargetIncludesOvertimeAlreadyUsedToday(t *testing.T) {
	dbPath := useTempWorkDB(t)

	runWorkCommand(t, "project", "add", "someproject")
	runWorkCommand(t, "project", "set", "someproject", "--weekly", "25h", "--workdays", "mon,tue,wed,thu,fri")
	runWorkCommand(t, "start", "2026-08-04 08:00")
	runWorkCommand(t, "end", "--at", "2026-08-04 10:00", "--use-overtime=1h")
	runWorkCommand(t, "start", "2026-08-04 10:30")

	output := runWorkCommand(t, "end", "--at", "2026-08-04 15:30", "--at-target")

	assertOutputContains(t, output, []string{
		"2026-08-04 12:30",
		"worked",
		"4h",
		"overtime",
		"1h",
		"accounted",
		"5h / 5h",
		"ignored",
		"3h",
	})

	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	defer store.Close()
	last, err := store.LastEndedSession(context.Background())
	if err != nil {
		t.Fatalf("LastEndedSession() error = %v", err)
	}
	want := time.Date(2026, 8, 4, 12, 30, 0, 0, time.Local)
	if last == nil || !last.EndedAt.Valid || !last.EndedAt.Time.Equal(want) {
		t.Fatalf("last ended session = %#v, want end %s", last, want)
	}
}

func TestEndAtTargetKeepsSessionRunningUntilTargetIsReached(t *testing.T) {
	dbPath := useTempWorkDB(t)

	runWorkCommand(t, "project", "add", "someproject")
	runWorkCommand(t, "project", "set", "someproject", "--weekly", "25h", "--workdays", "mon,tue,wed,thu,fri")
	runWorkCommand(t, "start", "2026-08-04 08:00")

	err := runWorkCommandError("end", "--at", "2026-08-04 11:00", "--at-target")
	if err == nil {
		t.Fatal("work end --at-target error = nil, want target-not-reached error")
	}
	if got := err.Error(); got != "today's target will be reached at 13:00; end normally or try --at-target later" {
		t.Fatalf("error = %q", got)
	}

	store, openErr := db.Open(dbPath)
	if openErr != nil {
		t.Fatalf("db.Open() error = %v", openErr)
	}
	defer store.Close()
	running, queryErr := store.RunningSession(context.Background())
	if queryErr != nil {
		t.Fatalf("RunningSession() error = %v", queryErr)
	}
	if running == nil {
		t.Fatal("RunningSession() = nil, want session to remain running")
	}
}

func TestEndAtTargetRejectsUseOvertimeCombination(t *testing.T) {
	for _, overtimeFlag := range []string{"--use-overtime", "--use-overtime="} {
		t.Run(overtimeFlag, func(t *testing.T) {
			useTempWorkDB(t)

			err := runWorkCommandError("end", "--at-target", overtimeFlag)
			if err == nil || err.Error() != "use either --at-target or --use-overtime" {
				t.Fatalf("error = %v, want mutually exclusive flags error", err)
			}
		})
	}
}
