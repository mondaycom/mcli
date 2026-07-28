package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestParentAliases verifies that the plural aliases ("items", "boards")
// resolve to the same command tree as their singular canonical names, so
// subcommands are reachable through either spelling.
func TestParentAliases(t *testing.T) {
	cases := []struct {
		name    string
		build   func() *cobra.Command
		args    []string
		wantSub string
	}{
		{"items->item list", newItemCmd, []string{"items", "list"}, "list"},
		{"item list", newItemCmd, []string{"item", "list"}, "list"},
		{"boards->board get", newBoardCmd, []string{"boards", "get"}, "get"},
		{"board get", newBoardCmd, []string{"board", "get"}, "get"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := &cobra.Command{Use: "mcli"}
			root.AddCommand(tc.build())

			found, _, err := root.Find(tc.args)
			if err != nil {
				t.Fatalf("Find(%v): %v", tc.args, err)
			}
			if found.Name() != tc.wantSub {
				t.Errorf("resolved to %q, want %q", found.Name(), tc.wantSub)
			}
		})
	}
}
