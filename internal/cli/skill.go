package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// newSkillCmd returns the 'mcli skill' command (alias: describe).
// It generates a structured Markdown document describing all mcli commands,
// intended for LLM agents to load once and understand the full CLI surface.
func newSkillCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "skill",
		Aliases: []string{"describe"},
		Short:   "Print a Markdown skill document describing all mcli commands",
		Long: `Print a structured Markdown document that fully describes all mcli commands,
their flags, output shapes, and workflow examples.

Designed for LLM agents: load once to learn the full CLI.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSkill(cmd)
		},
	}
}

// runSkill generates and writes the skill Markdown document.
func runSkill(cmd *cobra.Command) error {
	var sb strings.Builder

	writeSkillHeader(&sb)
	writeSkillAuthentication(&sb)
	writeSkillGlobalFlags(&sb)
	writeSkillCommands(&sb, cmd.Root())
	writeSkillOutputFormat(&sb)
	writeSkillErrorHandling(&sb)
	writeSkillExitCodes(&sb)
	writeSkillWorkflows(&sb)

	_, err := fmt.Fprint(cmd.OutOrStdout(), sb.String())
	return err
}

func writeSkillHeader(sb *strings.Builder) {
	sb.WriteString("# mcli — monday.com CLI\n\n")
	sb.WriteString("CLI for monday.com's GraphQL API. Structured JSON output, designed for LLM agents.\n\n")
	sb.WriteString("All commands emit JSON to stdout by default (or when `--json` is set).\n")
	sb.WriteString("Errors are emitted to stderr as `{\"error\":{\"code\":\"...\",\"message\":\"...\"}}` and exit with a non-zero code.\n\n")
}

func writeSkillAuthentication(sb *strings.Builder) {
	sb.WriteString("## Authentication\n\n")
	sb.WriteString("Token resolution order:\n")
	sb.WriteString("1. `--token` flag\n")
	sb.WriteString("2. `MONDAY_API_TOKEN` environment variable\n")
	sb.WriteString("3. Config file (`mcli auth login` stores token in OS keychain)\n\n")
	sb.WriteString("Use `mcli auth login` to store credentials. Use `mcli me` to verify.\n\n")
}

func writeSkillGlobalFlags(sb *strings.Builder) {
	sb.WriteString("## Global Flags\n\n")
	sb.WriteString("Available on every command:\n\n")
	sb.WriteString("| Flag | Type | Default | Description |\n")
	sb.WriteString("|------|------|---------|-------------|\n")
	sb.WriteString("| `--json` | bool | false | Force JSON output |\n")
	sb.WriteString("| `--pretty` | bool | false | Force human-readable output |\n")
	sb.WriteString("| `--token` | string | \"\" | monday.com API token |\n")
	sb.WriteString("| `--config` | string | \"\" | Config file path (default: $XDG_CONFIG_HOME/mcli/config.yaml) |\n")
	sb.WriteString("| `--verbose`, `-v` | bool | false | Emit request/response metadata to stderr |\n")
	sb.WriteString("| `--no-input` | bool | false | Never prompt; fail instead |\n\n")
}

