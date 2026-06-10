package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Rasalas/work-cli/internal/db"
)

func TestProjectSetWeeklyTargetAndWorkdaysShowsInProjectList(t *testing.T) {
	useTempWorkDB(t)

	runWorkCommand(t, "project", "add", "someproject")
	runWorkCommand(t, "project", "set", "someproject", "--weekly", "20h", "--workdays", "mon,tue,thu,fri")

	output := runWorkCommand(t, "project", "list")

	assertOutputContains(t, output, []string{
		"someproject",
		"20h/week",
		"Mon, Tue, Thu, Fri",
	})
	if strings.Contains(output, "Wed") {
		t.Fatalf("project list includes Wednesday despite configured workdays: %q", output)
	}
}

func TestProjectBalanceSetShowsOvertimeBalance(t *testing.T) {
	useTempWorkDB(t)

	runWorkCommand(t, "project", "add", "someproject")
	output := runWorkCommand(t, "project", "balance", "someproject", "--set", "80h", "--date", "2026-06-03")

	assertOutputContains(t, output, []string{
		"someproject",
		"balance",
		"+80h",
	})

	output = runWorkCommand(t, "project", "balance", "someproject")
	assertOutputContains(t, output, []string{
		"someproject",
		"balance",
		"+80h",
	})
}

func TestWeekCommandShowsProjectTargetProgressAndPlannedOvertimeBurnDown(t *testing.T) {
	dbPath := useTempWorkDB(t)

	runWorkCommand(t, "project", "add", "someproject")
	runWorkCommand(t, "project", "set", "someproject", "--weekly", "20h", "--workdays", "mon,tue,thu,fri")
	runWorkCommand(t, "project", "balance", "someproject", "--set", "80h", "--date", "2026-06-03")

	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	ctx := context.Background()
	project, err := store.ProjectByName(ctx, "someproject")
	if err != nil {
		t.Fatalf("ProjectByName() error = %v", err)
	}
	otherProject, err := store.AddProject(ctx, "otherproject")
	if err != nil {
		t.Fatalf("AddProject() error = %v", err)
	}
	monday := time.Date(2026, 6, 8, 8, 0, 0, 0, time.Local)
	addEndedProjectSessionForWeeklyTest(t, store, project.ID, monday, monday.Add(5*time.Hour))
	tuesday := monday.AddDate(0, 0, 1)
	addEndedProjectSessionForWeeklyTest(t, store, project.ID, tuesday, tuesday.Add(5*time.Hour))
	addEndedProjectSessionForWeeklyTest(t, store, otherProject.ID, monday.Add(10*time.Hour), monday.Add(12*time.Hour))
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	output := runWorkCommand(t, "week", "--project", "someproject", "--date", "2026-06-10")

	assertOutputContains(t, output, []string{
		"someproject",
		"10h / 20h",
		"left",
		"10h",
		"workdays",
		"Thu, Fri",
		"per day",
		"5h",
		"deadline",
		"balance",
		"+80h",
		"projected",
		"+70h",
	})
	if strings.Contains(output, "otherproject") {
		t.Fatalf("week output includes another project's time: %q", output)
	}
}

func useTempWorkDB(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "work.sqlite")
	t.Setenv("WORK_DB", dbPath)
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	return dbPath
}

func runWorkCommand(t *testing.T, args ...string) string {
	t.Helper()

	var buf bytes.Buffer
	oldOut := out
	out = &buf
	defer func() {
		out = oldOut
	}()

	cmd := rootCmd()
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("work %s failed: %v\noutput:\n%s", strings.Join(args, " "), err, buf.String())
	}
	return buf.String()
}

func addEndedProjectSessionForWeeklyTest(t *testing.T, store *db.Store, projectID int64, start, end time.Time) {
	t.Helper()
	if _, err := store.StartSession(context.Background(), start, &projectID); err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	if _, err := store.EndRunningSession(context.Background(), end, ""); err != nil {
		t.Fatalf("EndRunningSession() error = %v", err)
	}
}

func assertOutputContains(t *testing.T, output string, wants []string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q: %q", want, output)
		}
	}
}
