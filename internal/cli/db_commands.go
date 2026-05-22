package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Rasalas/work-cli/internal/db"
)

func dbCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Show database information",
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
	return cmd
}

func openStore() (*db.Store, error) {
	path, err := db.DefaultPath()
	if err != nil {
		return nil, err
	}
	return db.Open(path)
}
