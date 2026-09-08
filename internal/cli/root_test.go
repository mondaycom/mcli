package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/mondaycom/mcli/internal/errs"
)

func TestFriendlyUnknownCommand_WrapsWithHint(t *testing.T) {
	t.Parallel()

	got := friendlyUnknownCommand(errors.New(`unknown command "borad" for "mcli"`))
	e, ok := errors.AsType[*errs.Error](got)
	if !ok {
		t.Fatalf("expected *errs.Error, got %T", got)
	}
	if e.Code != errs.CodeUsage {
		t.Errorf("expected code %q, got %q", errs.CodeUsage, e.Code)
	}
	if !strings.Contains(e.Message, `unknown command "borad"`) {
		t.Errorf("expected original message preserved, got %q", e.Message)
	}
	if !strings.Contains(e.Message, unknownCommandHint) {
		t.Errorf("expected hint appended, got %q", e.Message)
	}
}

func TestFriendlyUnknownCommand_PassesThroughUnrelated(t *testing.T) {
	t.Parallel()

	if err := friendlyUnknownCommand(nil); err != nil {
		t.Errorf("nil must pass through, got %v", err)
	}
	orig := errors.New("boom")
	if got := friendlyUnknownCommand(orig); got != orig {
		t.Errorf("unrelated error must pass through unchanged, got %v", got)
	}
}

// newCobraValidationTree builds a command whose args and flag groups are validated by
// cobra itself, wired the way rootCmd is.
func newCobraValidationTree(args cobra.PositionalArgs, requiredFlags ...string) *cobra.Command {
	root := &cobra.Command{Use: "mcli", SilenceErrors: true, SilenceUsage: true}
	root.SetFlagErrorFunc(usageFlagError)

	leaf := &cobra.Command{
		Use:  "leaf",
		Args: args,
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	leaf.Flags().String("date", "", "")
	leaf.Flags().String("due", "", "")
	leaf.Flags().String("board", "", "")
	leaf.MarkFlagsMutuallyExclusive("date", "due")
	for _, name := range requiredFlags {
		if err := leaf.MarkFlagRequired(name); err != nil {
			panic(err)
		}
	}

	root.AddCommand(leaf)
	return root
}

// TestCobraValidationErrorsAreUsage drives real cobra rather than asserting on
// hand-written strings: the classifier matches cobra's message text, so a cobra
// upgrade that rewords one must fail here instead of silently regressing these
// invocations to INTERNAL (exit 5), which tells an agent to retry a call that can
// never succeed.
func TestCobraValidationErrorsAreUsage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		args     cobra.PositionalArgs
		argv     []string
		required []string
	}{
		{"exact args, too few", cobra.ExactArgs(1), []string{"leaf"}, nil},
		{"exact args, too many", cobra.ExactArgs(1), []string{"leaf", "a", "b"}, nil},
		{"at most", cobra.MaximumNArgs(1), []string{"leaf", "a", "b"}, nil},
		{"at least", cobra.MinimumNArgs(2), []string{"leaf", "a"}, nil},
		{"between", cobra.RangeArgs(2, 3), []string{"leaf", "a"}, nil},
		{"mutually exclusive flags", cobra.NoArgs, []string{"leaf", "--date", "x", "--due", "y"}, nil},
		{"unknown flag", cobra.NoArgs, []string{"leaf", "--bogus"}, nil},
		{"flag missing its value", cobra.NoArgs, []string{"leaf", "--board"}, nil},
		// Verified live: `mcli item update <id> --status Done` (no --board) reported
		// INTERNAL/5 until cobraRequiredFlagMarker was added. Nearly every item and
		// column command requires --board, so this is the most-hit malformed call.
		{"required flag not set", cobra.NoArgs, []string{"leaf"}, []string{"board"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := newCobraValidationTree(tc.args, tc.required...)
			root.SetOut(&bytes.Buffer{})
			root.SetErr(&bytes.Buffer{})
			root.SetArgs(tc.argv)

			err := friendlyUnknownCommand(root.Execute())
			if err == nil {
				t.Fatal("expected an error")
			}
			e, ok := errors.AsType[*errs.Error](err)
			if !ok {
				t.Fatalf("expected *errs.Error, got %T (%v)", err, err)
			}
			if e.Code != errs.CodeUsage {
				t.Errorf("expected USAGE (exit 1), got %s: %v", e.Code, err)
			}
		})
	}
}

// newNamespaceTree builds a minimal root → namespace → leaf tree mirroring the
// real command layout (a namespace with no action of its own) and applies
// requireKnownSubcommands to it.
func newNamespaceTree() (root, namespace, leaf *cobra.Command) {
	root = &cobra.Command{Use: "mcli", SilenceErrors: true, SilenceUsage: true}
	namespace = &cobra.Command{Use: "item"}
	leaf = &cobra.Command{Use: "get", RunE: func(*cobra.Command, []string) error { return nil }}
	namespace.AddCommand(leaf)
	root.AddCommand(namespace)
	requireKnownSubcommands(root)
	return root, namespace, leaf
}

func TestRequireKnownSubcommands_RejectsUnknownSubcommand(t *testing.T) {
	t.Parallel()

	root, _, _ := newNamespaceTree()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"item", "lst"})

	err := root.Execute()
	e, ok := errors.AsType[*errs.Error](err)
	if !ok {
		t.Fatalf("expected *errs.Error for unknown subcommand, got %T (%v)", err, err)
	}
	if e.Code != errs.CodeUsage {
		t.Errorf("expected USAGE, got %q", e.Code)
	}
	if !strings.Contains(e.Message, `unknown command "lst" for "mcli item"`) {
		t.Errorf("expected qualified command path, got %q", e.Message)
	}
	if !strings.Contains(e.Message, unknownCommandHint) {
		t.Errorf("expected hint, got %q", e.Message)
	}
}

func TestRequireKnownSubcommands_BareNamespaceShowsHelp(t *testing.T) {
	t.Parallel()

	root, _, _ := newNamespaceTree()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"item"})

	if err := root.Execute(); err != nil {
		t.Fatalf("bare namespace must not error, got %v", err)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Errorf("bare namespace should print help, got %q", out.String())
	}
}

func TestRequireKnownSubcommands_LeavesLeafRunEUntouched(t *testing.T) {
	t.Parallel()

	root, namespace, leaf := newNamespaceTree()

	// The namespace gained a RunE; the runnable leaf keeps its own.
	if namespace.RunE == nil {
		t.Error("namespace should have been given a RunE")
	}
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"item", "get"})
	if err := root.Execute(); err != nil {
		t.Errorf("known subcommand must still run, got %v", err)
	}
	if leaf.RunE == nil {
		t.Error("leaf RunE must be preserved")
	}
}
