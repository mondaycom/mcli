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

# Get board structure
mcli board get 9832181507

# List items with decoded column values
mcli item list --board 9832181507

# Create an item with column values
mcli item create --board 9832181507 --name "Ship feature" \
  --col 'status={"label":"Working on it"}' \
  --col 'due_date={"date":"2026-06-01"}'

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
mcli item find --board <id> --column <col> --value <text>   Find by column value
mcli item post-update <id> --body <text>                    Post a comment
mcli item get-updates <id> [--limit N]                      Read comments/updates
mcli item description <id> [--set <md>|--set-file <path>]   Read/write description

mcli doc read <id>                   Export document as markdown
mcli doc write <id> --content <md>   Replace document content

mcli query '<graphql>'               Raw GraphQL queries (inline, -f file, -f -)
mcli query save/list/run/delete      Saved query management

mcli mutation '<graphql>'            Raw GraphQL mutations (inline, -f file, -f -)
mcli mutation save/list/run/delete   Saved mutation management

mcli daemon start/stop/status        Background daemon (webhook receiver)
mcli webhook create/list/delete/events  Webhook registration management
mcli notification list/count/ack     Event inbox (poll for webhook events)

mcli skill                           Print LLM skill document
mcli version                         Print version
```

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
- Listens for webhook payloads on an HTTP port (default 6780)
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

On **write**, pass monday's raw column-value JSON via `--col <id>=<json>`:

```sh
mcli item create --board 123 --name "Task" \
  --col 'status={"label":"Done"}' \
  --col 'date={"date":"2026-05-10"}' \
  --col 'priority={"label":"High"}'
```

Use `mcli board column list --board <id>` to discover column IDs, types, and settings.

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

See [`examples/ecommerce-demo.md`](examples/ecommerce-demo.md) for a full walkthrough and [`examples/ecommerce-demo.sh`](examples/ecommerce-demo.sh) for a runnable script that sets up Products, Inventory, and Orders boards with a complete semantic layer.

## Development

```sh
make build         # Build binary to bin/mcli
make test          # Run tests with -race
make lint          # golangci-lint
make vet           # go vet
make fmt-check     # Check formatting
make schema        # Refresh monday.com GraphQL schema (requires token)
```

## Acknowledgments

Thanks to Witold Sosnowski (@wito) for the ideas behind the search, doc, item description, and item find commands.

## License

MIT
