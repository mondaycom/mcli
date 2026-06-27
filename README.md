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
mcli item find --board <id> --column <col> --value <text>   Find by column value
mcli item post-update <id> --body <text>                    Post a comment
mcli item get-updates <id> [--limit N]                      Read comments/updates
mcli item description <id> [--set <md>|--set-file <path>]   Read/write description

mcli doc read <id>                   Export document as markdown
mcli doc write <id> --content <md>   Replace document content

mcli api list [--type query|mutation]        Browse all ~250 API operations from the schema
mcli api describe <operation|type>          Inspect signature and argument types
mcli api <operation> [--arg k=v]...         Execute any operation; JSON args auto-coerced

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
mcli config set routing-key arnonro   # enable local-api-proxy routing
mcli config set routing-key ""        # disable
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
mcli config set routing-key arnonro   # add baggage: routingKey=arnonro header (local-api-proxy)
mcli config set routing-key ""        # clear routing key
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

### Debugging with local-api-proxy

[local-api-proxy](~/projects/local-api-proxy) lets you intercept API calls locally. Set a routing key so requests are routed through your local mirror instance:

```sh
# Persist for the session (stored in ~/.config/mcli/config.yaml)
mcli config set routing-key <your-username>

# Or per-command via env
MONDAY_ROUTING_KEY=<your-username> bin/mcli board list

# Clear when done
mcli config set routing-key ""
```

Combine with staging for local end-to-end debugging:

```sh
MONDAY_API_URL=https://api.mondaystaging.com/v2 \
MONDAY_ROUTING_KEY=<your-username> \
bin/mcli board list
```

## Acknowledgments

Thanks to Witold Sosnowski (@wito) for the ideas behind the search, doc, item description, and item find commands.

## License

MIT
