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

func TestProjectBalanceUsesOnlyActiveProjectWhenNameIsOmitted(t *testing.T) {
	useTempWorkDB(t)

	runWorkCommand(t, "project", "add", "someproject")

	output := runWorkCommand(t, "project", "balance", "--set", "80h")

	assertOutputContains(t, output, []string{
		"someproject",
		"balance",
		"+80h",
	})
}

func TestProjectBalanceMissingProjectNameUsesActionableError(t *testing.T) {
	useTempWorkDB(t)

	err := runWorkCommandError("project", "balance")
	if err == nil {
		t.Fatal("work project balance error = nil, want missing project name")
	}
	if got := err.Error(); !strings.Contains(got, "missing project name") || !strings.Contains(got, "work project balance <projectname>") {
		t.Fatalf("error = %q, want actionable missing project name guidance", got)
	}
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
		"schedule",
		"Mon, Tue, Thu, Fri",
		"remaining",
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

func TestWeekCommandUsesOnlyActiveProjectWhenProjectIsOmitted(t *testing.T) {
	dbPath := useTempWorkDB(t)

	runWorkCommand(t, "project", "add", "someproject")
	runWorkCommand(t, "project", "set", "someproject", "--weekly", "20h", "--workdays", "mon,tue,thu,fri")

	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open() error = %v", err)
	}
	project, err := store.ProjectByName(context.Background(), "someproject")
	if err != nil {
		t.Fatalf("ProjectByName() error = %v", err)
	}
	monday := time.Date(2026, 6, 8, 8, 0, 0, 0, time.Local)
	addEndedProjectSessionForWeeklyTest(t, store, project.ID, monday, monday.Add(5*time.Hour))
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	output := runWorkCommand(t, "week", "--date", "2026-06-10")

	assertOutputContains(t, output, []string{
		"someproject",
		"5h / 20h",
	})
}

func TestStatusProjectWeekLinesShowTodayOvertimeAndRemainingWorkdays(t *testing.T) {
	now := time.Date(2026, 6, 10, 17, 31, 0, 0, time.Local)
	info := projectWeekInfo{
		Project: db.Project{Name: "thk"},
		Schedule: &db.ProjectSchedule{
			WeeklyTarget: 20 * time.Hour,
		},
		Worked:            11*time.Hour + 16*time.Minute,
		Left:              8*time.Hour + 44*time.Minute,
		TodayWorked:       time.Hour + 6*time.Minute,
		TodayTarget:       0,
		TodayOvertime:     time.Hour + 6*time.Minute,
		RemainingWorkdays: []time.Time{time.Date(2026, 6, 11, 0, 0, 0, 0, time.Local), time.Date(2026, 6, 12, 0, 0, 0, 0, time.Local)},
		Balance:           78 * time.Hour,
		Projected:         69*time.Hour + 16*time.Minute,
	}

	output := strings.Join(statusProjectWeekLines(info, now), "\n")

	assertOutputContains(t, output, []string{
		"thk",
		"week",
		"11h 16m / 20h",
		"today",
		"1h 6m / 0m",
		"overtime",
		"+1h 6m",
		"remaining",
		"Thu, Fri",
		"per day",
		"4h 22m",
		"balance",
		"+78h",
		"projected",
		"+69h 16m",
	})
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

func runWorkCommandError(args ...string) error {
	cmd := rootCmd()
	cmd.SetArgs(args)
	return cmd.Execute()
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
