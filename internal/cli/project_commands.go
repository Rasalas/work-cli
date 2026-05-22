package cli

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/sahilm/fuzzy"
	"github.com/spf13/cobra"

	"github.com/Rasalas/work-cli/internal/db"
	"github.com/Rasalas/work-cli/internal/tui"
)

func projectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "add <name>",
		Short: "Add or reactivate a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			project, err := store.AddProject(context.Background(), args[0])
			if err != nil {
				return err
			}
			printBlock(badgeLine("project", project.Name))
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List active projects",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			projects, err := store.ActiveProjects(context.Background())
			if err != nil {
				return err
			}
			if len(projects) == 0 {
				printMuted(line("projects", "none"))
				return nil
			}
			printSection("projects")
			for _, project := range projects {
				printLine(line("", project.Name))
			}
			fmt.Fprintln(out)
			return nil
		},
	})
	return cmd
}

func resolveProject(ctx context.Context, store *db.Store, opts options) (*int64, string, error) {
	if opts.project != "" && opts.noProject {
		return nil, "", fmt.Errorf("use either --project or --no-project")
	}
	if opts.noProject {
		return nil, "", nil
	}
	if opts.project != "" {
		project, err := resolveNamedProject(ctx, store, opts.project)
		if err != nil {
			return nil, "", err
		}
		return &project.ID, project.Name, nil
	}

	projects, err := store.ActiveProjects(ctx)
	if err != nil {
		return nil, "", err
	}
	switch len(projects) {
	case 0:
		return nil, "", nil
	case 1:
		return &projects[0].ID, projects[0].Name, nil
	default:
		picked, err := tui.PickProject(projects)
		if err != nil {
			return nil, "", err
		}
		if picked == nil {
			return nil, "", fmt.Errorf("project selection cancelled")
		}
		return &picked.ID, picked.Name, nil
	}
}

func resolveNamedProject(ctx context.Context, store *db.Store, name string) (db.Project, error) {
	project, err := store.ProjectByName(ctx, name)
	if err == nil {
		if project.Archived {
			return store.AddProject(ctx, project.Name)
		}
		return project, nil
	}
	if err != sql.ErrNoRows {
		return db.Project{}, err
	}

	projects, err := store.ActiveProjects(ctx)
	if err != nil {
		return db.Project{}, err
	}
	matches := fuzzy.FindFrom(name, projectSource(projects))
	switch len(matches) {
	case 0:
		return db.Project{}, fmt.Errorf("project %q not found; use `work project add %s` to create it", name, name)
	case 1:
		return projects[matches[0].Index], nil
	default:
		names := make([]string, 0, len(matches))
		for _, match := range matches {
			names = append(names, projects[match.Index].Name)
		}
		return db.Project{}, fmt.Errorf("project %q matches multiple projects: %s", name, strings.Join(names, ", "))
	}
}

type projectSource []db.Project

func (p projectSource) String(i int) string {
	return p[i].Name
}

func (p projectSource) Len() int {
	return len(p)
}
