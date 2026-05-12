package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// execSkill runs 'mcli skill' and returns stdout.
func execSkill(t *testing.T, args ...string) (string, error) {
	t.Helper()

	var buf bytes.Buffer
	root := buildTestRoot()
	root.SetOut(&buf)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"skill"}, args...))
	err := root.Execute()
	return buf.String(), err
}

// buildTestRoot constructs a root command with all subcommands wired,
// mirroring what init() does, but isolated for testing.
func buildTestRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "mcli",
		Short:         "mcli — monday.com CLI",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.AddCommand(newVersionCmd())
	root.AddCommand(newAuthCmd())
	root.AddCommand(newMeCmd())
	root.AddCommand(newBoardCmd())
	root.AddCommand(newItemCmd())
	root.AddCommand(newWorkspaceCmd())
	root.AddCommand(newFolderCmd())
	root.AddCommand(newSkillCmd())
	return root
}

func TestSkill_NonEmpty(t *testing.T) {
	out, err := execSkill(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatal("expected non-empty output from skill command")
	}
}

func TestSkill_StartsWithH1(t *testing.T) {
	out, err := execSkill(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(out, "# mcli") {
		t.Errorf("expected output to start with '# mcli', got: %q", out[:minInt(50, len(out))])
	}
}

func TestSkill_ContainsKeyCommands(t *testing.T) {
	out, err := execSkill(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	required := []string{
		"mcli workspace list",
		"mcli board list",
		"mcli board get",
		"mcli board create",
		"mcli board delete",
		"mcli board archive",
		"mcli board group",
		"mcli board column",
		"mcli item list",
		"mcli item get",
		"mcli item create",
		"mcli item update",
		"mcli item move",
		"mcli item delete",
		"mcli item archive",
		"mcli folder list",
		"mcli auth",
		"mcli me",
		"mcli version",
		"mcli skill",
	}

	for _, keyword := range required {
		if !strings.Contains(out, keyword) {
			t.Errorf("expected skill output to contain %q", keyword)
		}
	}
}

func TestSkill_ContainsSections(t *testing.T) {
	out, err := execSkill(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sections := []string{
		"## Authentication",
		"## Global Flags",
		"## Commands",
		"## Output Format",
		"## Error Handling",
		"## Exit Codes",
		"## Workflows",
	}

	for _, section := range sections {
		if !strings.Contains(out, section) {
			t.Errorf("expected skill output to contain section %q", section)
		}
	}
}

func TestSkill_IsDeterministic(t *testing.T) {
	out1, err := execSkill(t)
	if err != nil {
		t.Fatalf("first run error: %v", err)
	}
	out2, err := execSkill(t)
	if err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if out1 != out2 {
		t.Error("skill output is not deterministic across two runs")
	}
}

func TestSkill_AliasDescribe(t *testing.T) {
	var buf bytes.Buffer
	root := buildTestRoot()
	root.SetOut(&buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"describe"})
	err := root.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "# mcli") {
		t.Errorf("alias 'describe' should produce same output, got: %q", out[:minInt(50, len(out))])
	}
}

func TestSkill_ContainsGlobalFlags(t *testing.T) {
	out, err := execSkill(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	globalFlags := []string{"--json", "--token", "--config", "--verbose", "--no-input", "--pretty"}
	for _, flag := range globalFlags {
		if !strings.Contains(out, flag) {
			t.Errorf("expected skill output to contain global flag %q", flag)
		}
	}
}

func TestSkill_ContainsErrorFormat(t *testing.T) {
	out, err := execSkill(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"error"`) {
		t.Error("expected skill output to document error JSON format")
	}
}

// minInt returns the smaller of a and b.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
