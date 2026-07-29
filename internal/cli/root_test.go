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
