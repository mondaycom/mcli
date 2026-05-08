package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version is injected at build time via -ldflags.
var version = "dev"

// newVersionCmd returns the 'mcli version' command.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the mcli version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "mcli version", version)
			return err
		},
	}
}
