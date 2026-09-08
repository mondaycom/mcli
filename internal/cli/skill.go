package cli

import (
	_ "embed"
	"fmt"

	"github.com/spf13/cobra"
)

// skillDoc is the agent-facing description of mcli. It lives in skill.md rather
// than in a Go string literal: markdown is full of backticks, and a Go raw string
// cannot contain one, so the previous `+ "`" + ` form made the doc effectively
// unreviewable and let it drift from the real flag set.
//
//go:embed skill.md
var skillDoc string

func newSkillCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "skill",
		Aliases: []string{"describe"},
		Short:   "Print a Markdown skill document describing all mcli commands",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSkill(cmd)
		},
	}
}

func runSkill(cmd *cobra.Command) error {
	_, err := fmt.Fprint(cmd.OutOrStdout(), skillDoc)
	return err
}
