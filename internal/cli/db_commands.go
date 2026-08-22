package cli

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/Rasalas/work-cli/internal/db"
)

func dbCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database inspection and maintenance",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Print the SQLite database path",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := db.DefaultPath()
			if err != nil {
				return err
			}
			fmt.Fprintln(out, path)
			return nil
		},
	})
	cmd.AddCommand(dbBackupCmd())
	return cmd
}

func dbBackupCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Write a consistent snapshot of the database to a timestamped copy",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := db.DefaultPath()
			if err != nil {
				return err
			}
			if dir == "" {
				dir = filepath.Dir(path)
			}

			store, err := openStore()
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()

			target := filepath.Join(dir, fmt.Sprintf(
				"%s.bak-%s.sqlite", filepath.Base(path), time.Now().Format("20060102-150405"),
			))
			if err := store.Backup(cmd.Context(), target); err != nil {
				return err
			}
			printBlock("Backup written to " + target)
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "directory for the backup file (default: alongside the database)")
	return cmd
}

func openStore() (*db.Store, error) {
	path, err := db.DefaultPath()
	if err != nil {
		return nil, err
	}
	return db.Open(path)
}