// writeSkillCommands walks the cobra command tree and writes documentation for each command.
func writeSkillCommands(sb *strings.Builder, root *cobra.Command) {
	sb.WriteString("## Commands\n\n")

	var walk func(cmd *cobra.Command, prefix string)
	walk = func(cmd *cobra.Command, prefix string) {
		// Skip the root command itself and the help command.
		if cmd == root || cmd.Name() == "help" {
			subs := sortedSubcommands(cmd)
			for _, sub := range subs {
				walk(sub, prefix)
			}
			return
		}

		fullPath := strings.TrimSpace(prefix + " " + cmd.Name())

		subs := sortedSubcommands(cmd)
		if len(subs) > 0 {
			// Parent command — document it briefly, then recurse.
			sb.WriteString("### mcli " + fullPath + "\n\n")
			sb.WriteString(cmd.Short + "\n\n")
			for _, sub := range subs {
				walk(sub, fullPath)
			}
			return
		}

		// Leaf command — full documentation.
		sb.WriteString("### mcli " + fullPath + "\n\n")
		sb.WriteString(cmd.Short + "\n\n")

		if cmd.Long != "" {
			sb.WriteString(cmd.Long + "\n\n")
		}

		// Usage line.
		use := "mcli " + fullPath
		if cmd.Use != cmd.Name() {
			// Use contains args, e.g. "get <id>"
			parts := strings.SplitN(cmd.Use, " ", 2)
			if len(parts) == 2 {
				use += " " + parts[1]
			}
		}
		sb.WriteString("**Usage:** `" + use + "`\n\n")

		// Aliases.
		if len(cmd.Aliases) > 0 {
			sb.WriteString("**Aliases:** `mcli " + strings.Join(aliasedPaths(fullPath, cmd.Aliases), "`, `mcli ") + "`\n\n")
		}

		// Flags.
		localFlags := cmd.LocalFlags()
		if localFlags.HasFlags() {
			sb.WriteString("**Flags:**\n\n")
			sb.WriteString("| Flag | Type | Default | Description |\n")
			sb.WriteString("|------|------|---------|-------------|\n")
			localFlags.VisitAll(func(f *pflag.Flag) {
				def := f.DefValue
				if def == "" {
					def = "\"\""
				}
				sb.WriteString(fmt.Sprintf("| `--%s` | %s | %s | %s |\n", f.Name, f.Value.Type(), def, f.Usage))
			})
			sb.WriteString("\n")
		}

		// Static example output per command.
		if ex := exampleOutput(fullPath); ex != "" {
			sb.WriteString("**Example output:**\n\n```json\n" + ex + "\n```\n\n")
		}
	}

	walk(root, "")
}

func writeSkillOutputFormat(sb *strings.Builder) {
	sb.WriteString("## Output Format\n\n")
	sb.WriteString("All commands emit JSON to stdout. The shape depends on the command:\n\n")
	sb.WriteString("- **List commands** return `{\"items\":[...],\"cursor\":\"...\"}` — `cursor` is empty when no more pages.\n")
	sb.WriteString("- **Get commands** return a single object.\n")
	sb.WriteString("- **Write commands** (create/update/delete/archive/move) return the affected object.\n\n")
	sb.WriteString("When `--pretty` is set (or stdout is a TTY with no `--json`), output is human-readable tabular/text.\n\n")
}

func writeSkillErrorHandling(sb *strings.Builder) {
	sb.WriteString("## Error Handling\n\n")
	sb.WriteString("Errors are written to stderr as JSON and the process exits non-zero:\n\n")
	sb.WriteString("```json\n{\"error\":{\"code\":\"NOT_FOUND\",\"message\":\"item 999: not found\"}}\n```\n\n")
	sb.WriteString("Common error codes:\n\n")
	sb.WriteString("| Code | Meaning |\n")
	sb.WriteString("|------|---------|\n")
	sb.WriteString("| `USAGE` | Invalid arguments or flags |\n")
	sb.WriteString("| `NOT_FOUND` | Resource does not exist |\n")
	sb.WriteString("| `UNAUTHORIZED` | Missing or invalid API token |\n")
	sb.WriteString("| `GRAPHQL_ERROR` | API returned GraphQL errors |\n")
	sb.WriteString("| `INTERNAL` | Unexpected internal error |\n\n")
}

func writeSkillExitCodes(sb *strings.Builder) {
	sb.WriteString("## Exit Codes\n\n")
	sb.WriteString("| Code | Meaning |\n")
	sb.WriteString("|------|---------|\n")
	sb.WriteString("| 0 | Success |\n")
	sb.WriteString("| 1 | API or internal error |\n")
	sb.WriteString("| 2 | GraphQL error with partial data |\n")
	sb.WriteString("| 5 | Usage error (bad flags/args) |\n\n")
}

