package cli

import (
	"context"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/Rasalas/work-cli/internal/db"
)

func internalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "internal",
		Short:  "Internal helper commands",
		Hidden: true,
	}
	cmd.AddCommand(internalDemoStatusCmd())
	return cmd
}

func internalDemoStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "demo-status",
		Short:  "Print deterministic demo status output",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printDemoStatus()
		},
	}
}

func printDemoStatus() error {
	path, err := demoStorePath()
	if err != nil {
		return err
	}
	defer os.Remove(path)

	store, err := db.Open(path)
	if err != nil {
		return err
	}
	defer store.Close()

	base := time.Now()
	now := time.Date(base.Year(), base.Month(), base.Day(), 18, 27, 0, 0, base.Location())
	ctx := context.Background()
	huntreport, err := store.AddProject(ctx, "huntreport")
	if err != nil {
		return err
	}
	thk, err := store.AddProject(ctx, "thk")
	if err != nil {
		return err
	}
	startTHKMorning := time.Date(now.Year(), now.Month(), now.Day(), 7, 58, 0, 0, now.Location())
	endTHKMorning := time.Date(now.Year(), now.Month(), now.Day(), 9, 39, 0, 0, now.Location())
	startTHKMidday := time.Date(now.Year(), now.Month(), now.Day(), 10, 6, 0, 0, now.Location())
	endTHKMidday := time.Date(now.Year(), now.Month(), now.Day(), 13, 37, 0, 0, now.Location())
	startHuntreport := time.Date(now.Year(), now.Month(), now.Day(), 14, 7, 0, 0, now.Location())
	if _, err := store.StartSession(ctx, startTHKMorning, &thk.ID); err != nil {
		return err
	}
	if _, err := store.AddNote(ctx, "do", "check merge requests, test divekit members alias functionality", startTHKMorning); err != nil {
		return err
	}
	if _, err := store.AddNote(ctx, "doing", "make aliases clear in other commands like members list, overview and gui", endTHKMorning); err != nil {
		return err
	}
	if _, err := store.EndRunningSession(ctx, endTHKMorning, ""); err != nil {
		return err
	}
	if _, err := store.StartSession(ctx, startTHKMidday, &thk.ID); err != nil {
		return err
	}
	if _, err := store.AddNote(ctx, "doing", "test more and tag to create a release", startTHKMidday); err != nil {
		return err
	}
	if _, err := store.AddNote(ctx, "doing", "feature done, writing docs while waiting for ci to finish", endTHKMidday); err != nil {
		return err
	}
	if _, err := store.EndRunningSession(ctx, endTHKMidday, ""); err != nil {
		return err
	}
	if _, err := store.StartSession(ctx, startHuntreport, &huntreport.ID); err != nil {
		return err
	}
	if _, err := store.AddNote(ctx, "doing", "Release-Workflow verifizieren", now.Add(-12*time.Minute)); err != nil {
		return err
	}

	today, err := todaySummary(ctx, store, now)
	if err != nil {
		return err
	}
	running, err := store.RunningSession(ctx)
	if err != nil {
		return err
	}
	lines := []string{
		badgeLine("running", formatDuration(now.Sub(running.StartedAt))),
		"",
	}
	if running.ProjectName.Valid {
		lines = append(lines, line("", running.ProjectName.String))
	}
	lines = append(lines, line("current", formatDateTime(running.StartedAt)))
	lines = appendTodaySummaryLines(lines, today, running, true)
	lines = appendTargetStatusLine(lines, 8*time.Hour, today.Work, now, true)
	printBlock(lines...)
	printTodayProjects(today.Sessions, now)
	return printTodayNotes(ctx, store, today.Sessions)
}

func demoStorePath() (string, error) {
	file, err := os.CreateTemp("", "work-demo-*.sqlite")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}
