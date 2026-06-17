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

### Search

` + "`mcli search <query> [-t boards|items|docs] [--limit N]`" + `

Search all entity types at once or restrict with ` + "`-t`" + `. Returns a JSON object with
` + "`boards`" + `, ` + "`items`" + `, and ` + "`docs`" + ` arrays (empty arrays for types not searched).

### CRUD items

- Create: ` + "`mcli item create --board <id> --name ... [--group <id>] [--col <col_id>=<json>]...`" + `
- Read list: ` + "`mcli item list --board <id> [--group <id>] [--limit N] [--cursor ...]`" + `
- Read one: ` + "`mcli item get <id>`" + ` — includes decoded column values, subitems, creator
- Find by column value: ` + "`mcli item find --board <id> --column <col_id> --value <text>`" + `
- Update: ` + "`mcli item update <id> --board <id> [--name ...] [--col <col_id>=<json>]...`" + `
- Move: ` + "`mcli item move <id> --to-group <id>`" + ` or ` + "`--to-board <id> --group <id>`" + `
- Archive: ` + "`mcli item archive <id>`" + ` (recoverable)
- Delete: ` + "`mcli item delete <id>`" + ` (permanent)
- Post update/comment: ` + "`mcli item post-update <id> --body <text>`" + `
- Read updates/comments: ` + "`mcli item get-updates <id> [--limit N]`" + `
- Read description: ` + "`mcli item description <id>`" + ` — returns markdown
- Write description: ` + "`mcli item description <id> --set <md>`" + ` or ` + "`--set-file <path>`" + `

### Subitems

` + "`mcli item create --parent <item_id> --name ...`" + ` — creates under a parent item.

### Workspace & folder management

- ` + "`mcli workspace list/get/create/update/delete`" + `
- ` + "`mcli folder list/create/rename/delete`" + `

### Board lifecycle

- ` + "`mcli board list`" + `, ` + "`mcli board rename <id> <name>`" + `, ` + "`mcli board archive <id>`" + `, ` + "`mcli board delete <id>`" + `
- Groups: ` + "`mcli board group list/create/rename/archive/delete`" + `
- Columns: ` + "`mcli board column list/create/rename/describe/delete`" + `

### Semantic Layer (recommended workflow)

Saved queries and mutations let you define a **business-level API** over monday.com
boards. Once set up, all operations use domain names like ` + "`create_order`" + ` or
` + "`list_products`" + ` instead of raw board IDs, column IDs, and GraphQL. This keeps
conversations in business terms: "add 3 widgets to the order" rather than
"create a subitem on board 1234 with col_xyz set to 3".

**Setup pattern (one-time, per project):**

1. Create boards and columns with ` + "`mcli board create`" + ` / ` + "`mcli board column create`" + `
2. Save domain-named queries and mutations that encode the GraphQL + board structure
3. Use only ` + "`mcli query run <name>`" + ` / ` + "`mcli mutation run <name>`" + ` for day-to-day operations

**Check if a semantic layer exists:** ` + "`mcli query list`" + ` and ` + "`mcli mutation list`" + `.
If saved operations exist, prefer them over raw commands.

**Example — e-commerce semantic layer:**

` + "```sh" + `
# Define once:
mcli mutation save create_order --query 'mutation($board: ID!, $name: String!, $cols: JSON!) { create_item(board_id: $board, item_name: $name, column_values: $cols) { id } }'
mcli mutation save add_order_line --query 'mutation($parent: ID!, $name: String!, $cols: JSON!) { create_subitem(parent_item_id: $parent, item_name: $name, column_values: $cols) { id } }'
mcli mutation save update_order_status --query 'mutation($board: ID!, $item: ID!, $cols: JSON!) { change_multiple_column_values(board_id: $board, item_id: $item, column_values: $cols) { id } }'
mcli query save list_products --query 'query($boardId: ID!) { boards(ids: [$boardId]) { items_page(limit:100) { items { id name column_values { id text value } } } } }'

