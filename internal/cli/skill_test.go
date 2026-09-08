package cli

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mondaycom/mcli/internal/errs"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
		"## Start Here: Discover IDs",
		"## Read Items",
		"## Write Items",
		"## Column Values",
		"## Errors: What To Do Next",
		"## Output Modes & Shapes",
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

// skillDocMaxBytes is a budget, not a limit found by measurement: this doc is loaded
// into an agent's context on every session, so a new section should displace
// something rather than simply appending. Raise it only with a reason.
//
// The budget is in bytes, not lines, because bytes are what the doc actually costs
// an agent. Line count is a poor proxy: reflowing a paragraph changes it without
// changing the cost, while one dense table row costs far more than one short line.
const skillDocMaxBytes = 14000

func TestSkill_Concise(t *testing.T) {
	out, err := execSkill(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) > skillDocMaxBytes {
		t.Errorf("skill doc should be concise; got %d bytes (max %d)", len(out), skillDocMaxBytes)
	}
}

// TestSkill_ErrorCodesMatchErrs guards the skill doc against drifting from the
// real exit-code mapping. Every code in errs.AllCodes must appear in the doc
// annotated with the exit code errs.ToExitCode actually returns for it.
func TestSkill_ErrorCodesMatchErrs(t *testing.T) {
	out, err := execSkill(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, code := range errs.AllCodes() {
		want := errs.ToExitCode(&errs.Error{Code: code})

		// Matches "`CODE` 4" and grouped forms like "`API`/`NOT_FOUND` 2".
		re := regexp.MustCompile("`" + string(code) + "`(?:/`[A-Z_]+`)*\\s+(\\d+)")
		m := re.FindStringSubmatch(out)
		if m == nil {
			t.Errorf("code %s is not documented with an exit code in the skill doc", code)
			continue
		}
		if got, _ := strconv.Atoi(m[1]); got != want {
			t.Errorf("skill doc documents %s as exit %d, but errs.ToExitCode returns %d", code, got, want)
		}
	}
}

// ─── Drift guard ────────────────────────────────────────────────────────────
//
// The skill doc is the only description of mcli an agent gets, and a wrong flag in
// it costs a failed API call and a retry loop. Nothing tied it to the real command
// tree, so it drifted: `board create` was documented without `--workspace` (whose
// absence returns an opaque "User unauthorized"), and `board column create` without
// `--defaults` (whose absence silently gives a status column monday's stock labels).
// These tests read the doc the way an agent does — every invocation it could copy —
// and check each one against the real cobra tree.

var (
	skillCodeSpanRE = regexp.MustCompile("`([^`\n]+)`")
	// skillCommandWordRE matches a token that could name a subcommand, including the
	// compressed alternation forms the doc uses ("list/create", "query|mutation").
	skillCommandWordRE = regexp.MustCompile(`^[a-z][a-z0-9|/-]*$`)
	skillFlagRE        = regexp.MustCompile(`--[a-z][a-z0-9-]*|(?:^|\s)-([a-z])\b`)
)

// skillInvocations returns every documented mcli invocation: the text of each inline
// code span, and each line of each fenced code block, from "mcli " onward. Those are
// the strings an agent copies, so they are the ones that have to stay true.
func skillInvocations(doc string) []string {
	var out []string
	add := func(s string) {
		if _, rest, ok := strings.Cut(s, "mcli "); ok {
			out = append(out, strings.TrimSpace(rest))
		}
	}
	inFence := false
	for line := range strings.SplitSeq(doc, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			add(line)
			continue
		}
		for _, m := range skillCodeSpanRE.FindAllStringSubmatch(line, -1) {
			add(m[1])
		}
	}
	return out
}

// skillCommandPaths expands an invocation's leading command words into one path per
// alternative, so "board group list/create" yields both real paths. Expansion stops
// at the first token that cannot be a command name — a placeholder, flag, or literal
// — which keeps flag *values* like "public|private|share" from being expanded.
func skillCommandPaths(tokens []string) [][]string {
	paths := [][]string{{}}
	for _, tok := range tokens {
		if !skillCommandWordRE.MatchString(tok) {
			break
		}
		alts := strings.FieldsFunc(tok, func(r rune) bool { return r == '/' || r == '|' })
		var next [][]string
		for _, p := range paths {
			for _, alt := range alts {
				next = append(next, append(append([]string{}, p...), alt))
			}
		}
		paths = next
	}
	return paths
}

