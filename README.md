# mcli

A command-line interface for monday.com's GraphQL API. Single static binary, structured JSON output, designed for both human and LLM-agent use.

## Install

```sh
# From source
go install github.com/mondaycom/mcli/cmd/mcli@latest

# Or build locally
make build    # → bin/mcli
```

## Quick Start

```sh
# Authenticate (one-time, stores in OS keychain)
mcli auth login --token <your-monday-api-token>

# Verify
mcli me

# List your boards
mcli board list

# Get board structure (add --items for the first page of items with column values)
mcli board get 9832181507
mcli board get 9832181507 --items --items-limit 50

# List items with decoded column values (add --subitems to include each item's subitems)
mcli item list --board 9832181507
mcli item list --board 9832181507 --subitems

# Get a single item, including its subitems with column values
mcli item get 1234567890 --subitems

# Create an item with typed column shorthands
mcli item create --board 9832181507 --name "Ship feature" \
  --status "Working on it" --due 2026-06-01

# Or with raw column JSON, for any column type
mcli item create --board 9832181507 --name "Ship feature" \
  --col 'status={"label":"Working on it"}' \
  --col 'due_date={"date":"2026-06-01"}'

# Create or update many items in one command (rate-limit friendly)
echo '[{"name":"Ship v1","status":"Done"},{"name":"Write docs","due":"2026-06-10"}]' \
  | mcli item create --board 9832181507 -

# Run a raw GraphQL query
mcli query 'query { me { id name } }'
```

## Design Principles

- **JSON by default** — all commands emit structured JSON to stdout. Use `--pretty` for human-readable output.
- **LLM-native** — designed to be driven by LLM agents. No interactive prompts on the read path. Self-describing via `mcli skill`.
- **Single binary** — no runtime dependencies. Build once, deploy anywhere.
- **Coherent surface** — normalizes monday.com API quirks into a consistent CLI vocabulary.
- **Semantic layer** — saved queries/mutations create a business-level API over boards. LLMs operate in domain terms, not GraphQL.
- **Escape hatch** — `mcli query` / `mcli mutation` give raw GraphQL access for anything not covered by structured commands.

## Authentication

Token resolution (first match wins):

1. `--token <t>` flag
2. `MONDAY_API_TOKEN` environment variable
3. Stored credential (`mcli auth login`)

Credentials are stored in the OS keychain (macOS Keychain, Linux Secret Service) or an age-encrypted file. Never plaintext on disk.

**Environment overrides** (take precedence over config file):

| Variable | Purpose | Default |
|----------|---------|---------|
| `MONDAY_API_TOKEN` | API token | — |
| `MONDAY_API_URL` | API endpoint | `https://api.monday.com/v2` |
| `MONDAY_API_VERSION` | API version header | `2026-07` |
| `MONDAY_ROUTING_KEY` | Routing key for local-api-proxy debugging | — |

## Commands

```
mcli auth login/logout/status        Authentication
mcli me                              Current user info (id, name, email, account, teams)

mcli search <query> [-t boards|items|docs] [--limit N]   Cross-entity search

mcli workspace list/get/create/update/delete
mcli folder list/create/rename/delete

mcli board list/get/create/rename/archive/delete
mcli board group list/create/rename/archive/delete
mcli board column list/create/rename/describe/delete

mcli item list/get/create/update/move/archive/delete
mcli item create/update ... --status/--due/--date/--number/--text/--checkbox
                                                            Typed column shorthands
mcli item create/update --board <id> -                      Batch write, rows on stdin
mcli item find --board <id> --column <col> --value <text>   Find by column value
mcli item post-update <id> --body <text>                    Post a comment
mcli item get-updates <id> [--limit N]                      Read comments/updates
mcli item description <id> [--set <md>|--set-file <path>]   Read/write description

mcli doc read <id>                   Export document as markdown
mcli doc write <id> --content <md>   Replace document content

mcli api list [--type query|mutation]        Browse all ~250 API operations from the schema
mcli api describe <operation|type>          Inspect signature and argument types
mcli api <operation> [--arg k=v]...         Execute any operation; JSON args auto-coerced

mcli schema status                   Report which schema is in use and how old it is
mcli schema refresh                  Fetch the live schema with your token and cache it

mcli query '<graphql>'               Raw GraphQL queries (inline, -f file, -f -)
mcli query save/list/run/delete      Saved query management

mcli mutation '<graphql>'            Raw GraphQL mutations (inline, -f file, -f -)
mcli mutation save/list/run/delete   Saved mutation management

mcli daemon start/stop/status        Background daemon (webhook receiver)
mcli webhook create/list/delete/events  Webhook registration management
mcli notification list/count/ack     Event inbox (poll for webhook events)

mcli config set <key> <value>        Persist a configuration value
mcli config get <key>                Read a configuration value

mcli skill                           Print LLM skill document
mcli version                         Print version
```

