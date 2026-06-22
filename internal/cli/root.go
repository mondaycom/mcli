// Package cli implements the mcli command tree.
package cli

import (
	"github.com/spf13/cobra"

	apischema "github.com/mondaycom/mcli/internal/api/schema"
)

// GlobalFlags holds the values of flags available on every command.
type GlobalFlags struct {
	JSON    bool
	Pretty  bool
	Terse   bool
	CSV     bool
	Config  string
	Token   string
	Verbose bool
	NoInput bool
}

var globals GlobalFlags

// rootCmd is the base command for the mcli CLI.
// SilenceErrors / SilenceUsage are true so that error rendering is owned by
// PrintError (per ADR-002) rather than cobra's default formatter.
var rootCmd = &cobra.Command{
	Use:           "mcli",
	Short:         "mcli — monday.com CLI",
	Long:          "mcli is a command-line interface for the monday.com GraphQL API.",
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	pf := rootCmd.PersistentFlags()
	pf.BoolVar(&globals.JSON, "json", false, "force JSON output")
	pf.BoolVar(&globals.Pretty, "pretty", false, "force human-readable output")
	pf.BoolVar(&globals.Terse, "terse", false, "force terse single-line output")
	pf.BoolVar(&globals.CSV, "csv", false, "force CSV output")
	pf.StringVar(&globals.Config, "config", "", "config file path (default: $XDG_CONFIG_HOME/mcli/config.yaml)")
	pf.StringVar(&globals.Token, "token", "", "monday.com API token (env: MONDAY_API_TOKEN)")
	pf.BoolVarP(&globals.Verbose, "verbose", "v", false, "emit request/response metadata to stderr")
	pf.BoolVar(&globals.NoInput, "no-input", false, "never prompt; fail instead")

	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newAuthCmd())
	rootCmd.AddCommand(newConfigCmd())
	rootCmd.AddCommand(newMeCmd())
	rootCmd.AddCommand(newBoardCmd())
	rootCmd.AddCommand(newItemCmd())
	rootCmd.AddCommand(newWorkspaceCmd())
	rootCmd.AddCommand(newFolderCmd())
	rootCmd.AddCommand(newSkillCmd())
	rootCmd.AddCommand(newQueryCmd())
	rootCmd.AddCommand(newMutationCmd())
	rootCmd.AddCommand(newDaemonCmd())
	rootCmd.AddCommand(newNotificationCmd())
	rootCmd.AddCommand(newWebhookCmd())
	rootCmd.AddCommand(newSearchCmd())
	rootCmd.AddCommand(newDocCmd())
	rootCmd.AddCommand(newAPICmd())

	apischema.SetConfigDir(resolveConfigDir())
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
