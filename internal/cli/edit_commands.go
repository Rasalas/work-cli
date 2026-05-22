package cli

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/Rasalas/work-cli/internal/db"
	"github.com/Rasalas/work-cli/internal/timeparse"
)

func editCmd() *cobra.Command {
	var opts struct {
		start     string
		end       string
		project   string
		noProject bool
	}
	cmd := &cobra.Command{
		Use:   "edit <session-id>",
		Short: "Edit a logged work session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.start == "" && opts.end == "" && opts.project == "" && !opts.noProject {
				return fmt.Errorf("nothing to edit; use --start, --end, --project, or --no-project")
			}
			if opts.project != "" && opts.noProject {
				return fmt.Errorf("use either --project or --no-project")
			}

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

			var update db.SessionUpdate
			endBase := session.StartedAt
			if opts.start != "" {
				startedAt, err := timeparse.ParseStartTime(opts.start, session.StartedAt)
				if err != nil {
					return err
				}
				update.StartedAt = &startedAt
				endBase = startedAt
			}
			if opts.end != "" {
				endedAt, err := timeparse.ParseStartTime(opts.end, endBase)
				if err != nil {
					return err
				}
				update.EndedAt = &endedAt
			}
			if opts.noProject {
				update.ClearProject = true
			}
			if opts.project != "" {
				project, err := resolveNamedProject(ctx, store, opts.project)
				if err != nil {
					return err
				}
				update.ProjectID = &project.ID
			}

			updated, err := store.UpdateSession(ctx, id, update)
			if err != nil {
				return err
			}

			lines := []string{
				badgeLine("edited", fmt.Sprintf("#%d", updated.ID)),
				line("time", formatDateTime(updated.StartedAt)+" - "+formatEnd(&updated)),
			}
			if updated.ProjectName.Valid {
				lines = append(lines, line("project", updated.ProjectName.String))
			}
			printBlock(lines...)
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.start, "start", "", "start time")
	cmd.Flags().StringVar(&opts.end, "end", "", "end time")
	cmd.Flags().StringVarP(&opts.project, "project", "p", "", "project name")
	cmd.Flags().BoolVar(&opts.noProject, "no-project", false, "remove project")
	return cmd
}
