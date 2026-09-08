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
	rootCmd.AddCommand(newSchemaCmd())

	requireKnownSubcommands(rootCmd)

	// Subcommands inherit this: cobra's FlagErrorFunc() walks up to the parent.
	rootCmd.SetFlagErrorFunc(usageFlagError)

	apischema.SetConfigDir(resolveConfigDir())
}

// unknownCommandHint is appended to every unknown-command error so a typo points
// the user (or an LLM) at the command index.
const unknownCommandHint = "Run 'mcli help' to see available commands."

// usageFlagError classifies a flag parse failure (unknown flag, missing value, bad
// value) as a usage error. Cobra returns these as plain errors, which would otherwise
// reach the exit-code translator unclassified and report INTERNAL.
func usageFlagError(cmd *cobra.Command, err error) error {
	return errs.Usage("%s\nRun '%s --help' for usage.", err.Error(), cmd.CommandPath())
}

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

// cobraArgErrorMarkers identify cobra's argument-count validation failures
// (cobra.ExactArgs and friends). Cobra builds these with fmt.Errorf and exposes no
// sentinel, so matching the message is the only hook available. The markers are the
// invariant middles of those messages rather than their prefixes, which vary
// ("accepts", "accepts at most", "accepts between … and …").
var cobraArgErrorMarkers = []string{
	" arg(s), received ",      // accepts N / at most N / between N and M
	" arg(s), only received ", // requires at least N
}

// cobraFlagGroupMarker identifies all three of cobra's flag-group validation
// failures (MarkFlagsMutuallyExclusive, RequiredTogether, OneRequired).
const cobraFlagGroupMarker = "flags in the group ["

// cobraRequiredFlagMarker identifies MarkFlagRequired violations, e.g.
// `required flag(s) "board" not set`. Cobra raises these from ValidateRequiredFlags
// after parsing succeeds, so they bypass the FlagErrorFunc entirely. This is the
// most common malformed call mcli sees — nearly every item and column command
// requires --board — which makes it the worst one to report as INTERNAL.
const cobraRequiredFlagMarker = "required flag(s) "

// friendlyUnknownCommand reclassifies cobra's own validation failures as usage
// errors so they exit 1 instead of a generic 5.
//
// This matters more than it looks: mcli is driven by LLM agents in steady state, and
// INTERNAL reads as "mcli broke, try again" while USAGE reads as "your call was
// wrong, fix it". Leaving a malformed invocation on exit 5 invites an agent to retry
// a call that can never succeed. Flag *parse* errors are handled by the FlagErrorFunc
// set in init; this covers the checks cobra runs after parsing, which do not route
// through it.
//
// The "unknown command" branch also keeps cobra's "Did you mean …?" suggestion.
// Unknown subcommands are handled in requireKnownSubcommands and already carry the
// hint, so they do not reach here.
func friendlyUnknownCommand(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()

	if strings.HasPrefix(msg, "unknown command ") {
		return errs.Usage("%s\n%s", msg, unknownCommandHint)
	}
	if strings.Contains(msg, cobraFlagGroupMarker) || strings.HasPrefix(msg, cobraRequiredFlagMarker) {
		return errs.Usage("%s", msg)
	}
	for _, marker := range cobraArgErrorMarkers {
		if strings.Contains(msg, marker) {
			return errs.Usage("%s", msg)
		}
	}
	return err
}
