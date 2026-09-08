# mcli — monday.com CLI

CLI for monday.com's GraphQL API, built for LLM agents: non-interactive, one JSON
object per call, stable error codes.

- **Output:** a single JSON object on stdout (never a stream). Errors go to stderr as `{"error":{"code":"...","message":"..."}}` and the exit code follows the code — see Errors.
- **It never guesses.** When a command cannot tell what you meant it fails with `USAGE` and names the candidates. Read the message; do not retry the same call.
- **Discovery:** `mcli <command> --help` for real flags · `mcli api list` for every API operation.
- **Auth:** `--token <t>` > `MONDAY_API_TOKEN` > stored credential (`mcli auth login --token <t>`). Verify with `mcli auth status`.
- **Global flags:** `--json` / `--pretty` / `--terse` / `--csv` · `--no-input` (never prompt, fail instead) · `-v` (request metadata to stderr).

## Monday Hierarchy

```
Workspace → Folder → Board → { Group, Column } → Item → Subitem
                   → Doc
```

An item lives in one group on one board. A *column* defines a field; a *column value*
holds that item's data for it. Subitems live on a separate board that monday creates
on first use.

## Start Here: Discover IDs

Board, group, and column IDs are opaque and not guessable, and every write needs them.

- Find by name: `mcli search <query> [-t boards|items|docs] [--limit N]` (max 20 per type)
- `mcli board list [--workspace <id>] [--limit N] [--cursor <c>]`
- **Board shape — the one to reach for:** `mcli board get <id>` returns groups + columns + settings. Add `--items` for the first page of items with values (`--items-limit N` max 500, `--items-cursor <c>`).
- Columns only: `mcli board column list --board <id>` → id, title, type, description, and `settings_str` (status labels live here)
- Groups only: `mcli board group list --board <id>`

## Read Items

- Page a board: `mcli item list --board <id> [--group <id>] [--limit N] [--cursor <c>] [--subitems]`
- One item: `mcli item get <id> [--subitems] [--subitem-count N]`
- By column value: `mcli item find --board <id> --column <col_id> --value <text>` — status columns match on **label text**
- Comments: `mcli item get-updates <id> [--limit N]`
- Long description: `mcli item description <id>` → markdown

Reads are decoded for you: status → its label, date → an ISO string, numbers → a
number. You never parse monday's raw column JSON when reading.

## Write Items

```sh
mcli item create --board <id> [--group <id>] --name "..." [shorthands] [--col <id>=<json>]...
mcli item update <id> --board <id> [--name "..."]  [shorthands] [--col <id>=<json>]...
```

**Typed shorthands** spare you a column lookup. Each addresses the board's *single*
column of that type and is validated before anything is sent:

`--status Done` · `--date 2026-05-10` (or `2026-05-10T14:30`) · `--due` (alias of `--date`) · `--number 42` · `--text "..."` · `--checkbox true`

If the board has **0 or 2+** columns of that type, the command fails with `USAGE` and
names the candidates rather than choosing one. Use `--col <column_id>=<json>` there,
and for every type without a shorthand (people, link, dropdown, long_text, timeline…).

`--dry-run` prints the exact `column_values` that would be sent, and writes nothing.

### Many items at once

Pass `-` in place of `--name` / `<id>` to read rows from stdin — a JSON array or one
object per line, max 500 rows. This is one command against monday's rate limit
instead of N:

```sh
echo '[{"name":"Ship v1","status":"Done","due":"2026-05-10"},{"name":"Docs","number":3}]' \
  | mcli item create --board 123 -
echo '{"id":"456","status":"Done"}' | mcli item update --board 123 -
```

Create rows need `name`; update rows need `id`. Both also accept `group`, `cols`
(raw, as `{"<column_id>": <json>}`), and the shorthand keys `status`, `date`/`due`,
`number`, `text`, `checkbox`.

- **Every row is validated before the first request.** One bad status label fails the whole batch with `USAGE` and writes nothing.
- Output: `{"written":N,"failed":N,"verb":"created","items":[...],"errors":[{"index":i,"code":"...","message":"..."}]}`
- Exit 2 on partial failure. **Retry only the rows named in `errors[].index`** — item creation has no dedupe key, so re-sending the batch duplicates the rows that already succeeded.

### Move, lifecycle, notes