// skillResolve walks as far down the real command tree as the path goes, returning
// the deepest command matched and how many tokens it consumed.
func skillResolve(path []string) (*cobra.Command, int) {
	cmd := rootCmd
	depth := 0
	for _, tok := range path {
		child := skillChild(cmd, tok)
		if child == nil {
			break
		}
		cmd, depth = child, depth+1
	}
	return cmd, depth
}

func skillChild(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
		if slices.Contains(c.Aliases, name) {
			return c
		}
	}
	return nil
}

// skillHasFlag reports whether name is a real flag on cmd, counting flags inherited
// from a parent or from the root's persistent set.
func skillHasFlag(cmd *cobra.Command, name string) bool {
	for _, set := range []*pflag.FlagSet{cmd.Flags(), cmd.PersistentFlags(), cmd.InheritedFlags(), rootCmd.PersistentFlags()} {
		if set.Lookup(name) != nil {
			return true
		}
	}
	return false
}

func skillHasShorthand(cmd *cobra.Command, short string) bool {
	for _, set := range []*pflag.FlagSet{cmd.Flags(), cmd.PersistentFlags(), cmd.InheritedFlags(), rootCmd.PersistentFlags()} {
		if set.ShorthandLookup(short) != nil {
			return true
		}
	}
	return false
}

// skillAudit is the result of checking a doc against the real command tree.
type skillAudit struct {
	problems     []string
	pathsChecked int
	flagsChecked int
}

// auditSkillDoc checks every invocation in doc against the real cobra tree: that the
// command path exists, and that each flag shown alongside it is a real flag on that
// command. It returns human-readable problems rather than failing, so a negative test
// can assert that it actually catches drift.
func auditSkillDoc(doc string) skillAudit {
	var a skillAudit
	for _, inv := range skillInvocations(doc) {
		tokens := strings.Fields(inv)
		if len(tokens) == 0 {
			continue
		}
		for _, path := range skillCommandPaths(tokens) {
			if len(path) == 0 {
				continue // e.g. `mcli <command> --help`, a deliberate placeholder
			}
			cmd, depth := skillResolve(path)
			if depth == 0 {
				a.problems = append(a.problems, fmt.Sprintf("unknown command %q (in `mcli %s`)", path[0], inv))
				continue
			}
			// Tokens past the resolved command are positional args, not typos:
			// `mcli mutation save create_order` resolves to "mutation save".
			a.pathsChecked++
			for _, m := range skillFlagRE.FindAllStringSubmatch(inv, -1) {
				a.flagsChecked++
				if short := m[1]; short != "" {
					if !skillHasShorthand(cmd, short) {
						a.problems = append(a.problems, fmt.Sprintf("-%s is not a shorthand on %q (in `mcli %s`)", short, cmd.CommandPath(), inv))
					}
					continue
				}
				name := strings.TrimPrefix(m[0], "--")
				if !skillHasFlag(cmd, name) {
					a.problems = append(a.problems, fmt.Sprintf("--%s is not a flag on %q (in `mcli %s`)", name, cmd.CommandPath(), inv))
				}
			}
		}
	}
	return a
}

// TestSkillDoc_MatchesCommandTree is the check that was missing when the doc drifted:
// every command and flag an agent could copy out of the doc has to be real.
func TestSkillDoc_MatchesCommandTree(t *testing.T) {
	a := auditSkillDoc(skillDoc)
	for _, p := range a.problems {
		t.Errorf("skill doc: %s", p)
	}
	// Guard the guard: without these, a broken extractor would pass vacuously.
	if a.pathsChecked < 40 {
		t.Errorf("only checked %d command paths; expected 40+ — is skillInvocations still working?", a.pathsChecked)
	}
	if a.flagsChecked < 60 {
		t.Errorf("only checked %d flags; expected 60+ — is skillFlagRE still working?", a.flagsChecked)
	}
}

// TestSkillDoc_AuditCatchesDrift proves the audit above can fail. Each case is a form
// of drift that has actually happened or plausibly could.
func TestSkillDoc_AuditCatchesDrift(t *testing.T) {
	cases := map[string]string{
		"renamed flag":      "`mcli board create --name x --workspac 1`",
		"flag on wrong cmd": "`mcli item list --defaults x`",
		"deleted command":   "`mcli board coloumn list --board 1`",
		"bad shorthand":     "`mcli search foo -z boards`",
		"drift in a fence":  "```sh\nmcli item create --board 1 --nam x\n```",
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			if a := auditSkillDoc(strings.ReplaceAll(doc, "\\n", "\n")); len(a.problems) == 0 {
				t.Errorf("audit found no problem in %q, but it should have", doc)
			}
		})
	}
}
