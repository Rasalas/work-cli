package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Rasalas/work-cli/internal/db"
)

func noteAdminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "note",
		Short: "List and edit work session notes",
	}
	cmd.AddCommand(noteListCmd(), noteEditCmd())
	return cmd
}

func noteListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list <session-id>",
		Aliases: []string{"ls"},
		Short:   "List notes for a work session",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID, err := parsePositiveID(args[0], "session id")
			if err != nil {
				return err
			}

			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			ctx := context.Background()
			if _, err := store.SessionByID(ctx, sessionID); errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("session #%d not found", sessionID)
			} else if err != nil {
				return err
			}
			notes, err := store.NotesForSession(ctx, sessionID)
			if err != nil {
				return err
			}
			if len(notes) == 0 {
				printMuted(line("notes", "none"))
				return nil
			}
			for _, note := range notes {
				printLine(fmt.Sprintf("%4d  %s", note.ID, noteLine(note)))
			}
			return nil
		},
	}
	return cmd
}

func noteEditCmd() *cobra.Command {
	var opts struct {
		at   string
		kind string
	}
	cmd := &cobra.Command{
		Use:   "edit <note-id> [note]",
		Short: "Edit a work session note",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.at == "" && opts.kind == "" && len(args) == 1 {
				return fmt.Errorf("nothing to edit; use --at, --kind, or provide note text")
			}
			noteID, err := parsePositiveID(args[0], "note id")
			if err != nil {
				return err
			}

			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			ctx := context.Background()
			note, err := store.NoteByID(ctx, noteID)
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("note #%d not found", noteID)
			}
			if err != nil {
				return err
			}
			session, err := store.SessionByID(ctx, note.SessionID)
			if err != nil {
				return err
			}

			var update db.NoteUpdate
			if opts.at != "" {
				createdAt, err := parseNoteTime(opts.at, session, note.CreatedAt)
				if err != nil {
					return err
				}
				update.CreatedAt = &createdAt
			}
			if opts.kind != "" {
				kind := strings.TrimSpace(opts.kind)
				if kind == "" {
					return fmt.Errorf("kind cannot be empty")
				}
				update.Kind = &kind
			}
			if len(args) > 1 {
				body := strings.Join(args[1:], " ")
				update.Body = &body
			}

			updated, err := store.UpdateNote(ctx, noteID, update)
			if err != nil {
				return err
			}
			printBlock(
				badgeLine("edited", fmt.Sprintf("#%d", updated.ID)),
				noteLine(updated),
			)
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.at, "at", "", "note time, or start/end for the note's session")
	cmd.Flags().StringVar(&opts.kind, "kind", "", "note kind")
	return cmd
}

func parsePositiveID(input, label string) (int64, error) {
	id, err := strconv.ParseInt(input, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid %s %q", label, input)
	}
	return id, nil
}
