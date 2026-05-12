package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

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
		t.Errorf("expected output to start with '# mcli', got: %q", out[:min(50, len(out))])
	}
}

func TestSkill_ContainsGoals(t *testing.T) {
	out, err := execSkill(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	required := []string{
		"mcli board create",
		"mcli board group create",
		"mcli board column create",
		"mcli item create",
		"mcli item list",
		"mcli item get",
		"mcli item update",
		"mcli item move",
		"mcli item delete",
		"mcli item archive",
		"mcli workspace",
		"mcli folder",
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
		"## Monday Hierarchy",
		"## Goals & Commands",
		"## Output Shapes",
		"## Column Values",
		"## Error Codes",
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
		t.Errorf("alias 'describe' should produce same output, got: %q", out[:min(50, len(out))])
	}
}

func TestSkill_Concise(t *testing.T) {
	out, err := execSkill(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Count(out, "\n")
	if lines > 200 {
		t.Errorf("skill doc should be concise; got %d lines (max 200)", lines)
	}
}