# Use forever after — business language, no GraphQL knowledge needed:
mcli mutation run create_order --var board=123 --var name="ORD-42" --var cols='{"customer":"Acme","status":{"label":"Draft"}}'
mcli mutation run add_order_line --var parent=456 --var name="Widget x2" --var cols='{}'
mcli mutation run update_order_status --var board=123 --var item=456 --var cols='{"status":{"label":"Ordered"}}'
mcli query run list_products --var boardId=789
` + "```" + `

**JSON variable coercion:** Variables declared as ` + "`JSON`" + ` in the query are automatically
stringified on the wire. Write natural JSON in ` + "`--var cols='{...}'`" + ` — no double-encoding.

Saved operations live in ` + "`.mcli/queries/`" + ` and ` + "`.mcli/mutations/`" + ` (local, version-controllable)
or ` + "`~/.config/mcli/queries/`" + ` / ` + "`~/.config/mcli/mutations/`" + ` (` + "`--global`" + `).

Management: ` + "`mcli query list`" + ` · ` + "`mcli query delete <name>`" + ` · ` + "`mcli mutation list`" + ` · ` + "`mcli mutation delete <name>`" + `

### Raw GraphQL (escape hatch)

For anything not covered by structured commands or the semantic layer.
See https://developer.monday.com/api-reference/reference/about-the-api-reference for the full API.

- Inline: ` + "`mcli query '<graphql>' [--var key=value]...`" + `
- From file: ` + "`mcli query -f <file> [--var key=value]... [--vars-file vars.json]`" + `
- From stdin: ` + "`echo '...' | mcli query -f -`" + `

Variables are auto-typed: 42→int, true→bool, ` + "`{...}`" + `→JSON, else string.
Variables declared as ` + "`JSON`" + ` in the query are auto-stringified (no double-encoding needed).

` + "```sh" + `
mcli query save my-report --query 'query($boardId: ID!) { boards(ids: [$boardId]) { items_page(limit:100) { items { name column_values { id text } } } } }'
mcli query run my-report --var boardId=9832181507
` + "```" + `

Inline mutations: ` + "`mcli mutation '<graphql>' [--var key=value]...`" + `

Output is raw passthrough: ` + "`" + `{"data":...,"errors":...,"extensions":...}` + "`" + `. Exit 0 if no errors, exit 2 if errors present.

### Documents

- Read: ` + "`mcli doc read <doc_id>`" + ` — export full document as markdown
- Write: ` + "`mcli doc write <doc_id> --content <md>`" + ` or ` + "`--file <path>`" + ` — replace all content

### Current User

` + "`mcli me`" + ` — print id, name, email, account, and team memberships.

### Daemon + Notifications (webhook inbox)

1. Start daemon: ` + "`mcli daemon start`" + ` (requires ` + "`cloudflared`" + ` in PATH, or pass ` + "`--url`" + `)
2. Register webhook: ` + "`mcli webhook create --board <id> --event create_item`" + `
3. Poll for events: ` + "`mcli notification count`" + ` (quick) or ` + "`mcli notification list --unread`" + `
4. Acknowledge: ` + "`mcli notification ack <id>`" + ` or ` + "`mcli notification ack --all`" + `

Available events: ` + "`mcli webhook events`" + `

## Output Modes

Control output format with a global flag or ` + "`mcli config set output-mode <mode>`" + `:

| Flag | Config value | Description |
|------|-------------|-------------|
| ` + "`--json`" + ` | ` + "`json`" + ` | Machine-readable JSON (default when stdout is not a TTY) |
| ` + "`--pretty`" + ` | ` + "`pretty`" + ` | Human-readable table/label output (default when stdout is a TTY) |
| ` + "`--terse`" + ` | ` + "`terse`" + ` | Compact one-line-per-result output for scripting |
| ` + "`--csv`" + ` | ` + "`csv`" + ` | Comma-separated values with header row |

Precedence: explicit flag > config file > TTY detection.

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
