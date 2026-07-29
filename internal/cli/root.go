// Package cli implements the mcli command tree.
package cli

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	apischema "github.com/mondaycom/mcli/internal/api/schema"
	"github.com/mondaycom/mcli/internal/errs"
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

	requireKnownSubcommands(rootCmd)

	apischema.SetConfigDir(resolveConfigDir())
}

// unknownCommandHint is appended to every unknown-command error so a typo points
// the user (or an LLM) at the command index.
const unknownCommandHint = "Run 'mcli help' to see available commands."

// requireKnownSubcommands makes every namespace command (one with subcommands
// but no action of its own) reject unknown subcommands. Cobra's default shows
// the namespace's help and exits 0, so a typo like `mcli item lst` looks like a
// success. Giving the namespace a RunE makes it runnable, so cobra dispatches
// the stray arg here instead of bailing to help: no args → show help (the prior
// behavior), any arg → an "unknown command" usage error. The root command is
// left untouched — it already errors on unknown commands, with "Did you mean …?"
// suggestions.
func requireKnownSubcommands(parent *cobra.Command) {
	for _, cmd := range parent.Commands() {
		if cmd.Run == nil && cmd.RunE == nil && cmd.HasSubCommands() {
			cmd.RunE = func(c *cobra.Command, args []string) error {
				if len(args) > 0 {
					return errs.Usage("unknown command %q for %q\n%s", args[0], c.CommandPath(), unknownCommandHint)
				}
				return c.Help()
			}
		}
		requireKnownSubcommands(cmd)
	}
}

// Execute runs the root command with the given context.
func Execute(ctx context.Context) error {
	rootCmd.SetContext(ctx)
	return friendlyUnknownCommand(rootCmd.Execute())
}

// friendlyUnknownCommand rewrites cobra's bare "unknown command …" error (raised
// for an unknown top-level command) into a usage error that points at
// `mcli help`. This gives a typo an actionable message and exit code 1 (usage)
// instead of a generic exit 5, while preserving any "Did you mean …?" suggestion
// cobra already put in the message. Unknown subcommands are handled in
// requireKnownSubcommands and already carry the hint, so they don't match here.
func friendlyUnknownCommand(err error) error {
	if err == nil {
		return nil
	}
	if strings.HasPrefix(err.Error(), "unknown command ") {
		return errs.Usage("%s\n%s", err.Error(), unknownCommandHint)
	}
	return err
}
