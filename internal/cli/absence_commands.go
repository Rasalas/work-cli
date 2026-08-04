package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Rasalas/work-cli/internal/db"
)

func absenceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "absence",
		Short: "Manage planned absences",
	}
	cmd.AddCommand(absenceAddCmd(), absenceListCmd())
	return cmd
}

func absenceAddCmd() *cobra.Command {
	var opts struct {
		project string
		from    string
		to      string
		kind    string
	}
	cmd := &cobra.Command{
		Use:   "add [project]",
		Short: "Add an absence date range",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.project != "" && len(args) > 0 {
				return fmt.Errorf("use either --project or positional project")
			}
			if opts.from == "" {
				return fmt.Errorf("--from is required")
			}
			kind := strings.TrimSpace(strings.ToLower(opts.kind))
			if kind == "" {
				return fmt.Errorf("--type is required")
			}
			startsOn, err := parseLogDate(opts.from, time.Local)
			if err != nil {
				return err
			}
			endsOn := startsOn
			if opts.to != "" {
				endsOn, err = parseLogDate(opts.to, time.Local)
				if err != nil {
					return err
				}
			}
			if endsOn.Before(startsOn) {
				return fmt.Errorf("absence end cannot be before start")
			}

			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()
			ctx := context.Background()
			projectArgs := args
			if opts.project != "" {
				projectArgs = []string{opts.project}
			}
			project, err := resolveProjectCommandProject(ctx, store, projectArgs, "work absence add <projectname> --from YYYY-MM-DD [--to YYYY-MM-DD]")
			if err != nil {
				return err
			}
			absence, err := store.AddProjectAbsence(ctx, project.ID, startsOn, endsOn, kind)
			if errors.Is(err, db.ErrOverlappingAbsence) {
				return fmt.Errorf("an absence for project %q already overlaps %s through %s", project.Name, startsOn.Format("2006-01-02"), endsOn.Format("2006-01-02"))
			}
			if err != nil {
				return err
			}
			printBlock(
				badgeLine("absence", fmt.Sprintf("#%d", absence.ID)),
				line("project", project.Name),
				line("type", absence.Kind),
				line("from", absence.StartsOn.Format("2006-01-02")),
				line("to", absence.EndsOn.Format("2006-01-02")),
			)
			return nil
		},
	}
	cmd.Flags().StringVarP(&opts.project, "project", "p", "", "project name")
	cmd.Flags().StringVar(&opts.from, "from", "", "first absence date YYYY-MM-DD")
	cmd.Flags().StringVar(&opts.to, "to", "", "last absence date YYYY-MM-DD (defaults to --from)")
	cmd.Flags().StringVar(&opts.kind, "type", "vacation", "absence type")
	return cmd
}

func absenceListCmd() *cobra.Command {
	var opts struct {
		project string
		from    string
		to      string
	}
	cmd := &cobra.Command{
		Use:   "list [project]",
		Short: "List absences",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.project != "" && len(args) > 0 {
				return fmt.Errorf("use either --project or positional project")
			}
			var from, to *time.Time
			if opts.from != "" {
				parsed, err := parseLogDate(opts.from, time.Local)
				if err != nil {
					return err
				}
				from = &parsed
			}
			if opts.to != "" {
				parsed, err := parseLogDate(opts.to, time.Local)
				if err != nil {
					return err
				}
				parsed = parsed.AddDate(0, 0, 1)
				to = &parsed
			}

			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()
			ctx := context.Background()
			projectArgs := args
			if opts.project != "" {
				projectArgs = []string{opts.project}
			}
			project, err := resolveProjectCommandProject(ctx, store, projectArgs, "work absence list <projectname>")
			if err != nil {
				return err
			}
			absences, err := store.ProjectAbsences(ctx, project.ID, from, to)
			if err != nil {
				return err
			}
			if len(absences) == 0 {
				printMuted(line("absences", "none"))
				return nil
			}
			printSection("absences")
			for _, absence := range absences {
				printLine(line("", fmt.Sprintf("#%d  %s  %s - %s", absence.ID, absence.Kind, absence.StartsOn.Format("2006-01-02"), absence.EndsOn.Format("2006-01-02"))))
			}
			fmt.Fprintln(out)
			return nil
		},
	}
	cmd.Flags().StringVarP(&opts.project, "project", "p", "", "project name")
	cmd.Flags().StringVar(&opts.from, "from", "", "include absences ending on or after YYYY-MM-DD")
	cmd.Flags().StringVar(&opts.to, "to", "", "include absences starting on or before YYYY-MM-DD")
	return cmd
}