- `mcli item move <id> --to-group <id>` (same board) or `--to-board <id> --group <id>`
- `mcli item archive <id>` (recoverable) · `mcli item delete <id>` (**permanent**)
- Comment: `mcli item post-update <id> --body <text|-> [--parent <update_id>]`
- Description: `mcli item description <id> --set <md>` or `--set-file <path>`
- Subitem: `mcli item create --parent <item_id> --name "..."` — the response carries `board.id`, the board monday put subitems on. Pass that ID to `board column create` and to later `item update --board`.

## Column Values (write shapes)

Prefer a shorthand. Where none exists, `--col <column_id>=<json>` takes monday's raw
shape:

| Type | Write JSON |
|---|---|
| status | `{"label":"Done"}` |
| date | `{"date":"2026-05-10"}` |
| text, long_text | `"hello"` or `{"text":"hello"}` |
| numbers | `"42"` — quoted |
| checkbox | `{"checked":"true"}` |
| people | `{"personsAndTeams":[{"id":123,"kind":"person"}]}` |
| dropdown | `{"ids":[1,2]}` |
| link | `{"url":"https://...","text":"label"}` |
| timeline | `{"from":"2026-01-01","to":"2026-03-01"}` |

`""` clears a column. `null` is rejected — use `""`. Computed types (mirror, formula,
progress, dependency, auto_number) cannot be written directly.

## Errors: What To Do Next

stderr carries `{"error":{"code":"...","message":"..."}}` and the exit code follows:

| Code / exit | Meaning, and the next move |
|---|---|
| `USAGE` 1 | Your call is malformed — bad flag, ambiguous shorthand, invalid label. **Fix the call; never retry it unchanged.** |
| `API` 2 | monday rejected the request. Read the message: usually a permission problem or an invalid column value. |
| `NOT_FOUND` 2 | The ID does not exist, or this token cannot see it. Re-discover it; this is not transient. |
| `AUTH` 3 | Token missing, invalid, or expired. Not retryable — check `mcli auth status`. |
| `RATE_LIMITED` 4 | mcli already retried with backoff (up to 3×, honouring `Retry-After` / `reset_in`) and still failed. The complexity budget is **per minute**, so parallelism does not help: wait, then batch instead of looping per item. |
| `INTERNAL` 5 | Unexpected, and the fallback for any unclassified error. Retrying will not help; report it. |
| `DAEMON_REQUIRED` 6 | Start the daemon first (see Webhook Inbox). |
| `INTERRUPTED` 130 | Cancelled by SIGINT/SIGTERM. |

## Failures Worth Knowing About In Advance

These fail in ways the error message alone will not explain:

- **`board create` without `--workspace`** goes to the account's default workspace and often returns `API: User unauthorized to perform action`, which says nothing about workspaces. Pass `--workspace <id>` from `mcli workspace list`.
- **A `status` column created without `--defaults`** gets monday's stock labels, so a later `--status "<your label>"` fails validation. Define labels at creation time.
- **`item update` needs `--board`** even though the item ID is already unique.
- **A board created through the API has only a Name column** — no default status, date, or person columns. There is nothing to inherit; create what you need.
- **Shorthands describe a board's shape, not a convention.** On a board you did not create, a shorthand may be ambiguous (two `date` columns) or missing. Check `board column list` first, or address columns by ID.
- **A subitem's columns are not the parent board's columns.** Updating a subitem takes the *subitems* board ID.
- **`--col` takes JSON, not text:** `--col status='{"label":"Done"}'`, not `--col status=Done`.

## Output Modes & Shapes

`--json` (default when stdout is not a TTY) · `--pretty` (default on a TTY) · `--terse`
· `--csv`. Precedence: explicit flag > `mcli config set output-mode <mode>` > TTY
detection. **Pass `--json` explicitly** rather than relying on TTY detection.

| Pattern | Shape |
|---|---|
| List | `{"items":[...],"cursor":"..."}` — empty cursor means last page |
| Get | a single object |
| Write | the affected object |
| Delete / archive | `{"id":"...","name":"..."}`, plus `"state"` for items |
| Batch write | `{"written":N,"failed":N,"verb":"...","items":[...],"errors":[...]}` |
| Raw GraphQL | passthrough `{"data":...,"errors":...,"extensions":...}` |

## Build A Board (one-time setup)

