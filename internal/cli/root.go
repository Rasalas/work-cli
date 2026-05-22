package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

type options struct {
	project   string
	noProject bool
	at        string
	today     bool
	week      bool
}

func Execute() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "work",
		Short:         "Track local work sessions",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(startCmd(), noteCmd("do"), noteCmd("doing"), noteCmd("done"), endCmd(), statusCmd(), logCmd(), projectCmd(), dbCmd())
	return cmd
}
