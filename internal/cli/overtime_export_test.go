package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Rasalas/work-cli/internal/db"
)

func TestEndUseOvertimeFillsPlannedDailyTarget(t *testing.T) {
	dbPath := useTempWorkDB(t)

	runWorkCommand(t, "project", "add", "someproject")
	runWorkCommand(t, "project", "set", "someproject",
		"--weekly", "25h",
		"--workdays", "mon,tue,wed,thu,fri",
		"--report-start", "0800",
		"--report-end", "1300",
	)
	runWorkCommand(t, "project", "balance", "someproject", "--set", "80h", "--date", "2026-08-03")
	runWorkCommand(t, "start", "2026-08-04 08:00")

	output := runWorkCommand(t, "end", "--at", "2026-08-04 11:30", "--use-overtime")

	assertOutputContains(t, output, []string{
		"ended",
		"2026-08-04 13:00",
		"stopped",
		"2026-08-04 11:30",
		"worked",
		"3h 30m",
		"overtime",
		"1h 30m",
		"accounted",
		"5h / 5h",
		"balance",
		"+78h 30m",
	})

	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	defer store.Close()
	project, err := store.ProjectByName(context.Background(), "someproject")
	if err != nil {
		t.Fatalf("ProjectByName() error = %v", err)
	}
	from := time.Date(2026, 8, 4, 0, 0, 0, 0, time.Local)
	to := from.AddDate(0, 0, 1)
	used, err := store.ProjectOvertimeUsed(context.Background(), project.ID, &from, &to)
	if err != nil {
		t.Fatalf("ProjectOvertimeUsed() error = %v", err)
	}
	if got, want := used, 90*time.Minute; got != want {
		t.Fatalf("ProjectOvertimeUsed() = %s, want %s", got, want)
	}
}

func TestEndUseOvertimeAcceptsExplicitDuration(t *testing.T) {
	useTempWorkDB(t)

	runWorkCommand(t, "project", "add", "someproject")
	runWorkCommand(t, "project", "set", "someproject",
		"--weekly", "25h",
		"--workdays", "mon,tue,wed,thu,fri",
		"--report-start", "0800",
		"--report-end", "1300",
	)
	runWorkCommand(t, "start", "2026-08-04 08:00")

	output := runWorkCommand(t, "end", "--at", "2026-08-04 11:30", "--use-overtime=1h")

	assertOutputContains(t, output, []string{
		"ended",
		"2026-08-04 12:30",
		"stopped",
		"2026-08-04 11:30",
		"worked",
		"3h 30m",
		"overtime",
		"1h",
		"accounted",
		"4h 30m / 5h",
	})

	exported := runWorkCommand(t, "export", "someproject", "--date", "2026-08-04", "--show-overtime")
	want := strings.Join([]string{
		"date,start,end,type,duration",
		"2026-08-04,08:00,12:00,work,04:00",
		"2026-08-04,12:00,13:00,overtime_use,01:00",
		"",
	}, "\n")
	if exported != want {
		t.Fatalf("export = %q, want %q", exported, want)
	}
}

func TestStatusShowsPlannedStopAfterUsingOvertime(t *testing.T) {
	useTempWorkDB(t)

	runWorkCommand(t, "project", "add", "someproject")
	runWorkCommand(t, "project", "set", "someproject", "--weekly", "25h", "--workdays", "mon,tue,wed,thu,fri")
	runWorkCommand(t, "start", "2026-08-04 08:00")
	runWorkCommand(t, "end", "--at", "2026-08-04 12:30", "--at-target")

	store, err := openStore()
	if err != nil {
		t.Fatalf("openStore() error = %v", err)
	}
	defer store.Close()
	start := time.Date(2026, 8, 4, 0, 0, 0, 0, time.Local)
	end := start.AddDate(0, 0, 1)
	sessions, err := store.LogSessions(context.Background(), &start, &end, "someproject")
	if err != nil {
		t.Fatalf("LogSessions() error = %v", err)
	}

	groups, err := todayNotes(context.Background(), store, sessions)
	if err != nil {
		t.Fatalf("todayNotes() error = %v", err)
	}
	if len(groups) != 1 || len(groups[0].Events) != 2 {
		t.Fatalf("todayNotes() = %#v, want one start and stop", groups)
	}
	stop := groups[0].Events[1]
	want := time.Date(2026, 8, 4, 13, 0, 0, 0, time.Local)
	if stop.Kind != "stop" || !stop.At.Equal(want) {
		t.Fatalf("stop = %#v, want %s", stop, want)
	}
}

func TestExportKeepsSchemaAndSplitsOvertimeWhenRequested(t *testing.T) {
	useTempWorkDB(t)

	runWorkCommand(t, "project", "add", "someproject")
	runWorkCommand(t, "project", "set", "someproject",
		"--weekly", "25h",
		"--workdays", "mon,tue,wed,thu,fri",
		"--report-start", "0800",
		"--report-end", "1300",
	)
	runWorkCommand(t, "start", "2026-08-04 08:00")
	runWorkCommand(t, "end", "--at", "2026-08-04 11:30", "--use-overtime")

	compact := runWorkCommand(t, "export", "someproject", "--date", "2026-08-04")
	wantCompact := strings.Join([]string{
		"date,start,end,type,duration",
		"2026-08-04,08:00,13:00,work,05:00",
		"",
	}, "\n")
	if compact != wantCompact {
		t.Fatalf("compact export = %q, want %q", compact, wantCompact)
	}

	detailed := runWorkCommand(t, "export", "someproject", "--date", "2026-08-04", "--show-overtime")
	wantDetailed := strings.Join([]string{
		"date,start,end,type,duration",
		"2026-08-04,08:00,11:30,work,03:30",
		"2026-08-04,11:30,13:00,overtime_use,01:30",
		"",
	}, "\n")
	if detailed != wantDetailed {
		t.Fatalf("detailed export = %q, want %q", detailed, wantDetailed)
	}
}

func TestProjectSetCanConfigureReportingWindowWithoutChangingSchedule(t *testing.T) {
	useTempWorkDB(t)

	runWorkCommand(t, "project", "add", "someproject")
	runWorkCommand(t, "project", "set", "someproject", "--weekly", "25h", "--workdays", "mon,tue,wed,thu,fri")
	output := runWorkCommand(t, "project", "set", "someproject", "--report-start", "8", "--report-end", "13")

	assertOutputContains(t, output, []string{
		"25h/week",
		"Mon, Tue, Wed, Thu, Fri",
		"08:00 - 13:00",
	})
}
