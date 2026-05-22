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
	target    string
	today     bool
	week      bool
	timeline  bool
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

	cmd.AddCommand(startCmd(), noteCmd("do"), noteCmd("doing"), noteCmd("done"), endCmd(), editCmd(), deleteCmd(), statusCmd(), logCmd(), projectCmd(), dbCmd())
	return cmd
}