## Configuration

All config keys are stored in `~/.config/mcli/config.yaml` (or `$XDG_CONFIG_HOME/mcli/config.yaml`).

| Key | Values | Description |
|-----|--------|-------------|
| `output-mode` | `default`, `json`, `pretty`, `terse`, `csv` | Default output format; `default` auto-detects (pretty on TTY, JSON otherwise) |
| `api-url` | any URL, or `""` to reset | monday.com API endpoint (e.g. `https://api.mondaystaging.com/v2`) |
| `api-version` | `YYYY-MM` (e.g. `2026-07`), named (e.g. `dev`), or `default` to reset | monday.com API version; triggers schema fetch and cache on set |
| `routing-key` | any string, or `""` to clear | Adds `baggage: routingKey=<v>` header for local-api-proxy debugging |

```sh
mcli config set output-mode terse
mcli config set api-url https://api.mondaystaging.com/v2   # point at staging
mcli config set api-url ""                                  # reset to production
mcli config set api-version 2026-08   # fetches and caches schema
mcli config set api-version default   # revert to built-in schema
mcli config set routing-key <your-key>   # enable local API proxy routing
mcli config set routing-key ""           # disable
mcli config get api-url
```

Environment variables override config file values — see the table in [Authentication](#authentication).

## Dynamic API (`mcli api`)

Every monday.com API operation is available without writing GraphQL. The schema (~250 operations) is embedded in the binary and can be refreshed for a specific API version.

```sh
# Browse all operations
mcli api list
mcli api list --type mutation | grep team

# Inspect an operation or type
mcli api describe create_notification
mcli api describe Board

# Execute any operation — JSON arguments are auto-coerced, no double-encoding needed
mcli api create_notification \
  --arg user_id=12345 --arg text="Hello" \
  --arg target_id=67890 --arg target_type=Project

mcli api change_column_value \
  --arg board_id=123 --arg item_id=456 \
  --arg column_id=status --arg value='{"label":"Done"}'

# Preview generated GraphQL without executing
mcli api boards --arg limit=5 --dry-run

# Override the auto-selected return fields
mcli api boards --arg limit=5 --select "id,name,state"
```

**Config keys** for API behaviour:

```sh
mcli config set api-version 2026-08   # fetch + cache schema for this version; "default" to reset
mcli config set routing-key <your-key>   # add baggage: routingKey=<your-key> header
mcli config set routing-key ""           # clear routing key
```

### Keeping the schema fresh

The embedded schema is a snapshot taken when the binary was built, so a long-lived
install gradually falls behind the live API. `mcli schema refresh` introspects the API
with your own token and caches the result in `~/.config/mcli/schema.graphql`, which takes
precedence over the embedded copy — no rebuild, no release wait.

```sh
mcli schema status                        # source, api-version, age, type count
mcli schema refresh                       # fetch + cache; prints the type delta
mcli schema refresh --api-version 2026-08 # one-off fetch, not persisted to config
```

`mcli api` warns on stderr (never stdout) when the schema in use is more than 30 days
old. Refreshing is always explicit: no mcli command fetches a schema behind your back.
To drop the cache and go back to the embedded schema, delete the cached file or run
`mcli config set api-version default`.

## Output Modes

Control output format with a global flag or persist a default:

```sh
mcli config set output-mode terse   # persist preferred mode
mcli config set output-mode default # reset to auto-detect
```

| Flag | Description |
|------|-------------|
| `--json` | Machine-readable JSON (default when stdout is not a TTY) |
| `--pretty` | Human-readable tables (default when stdout is a TTY) |
| `--terse` | One-line-per-result compact output |
| `--csv` | Comma-separated values with header row |

Run `mcli <command> --help` for flags and usage on any command.

## Daemon & Webhooks

mcli includes a background daemon that receives monday.com webhooks and stores them as an inbox for LLM agents to poll.

### Setup

```sh
# Start the daemon (foreground — it opens a Cloudflare Quick Tunnel automatically)
mcli daemon start

# Or supply your own public URL (skips tunnel)
mcli daemon start --url https://your-server.example.com
```

The daemon:
- Listens for webhook payloads on an HTTP port (default 8420)
- Exposes a Unix socket IPC for CLI commands
- Opens a Cloudflare Quick Tunnel for a public URL (requires `cloudflared` in PATH)
- Re-registers webhooks automatically when the tunnel URL changes

### Register Webhooks

```sh
# Register a webhook for item creation events on a board
mcli webhook create --board 9832181507 --event create_item

# List registered webhooks
mcli webhook list

# List supported event types
mcli webhook events

# Remove a webhook
mcli webhook delete <webhook-id>
```

### Poll Notifications

```sh
# Check if there are unread events
mcli notification count

# List unread events
mcli notification list --unread

# Filter by board or event type
mcli notification list --board 9832181507 --event change_column_value

# Acknowledge (mark read)
mcli notification ack <event-id>
mcli notification ack --all
```

All notification/webhook commands require the daemon to be running. If it isn't, they return a `DAEMON_REQUIRED` error with exit code 6.

## LLM Integration

mcli is designed as a tool for LLM agents to manage monday.com programmatically.

```sh
# Load the skill document into your LLM context
mcli skill
```

This outputs a concise goal-oriented guide that tells the LLM what commands exist and how to accomplish common tasks. The LLM then discovers exact flags via `--help` as needed.

## Column Values

On **read**, column values are decoded to human-readable form (status labels, ISO dates, etc.).

On **write** there are two paths: typed shorthands for the common types, and raw
`--col <id>=<json>` for everything else.

### Typed shorthands

| Flag | Column type | Accepts |
|------|-------------|---------|
| `--status <label>` | `status` | a label configured on the column, matched case-insensitively |
| `--date` / `--due` | `date` | `2026-05-10`, `2026-05-10T14:30`, or an RFC3339 timestamp (converted to UTC) |
| `--number <n>` | `numbers` | any number; empty string clears the column |
| `--text <s>` | `text` | any string; empty string clears the column |
| `--checkbox <b>` | `checkbox` | `true`/`false`, `yes`/`no`, `1`/`0` |

```sh
mcli item create --board 123 --name "Task" --status Done --due 2026-05-10 --number 3
mcli item update 456 --board 123 --status "Working on it"
```

A shorthand carries no column id: it addresses **the board's single column of that
type**, looked up live. That is a deliberate trade:

- If the board has no column of the type, or more than one, the shorthand errors and
  names the candidates. It never picks one for you — writing a correct value to the
  wrong column is a silent, expensive mistake.
- Values are validated before anything is sent. This matters most for `--status`: every
  mcli item mutation sends `create_labels_if_missing`, so an unvalidated typo would
  quietly add a new label to the board instead of failing. A bad label is rejected with
  the column's real labels listed.
- Using a shorthand costs one extra request (the board's column list) per command — or
  per *batch*, not per row.
- `--date` and `--due` are the same flag; passing both is an error.

Shorthands are not available with `--parent` (a subitem lives on its own board, which
would need a second lookup) — use `--col` there.

### Raw column JSON

`--col <id>=<json>` writes any column, including types with no shorthand:

```sh
mcli item create --board 123 --name "Task" \
  --col 'status={"label":"Done"}' \
  --col 'date={"date":"2026-05-10"}' \
  --col 'people={"personsAndTeams":[{"id":123,"kind":"person"}]}'
```

Repeated `--col` flags are order-preserving; if the same column id appears twice the
last value wins. A shorthand and a `--col` that target the same column is an error, not
a precedence rule.

Use `mcli board column list --board <id>` to discover column IDs, types, and settings.

## Batch Writes

`mcli item create --board <id> -` and `mcli item update --board <id> -` read rows from
stdin instead of flags — a JSON array or one JSON object per line (NDJSON), up to 500
rows. Create rows need `name`; update rows need `id`. Both accept `group`, `cols`, and
the same typed shorthands as flags.

```sh
# JSON array
echo '[{"name":"Ship v1","status":"Done","due":"2026-05-10"},
       {"name":"Write docs","number":3,"cols":{"text_9":"draft"}}]' \
  | mcli item create --board 123 -

# NDJSON, e.g. straight out of jq
mcli item list --board 999 | jq -c '.items[] | {id, status:"Done"}' \
  | mcli item update --board 123 -
```

Row fields are JSON scalars, so `"number": 42` and `"number": "42"` both work.

**Validation happens up front.** Every row is parsed, checked, and encoded before the
first request fires, so a bad status label in row 40 fails the whole batch with `USAGE`
and nothing is written. Unknown row fields are rejected too — a mistyped `"columns"`
instead of `"cols"` would otherwise drop the row's values and report success.

**Rows are sent sequentially.** monday's rate limit is a complexity budget per minute,
so parallelism would not raise throughput, only reach the ceiling sooner.

Output is a single JSON object:

```json
{
  "written": 2,
  "failed": 1,
  "verb": "created",
  "items": [ ... ],
  "errors": [{"index": 1, "code": "API", "message": "column not found"}]
}
```

**On partial failure the exit code is 2 and you should retry only the rows named in
`errors[].index`.** Item creation has no dedupe key, so re-sending the whole payload
duplicates every row that succeeded.

`--dry-run` prints the exact `column_values` that would be sent for each row without
writing anything. It works on single-item create/update too, and needs no API access at
all unless a shorthand has to be resolved:

```sh
echo '{"name":"Check me","status":"Done"}' | mcli item create --board 123 - --dry-run
```

## Fetching nested data in one call

Batch writes save round-trips on the write path; on the read path, several commands can pull related records with their decoded column values in a single request:

| Command | Flag | Adds |
|---------|------|------|
| `board get <id>` | `--items` | first page of items (`items[]`, `items_cursor`); `--items-limit` (default 25, max 500), `--items-cursor` |
| `item get <id>` | `--subitems` | each subitem's column values; `--subitem-count` caps the count (default 25) and sets `subitems_truncated` when it drops rows |
| `item list --board <id>` | `--subitems` | each item's subitems with column values (JSON output only) |

Without these flags the queries stay cheap — the extra selections are `@include`-gated so default cost is unchanged. Nested column values decode the same way as top-level ones.

## Saved Queries & Mutations

Prepare reusable GraphQL operations — save once, run with just variables:

```sh
# Save a query
mcli query save weekly-report --query 'query($board: ID!) { 
  boards(ids: [$board]) { items_page(limit:100) { items { name column_values { id text } } } } 
}'

# Save a mutation
mcli mutation save create-task --query 'mutation($board: ID!, $name: String!, $cols: JSON!) {
  create_item(board_id: $board, item_name: $name, column_values: $cols) { id }
}'

# Run with variables
mcli query run weekly-report --var board=9832181507
mcli mutation run create-task --var board=9832181507 --var name="Ship feature" \
  --var cols='{"status":{"label":"Working on it"}}'

# Variables are auto-typed: 42→int, true→bool, {...}→JSON, else string
# Variables declared as JSON in the query are auto-stringified (no double-encoding needed)

# Management
mcli query list / mcli mutation list
mcli query delete <name> / mcli mutation delete <name>
```

Saved operations live in `.mcli/queries/` and `.mcli/mutations/` (local, project-shareable) or `~/.config/mcli/queries/` / `~/.config/mcli/mutations/` (`--global`).

## Semantic Layer

Saved queries and mutations can form a **business-level API** over your monday.com boards. Instead of working with board IDs, column IDs, and GraphQL, you define domain-named operations like `create_order`, `list_products`, or `update_inventory` — then interact entirely in business terms.

This is especially powerful for LLM agents: the conversation stays at "add 3 widgets to the order" rather than "create a subitem on board 1234 with column xyz set to 3".

**Pattern:**

1. Set up boards and columns (one-time)
2. Save domain-named queries/mutations that encode the board structure
3. Operate exclusively via `mcli query run <name>` / `mcli mutation run <name>`

```sh
# After setup, an LLM agent just needs:
mcli query run list_products --var boardId=123
mcli mutation run create_order --var board=456 --var name="ORD-99" \
  --var cols='{"customer":"Acme Corp","status":{"label":"Draft"}}'
mcli mutation run add_order_line --var parent=789 --var name="Widget x2" --var cols='{}'
mcli mutation run update_order_status --var board=456 --var item=789 \
  --var cols='{"status":{"label":"Ordered"}}'
```

Each domain below has a full walkthrough (`.md`) and a runnable script (`.sh`) that
creates the boards, defines a semantic layer, and exercises the workflow end to end:

| Domain | Boards | Walkthrough |
|---|---|---|
| E-commerce | Products, Inventory, Orders | [`examples/ecommerce-demo.md`](examples/ecommerce-demo.md) · [`.sh`](examples/ecommerce-demo.sh) |
| CRM | Companies, Contacts, Deals | [`examples/crm-demo.md`](examples/crm-demo.md) · [`.sh`](examples/crm-demo.sh) |
| Project portfolio | Portfolios, Projects, Milestones | [`examples/portfolio-demo.md`](examples/portfolio-demo.md) · [`.sh`](examples/portfolio-demo.sh) |

They also exercise the write ergonomics against real board shapes: the e-commerce demo
seeds inventory with one batch write, and the portfolio demo shows the case a shorthand
*cannot* serve — a board with two `date` columns, where `--col` takes over.

## Development

```sh
make build         # Build binary to bin/mcli
make test          # Run tests with -race
make lint          # golangci-lint
make vet           # go vet
make fmt-check     # Check formatting
make schema        # Refresh monday.com GraphQL schema (requires token)
```

`make schema` rewrites `schema/monday.graphql` and stamps `schema/fetched_at.txt` with
today's date. That stamp is what `mcli schema status` reports as the embedded schema's
age, so never edit either file by hand.

### Before cutting a release

```sh
make schema          # refresh the embedded schema + provenance stamp
go generate ./...    # regenerate genqlient code against the new schema
make test lint vet   # must be green
```

Shipping a release without refreshing means every user starts out with a schema as old
as the last refresh, and sees the staleness warning sooner.

### Running against staging

```sh
# Persistent (survives new shells)
mcli config set api-url https://api.mondaystaging.com/v2
MONDAY_API_TOKEN=<staging-token> bin/mcli board list
mcli config set api-url ""   # reset when done

# Or per-command via env
MONDAY_API_URL=https://api.mondaystaging.com/v2 \
MONDAY_API_TOKEN=<staging-token> \
bin/mcli board list
```

### Debugging with a local API proxy

mcli supports routing requests through a local API proxy via a routing key. When set, every request includes a `baggage: routingKey=<value>` header that a compatible proxy can use to intercept or route the call.

```sh
# Persist for the session (stored in ~/.config/mcli/config.yaml)
mcli config set routing-key <your-key>

# Or per-command via env
MONDAY_ROUTING_KEY=<your-key> bin/mcli board list

# Clear when done
mcli config set routing-key ""
```

Combine with a staging endpoint for local end-to-end debugging:

```sh
MONDAY_API_URL=https://api.mondaystaging.com/v2 \
MONDAY_ROUTING_KEY=<your-key> \
bin/mcli board list
```

## Acknowledgments

Thanks to Witold Sosnowski (@wito) for the ideas behind the search, doc, item description, and item find commands.

## License

MIT
