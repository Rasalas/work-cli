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

func TestEndAtTargetUsesOvertimeWhenEndingBeforeTarget(t *testing.T) {
	dbPath := useTempWorkDB(t)

	runWorkCommand(t, "project", "add", "someproject")
	runWorkCommand(t, "project", "set", "someproject", "--weekly", "25h", "--workdays", "mon,tue,wed,thu,fri")
	runWorkCommand(t, "project", "balance", "someproject", "--set", "80h", "--date", "2026-08-03")
	runWorkCommand(t, "start", "2026-08-04 08:00")

	output := runWorkCommand(t, "end", "--at", "2026-08-04 12:30", "--at-target")

	assertOutputContains(t, output, []string{
		"2026-08-04 13:00",
		"worked",
		"4h 30m",
		"overtime",
		"30m",
		"accounted",
		"5h / 5h",
		"balance",
		"+79h 30m",
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
	wantActualEnd := time.Date(2026, 8, 4, 12, 30, 0, 0, time.Local)
	if last == nil || !last.EndedAt.Valid || !last.EndedAt.Time.Equal(wantActualEnd) {
		t.Fatalf("last ended session = %#v, want actual end %s", last, wantActualEnd)
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

func TestEndAtTargetUsesOvertimeForAllRemainingWork(t *testing.T) {
	dbPath := useTempWorkDB(t)

	runWorkCommand(t, "project", "add", "someproject")
	runWorkCommand(t, "project", "set", "someproject", "--weekly", "25h", "--workdays", "mon,tue,wed,thu,fri")
	runWorkCommand(t, "start", "2026-08-04 08:00")

	output := runWorkCommand(t, "end", "--at", "2026-08-04 11:00", "--at-target")
	assertOutputContains(t, output, []string{
		"2026-08-04 13:00",
		"stopped",
		"2026-08-04 11:00",
		"worked",
		"3h",
		"overtime",
		"2h",
		"accounted",
		"5h / 5h",
	})

	store, openErr := db.Open(dbPath)
	if openErr != nil {
		t.Fatalf("db.Open() error = %v", openErr)
	}
	defer store.Close()
	running, queryErr := store.RunningSession(context.Background())
	if queryErr != nil {
		t.Fatalf("RunningSession() error = %v", queryErr)
	}
	if running != nil {
		t.Fatalf("RunningSession() = %#v, want nil", running)
	}
}

func TestEndAtTargetRejectsUseOvertimeCombination(t *testing.T) {
	for _, overtimeFlag := range []string{"--use-overtime", "--use-overtime="} {
		t.Run(overtimeFlag, func(t *testing.T) {
			useTempWorkDB(t)

			err := runWorkCommandError("end", "--at-target", overtimeFlag)
			if err == nil || err.Error() != "use --at-target to finish at today's target, or --use-overtime=<duration> to use a specific amount" {
				t.Fatalf("error = %v, want mutually exclusive flags error", err)
			}
		})
	}
}