func writeSkillWorkflows(sb *strings.Builder) {
	sb.WriteString("## Workflows\n\n")

	sb.WriteString("### Create a board with structure and add items\n\n")
	sb.WriteString("```sh\n")
	sb.WriteString("# 1. Create the board\n")
	sb.WriteString("mcli board create --name \"Sprint Board\" --kind public\n")
	sb.WriteString("# → {\"id\":\"9832181507\",\"name\":\"Sprint Board\", ...}\n\n")
	sb.WriteString("# 2. Create a group\n")
	sb.WriteString("mcli board group create --board 9832181507 --name \"Sprint 1\"\n")
	sb.WriteString("# → {\"id\":\"topics\",\"title\":\"Sprint 1\", ...}\n\n")
	sb.WriteString("# 3. Add a column\n")
	sb.WriteString("mcli board column create --board 9832181507 --title Status --type status\n")
	sb.WriteString("# → {\"id\":\"status\",\"title\":\"Status\",\"type\":\"status\"}\n\n")
	sb.WriteString("# 4. Create an item in that group with a column value\n")
	sb.WriteString("mcli item create --board 9832181507 --group topics --name \"First task\" \\\n")
	sb.WriteString("  --col 'status={\"label\":\"Done\"}'\n")
	sb.WriteString("# → {\"id\":\"1234567890\",\"name\":\"First task\",\"state\":\"active\", ...}\n")
	sb.WriteString("```\n\n")

	sb.WriteString("### Read board structure and item data\n\n")
	sb.WriteString("```sh\n")
	sb.WriteString("# List all boards\n")
	sb.WriteString("mcli board list\n\n")
	sb.WriteString("# Get full board details\n")
	sb.WriteString("mcli board get 9832181507\n\n")
	sb.WriteString("# List groups on a board\n")
	sb.WriteString("mcli board group list --board 9832181507\n\n")
	sb.WriteString("# List items on a board (first page)\n")
	sb.WriteString("mcli item list --board 9832181507 --limit 50\n\n")
	sb.WriteString("# List items in a specific group\n")
	sb.WriteString("mcli item list --board 9832181507 --group topics\n\n")
	sb.WriteString("# Get a single item with full column details\n")
	sb.WriteString("mcli item get 1234567890\n")
	sb.WriteString("```\n\n")

	sb.WriteString("### Move, archive, and delete items\n\n")
	sb.WriteString("```sh\n")
	sb.WriteString("# Move item to a different group on the same board\n")
	sb.WriteString("mcli item move 1234567890 --to-group done_group\n\n")
	sb.WriteString("# Move item to a different board\n")
	sb.WriteString("mcli item move 1234567890 --to-board 9999999999 --group target_group\n\n")
	sb.WriteString("# Archive an item (recoverable)\n")
	sb.WriteString("mcli item archive 1234567890\n")
	sb.WriteString("# → {\"id\":\"1234567890\",\"name\":\"First task\",\"state\":\"archived\"}\n\n")
	sb.WriteString("# Permanently delete an item\n")
	sb.WriteString("mcli item delete 1234567890\n")
	sb.WriteString("# → {\"id\":\"1234567890\",\"name\":\"First task\",\"state\":\"deleted\"}\n")
	sb.WriteString("```\n\n")
}

// sortedSubcommands returns the subcommands of cmd sorted by name, excluding "help".
func sortedSubcommands(cmd *cobra.Command) []*cobra.Command {
	subs := make([]*cobra.Command, 0, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		if sub.Name() == "help" {
			continue
		}
		subs = append(subs, sub)
	}
	sort.Slice(subs, func(i, j int) bool {
		return subs[i].Name() < subs[j].Name()
	})
	return subs
}

// aliasedPaths returns the full paths for each alias.
func aliasedPaths(parentPath string, aliases []string) []string {
	// parentPath is the path without the final command name, so we need to
	// replace the last segment. E.g. "skill" → "describe" for path "skill".
	parts := strings.Fields(parentPath)
	if len(parts) == 0 {
		return aliases
	}
	parent := strings.Join(parts[:len(parts)-1], " ")
	result := make([]string, len(aliases))
	for i, a := range aliases {
		if parent == "" {
			result[i] = a
		} else {
			result[i] = parent + " " + a
		}
	}
	return result
}

