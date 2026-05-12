package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

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

const skillDoc = `# mcli — monday.com CLI

CLI for monday.com's GraphQL API. Designed for LLM agents.

- JSON to stdout by default. Errors to stderr as ` + "`" + `{"error":{"code":"...","message":"..."}}` + "`" + `
- Discover flags for any command: ` + "`mcli <command> --help`" + `
- Auth: ` + "`--token <t>`" + ` flag > ` + "`MONDAY_API_TOKEN`" + ` env > stored credential (` + "`mcli auth login`" + `)

## Monday Hierarchy

` + "```" + `
Workspace → Folder → Board → { Group, Column } → Item → Subitem
` + "```" + `

## Goals & Commands

### Set up a board

1. Create a board: ` + "`mcli board create --name ... --kind public`" + `
2. Add groups to organize items: ` + "`mcli board group create --board <id> --name ...`" + `
3. Add columns to define data shape: ` + "`mcli board column create --board <id> --title ... --type status`" + `
4. Set column description (helps you understand it later): ` + "`mcli board column describe --board <id> --column <id> --text ...`" + `

### Discover board structure

- ` + "`mcli board get <id>`" + ` — full detail (groups + columns + settings)
- ` + "`mcli board group list --board <id>`" + ` — just groups
- ` + "`mcli board column list --board <id>`" + ` — just columns (includes settings_str with labels/options)

### CRUD items

- Create: ` + "`mcli item create --board <id> --name ... [--group <id>] [--col <col_id>=<json>]...`" + `
- Read list: ` + "`mcli item list --board <id> [--group <id>] [--limit N] [--cursor ...]`" + `
- Read one: ` + "`mcli item get <id>`" + ` — includes decoded column values, subitems, creator
- Update: ` + "`mcli item update <id> --board <id> [--name ...] [--col <col_id>=<json>]...`" + `
- Move: ` + "`mcli item move <id> --to-group <id>`" + ` or ` + "`--to-board <id> --group <id>`" + `
- Archive: ` + "`mcli item archive <id>`" + ` (recoverable)
- Delete: ` + "`mcli item delete <id>`" + ` (permanent)

### Subitems

` + "`mcli item create --parent <item_id> --name ...`" + ` — creates under a parent item.

### Workspace & folder management

- ` + "`mcli workspace list/get/create/update/delete`" + `
- ` + "`mcli folder list/create/rename/delete`" + `

### Board lifecycle

- ` + "`mcli board list`" + `, ` + "`mcli board rename <id> <name>`" + `, ` + "`mcli board archive <id>`" + `, ` + "`mcli board delete <id>`" + `
- Groups: ` + "`mcli board group list/create/rename/archive/delete`" + `
- Columns: ` + "`mcli board column list/create/rename/describe/delete`" + `

## Output Shapes

| Pattern | Shape |
|---------|-------|
| List | ` + "`" + `{"items":[...],"cursor":""}` + "`" + ` — empty cursor = last page |
| Get | Single object |
| Write | The affected object |
| Delete/Archive | ` + "`" + `{"id":"...","name":"...","state":"..."}` + "`" + ` |

## Column Values

On read: decoded to human-readable (status→label, date→ISO string, etc.).
On write: pass monday's raw JSON via ` + "`--col <column_id>=<json>`" + `. Common shapes:

| Type | Write JSON |
|------|-----------|
| status | ` + "`" + `{"label":"Done"}` + "`" + ` |
| date | ` + "`" + `{"date":"2026-05-10"}` + "`" + ` |
| text | ` + "`" + `"hello"` + "`" + ` |
| numeric | ` + "`" + `"42"` + "`" + ` |
| people | ` + "`" + `{"personsAndTeams":[{"id":123,"kind":"person"}]}` + "`" + ` |
| dropdown | ` + "`" + `{"ids":[1,2]}` + "`" + ` |
| checkbox | ` + "`" + `{"checked":"true"}` + "`" + ` |
| link | ` + "`" + `{"url":"https://...","text":"label"}` + "`" + ` |

Use ` + "`mcli board column list --board <id>`" + ` to discover column IDs, types, and settings.

## Error Codes

` + "`USAGE`" + ` (exit 5) · ` + "`AUTH`" + ` (exit 1) · ` + "`API`" + ` (exit 1) · ` + "`RATE_LIMITED`" + ` (auto-retried, exit 1 if exhausted)
`