1. `mcli board create --name "..." --kind public|private|share --workspace <id> [--empty] [--description "..."]` — `--empty` omits monday's sample *items*; either way an API-created board starts with just a Name column.
2. `mcli board group create --board <id> --name "..." [--color <hex>]`
3. `mcli board column create --board <id> --title "..." --type <type> [--defaults <json>] [--description "..."]`

Set `--description` when creating the column rather than making a second
`board column describe` call — a described column tells the next reader, human or
agent, what it means, and `board column list` / `board get` return it. Status
columns need their labels up front:

```sh
mcli board column create --board <id> --title "Stage" --type status \
  --defaults '{"labels":{"0":"Todo","1":"Doing","2":"Done"}}' \
  --description "Delivery stage; Done means shipped to production"
```

Common writable `--type` values: `text`, `long_text`, `numbers`, `status`, `dropdown`,
`date`, `timeline`, `people`, `checkbox`, `link`, `email`, `phone`, `location`,
`rating`, `hour`, `country`, `file`, `doc`, `board_relation`.

## Boards, Workspaces, Folders, Docs

- Boards: `mcli board list/get/create/rename/archive/delete`
- Groups: `mcli board group list/create/rename/archive/delete --board <id>`
- Columns: `mcli board column list/create/rename/describe/delete --board <id>`
- Workspaces: `mcli workspace list/get/create/update/delete` — `create` needs `--kind open|closed`
- Folders: `mcli folder list/create/rename/delete` — `--workspace <id>`, `--parent <id>`
- Docs: `mcli doc create --workspace <id> --name "..."` · `mcli doc read <id>` → markdown · `mcli doc write <id> --content <md>` or `--file <path|->` (replaces all content)
- `mcli me` — current user id, name, email, account, teams
- `mcli config get|set <key> [<value>]` · `mcli version`

## Semantic Layer (optional; worth it for repeated work)

Saved queries and mutations attach business names to operations, so later calls carry
no board or column IDs. **Check whether one already exists before doing anything by
hand:** `mcli query list` and `mcli mutation list` — if saved operations exist, prefer
them over raw commands.

```sh
mcli mutation save create_order --query 'mutation($board: ID!, $name: String!, $cols: JSON!) { create_item(board_id: $board, item_name: $name, column_values: $cols) { id } }'
mcli mutation run  create_order --var board=123 --var name="ORD-42" --var cols='{"status":{"label":"Draft"}}'
```

Variables declared `JSON` in the query are stringified on the wire, so write natural
JSON in `--var` with no double-encoding. Other `--var` values are auto-typed:
`42`→int, `true`→bool, `{...}`→JSON, otherwise string. `--vars-file <json>` supplies a
whole variables object.

Definitions live in `.mcli/queries/` and `.mcli/mutations/` (project-local and
version-controllable) or `~/.config/mcli/` with `--global`. Manage them with
`mcli query|mutation list` and `... delete <name>`.

## Escape Hatches

**Any API operation, still structured** — prefer this over raw GraphQL:

`mcli api list [--type query|mutation] [--no-builtins]` · `mcli api describe <op>` · `mcli api <op> --arg key=value...`

JSON args are auto-coerced. `--select "id,name,column_values{id,text}"` overrides the
selection set; `--dry-run` previews the request. The operation list comes from a
schema embedded at build time — if `mcli api` warns on stderr that it is stale, or an
operation you expect is missing, run `mcli schema refresh` (`mcli schema status`
reports which schema is in use and its age).

**Raw GraphQL**, when nothing above fits:

`mcli query '<graphql>' [--var k=v]...` · `-f <file>` · `-f -` for stdin · `mcli mutation '<graphql>' ...`

Output is passthrough `{"data":...,"errors":...}`: exit 0 when `errors` is absent,
exit 2 when present. Reference:
https://developer.monday.com/api-reference/reference/about-the-api-reference

## Webhook Inbox (react to changes instead of polling)

1. `mcli daemon start` — needs `cloudflared` in PATH, or pass `--url <public-url>` (`--port N` to move off 8420)
2. `mcli webhook create --board <id> --event <type>` — `mcli webhook events` lists types; `mcli webhook list [--board <id>]` and `mcli webhook delete <id>` manage them
3. `mcli notification count` is the cheap poll; then `mcli notification list [--unread] [--board <id>] [--event <t>] [--since <RFC3339>] [--limit N]`
4. `mcli notification ack <id>` or `mcli notification ack --all`

Commands that need the daemon fail with `DAEMON_REQUIRED` (exit 6) when it is not
running.