// exampleOutput returns a static example JSON output string for a given command path.
// Returns empty string if no example is defined.
func exampleOutput(path string) string {
	examples := map[string]string{
		"version": `mcli version dev`,
		"me": `{
  "id": "12345678",
  "name": "Alice",
  "email": "alice@example.com",
  "account": {"id": "1234", "name": "My Org"}
}`,
		"auth login":  `Logged in as Alice (alice@example.com)`,
		"auth logout": `Logged out`,
		"auth status": `Logged in as Alice (alice@example.com)`,
		"workspace list": `{
  "items": [
    {"id": "1111111", "name": "Main Workspace", "kind": "open", "state": "active"}
  ],
  "cursor": ""
}`,
		"folder list": `{
  "items": [
    {"id": "2222222", "name": "Engineering", "workspace_id": "1111111"}
  ],
  "cursor": ""
}`,
		"board list": `{
  "items": [
    {"id": "9832181507", "name": "Sprint Board", "kind": "public", "state": "active", "workspace_id": "1111111"}
  ],
  "cursor": ""
}`,
		"board get": `{
  "id": "9832181507",
  "name": "Sprint Board",
  "kind": "public",
  "state": "active",
  "workspace_id": "1111111",
  "groups": [{"id": "topics", "title": "Sprint 1"}],
  "columns": [{"id": "status", "title": "Status", "type": "status"}]
}`,
		"board create": `{
  "id": "9832181507",
  "name": "Sprint Board",
  "kind": "public",
  "state": "active"
}`,
		"board rename": `{
  "id": "9832181507",
  "name": "New Name",
  "state": "active"
}`,
		"board delete": `{"id": "9832181507", "name": "Sprint Board"}`,
		"board archive": `{
  "id": "9832181507",
  "name": "Sprint Board",
  "state": "archived"
}`,
		"board group list": `{
  "items": [
    {"id": "topics", "title": "Sprint 1", "color": "#FF0000", "position": "0"}
  ],
  "cursor": ""
}`,
		"board group create": `{"id": "new_group_id", "title": "Sprint 2"}`,
		"board group delete": `{"id": "topics", "deleted": true}`,
		"board column list": `{
  "items": [
    {"id": "name", "title": "Name", "type": "name"},
    {"id": "status", "title": "Status", "type": "status"}
  ],
  "cursor": ""
}`,
		"board column create": `{"id": "status", "title": "Status", "type": "status"}`,
		"board column delete": `{"id": "status", "deleted": true}`,
		"item list": `{
  "items": [
    {
      "id": "1234567890",
      "name": "Build feature",
      "state": "active",
      "group": {"id": "topics", "title": "Sprint 1"},
      "columns": [
        {"id": "status", "type": "status", "value": "Done"},
        {"id": "due_date", "type": "date", "value": "2026-05-10"}
      ]
    }
  ],
  "cursor": ""
}`,
		"item get": `{
  "id": "1234567890",
  "name": "Build feature",
  "state": "active",
  "created_at": "2026-05-01",
  "updated_at": "2026-05-09",
  "creator": {"id": "1001", "name": "Alice"},
  "group": {"id": "topics", "title": "Sprint 1"},
  "board": {"id": "9832181507", "name": "Sprint Board"},
  "subitems": [],
  "columns": [
    {"id": "status", "type": "status", "value": "Done"}
  ]
}`,
		"item create": `{
  "id": "1234567890",
  "name": "Build feature",
  "state": "active",
  "board": {"id": "9832181507", "name": "Sprint Board"},
  "group": {"id": "topics", "title": "Sprint 1"}
}`,
		"item update": `{
  "id": "1234567890",
  "name": "Updated name",
  "state": "active",
  "board": {"id": "9832181507", "name": "Sprint Board"},
  "group": {"id": "topics", "title": "Sprint 1"}
}`,
		"item move": `{
  "id": "1234567890",
  "name": "Build feature",
  "state": "active",
  "board": {"id": "9832181507", "name": "Sprint Board"},
  "group": {"id": "done_group", "title": "Done"}
}`,
		"item delete":  `{"id": "1234567890", "name": "Build feature", "state": "deleted"}`,
		"item archive": `{"id": "1234567890", "name": "Build feature", "state": "archived"}`,
		"skill":        `(this document)`,
	}
	return examples[path]
}
