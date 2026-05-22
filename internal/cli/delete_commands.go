package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Rasalas/work-cli/internal/db"
	"github.com/Rasalas/work-cli/internal/tui"
)

var confirmWithContent = tui.ConfirmWithContent

func deleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <session-id>",
		Aliases: []string{"rm"},
		Short:   "Delete a logged work session",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil || id <= 0 {
				return fmt.Errorf("invalid session id %q", args[0])
			}

			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			ctx := context.Background()
			session, err := store.SessionByID(ctx, id)
			if err != nil {
				return err
			}
			notes, err := store.NotesForSession(ctx, session.ID)
			if err != nil {
				return err
			}
			confirmed, err := confirmDeleteSession(session, notes)
			if err != nil {
				return err
			}
			if !confirmed {
				printMuted(line("delete", "cancelled"))
				return nil
			}

			deleted, err := store.DeleteSession(ctx, id)
			if err != nil {
				return err
			}

			printBlock(
				badgeLine("deleted", fmt.Sprintf("#%d", deleted.ID)),
				line("time", formatDateTime(deleted.StartedAt)+" - "+formatEnd(&deleted)),
			)
			return nil
		},
	}
	return cmd
}

func confirmDeleteSession(session db.Session, notes []db.Note) (bool, error) {
	content := strings.Join(deleteSessionPlanLines(session, notes), "\n")
	return confirmWithContent(fmt.Sprintf("DELETE Session #%d", session.ID), content, "Delete this session?")
}

func deleteSessionPlanLines(session db.Session, notes []db.Note) []string {
	lines := []string{
		fmt.Sprintf("  %s  %s - %s", confirmPlanLabel("time"), formatDateTime(session.StartedAt), formatEnd(&session)),
		fmt.Sprintf("  %s  %s", confirmPlanLabel("duration"), formatSessionDuration(session, time.Now())),
	}
	if session.ProjectName.Valid && strings.TrimSpace(session.ProjectName.String) != "" {
		lines = append(lines, fmt.Sprintf("  %s  %s", confirmPlanLabel("project"), session.ProjectName.String))
	}
	if len(notes) > 0 {
		lines = append(lines, "", fmt.Sprintf("  %s", confirmPlanSection("notes")))
		for _, note := range notes {
			lines = append(lines, fmt.Sprintf("  %s  %s", confirmPlanNoteKind(note.Kind), note.Body))
		}
	}
	return lines
}

func confirmPlanLabel(label string) string {
	text := fmt.Sprintf("%8s", label)
	if _, noColor := os.LookupEnv("NO_COLOR"); noColor {
		return text
	}
	return "\033[38;5;246m" + text + "\033[39m"
}

func confirmPlanNoteKind(kind string) string {
	text := fmt.Sprintf("%8s", kind)
	if _, noColor := os.LookupEnv("NO_COLOR"); noColor {
		return text
	}
	return "\033[38;5;246m" + text + "\033[39m"
}

func confirmPlanSection(title string) string {
	text := fmt.Sprintf("%8s", title)
	if _, noColor := os.LookupEnv("NO_COLOR"); noColor {
		return text
	}
	return "\033[38;5;252m" + text + "\033[39m"
}
