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
- **Escape hatch** — `mcli query` gives raw GraphQL access for anything not covered by structured commands.

## Authentication

Token resolution (first match wins):

1. `--token <t>` flag
2. `MONDAY_API_TOKEN` environment variable
3. Stored credential (`mcli auth login`)

Credentials are stored in the OS keychain (macOS Keychain, Linux Secret Service) or an age-encrypted file. Never plaintext on disk.

## Commands

```
mcli auth login/logout/status        Authentication
mcli me                              Current user info

mcli workspace list/get/create/update/delete
mcli folder list/create/rename/delete

mcli board list/get/create/rename/archive/delete
mcli board group list/create/rename/archive/delete
mcli board column list/create/rename/describe/delete

mcli item list/get/create/update/move/archive/delete

mcli query '<graphql>'               Raw GraphQL (inline, -f file, -f -)
mcli query save/list/run/delete      Saved query management

mcli skill                           Print LLM skill document
mcli version                         Print version
```

Run `mcli <command> --help` for flags and usage on any command.

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

## Saved Queries

Prepare reusable GraphQL queries:

```sh
# Save
mcli query save weekly-report --query 'query($board: ID!) { 
  boards(ids: [$board]) { items_page(limit:100) { items { name column_values { id text } } } } 
}'

# Run with variables
mcli query run weekly-report --var board=9832181507

# Variables are auto-typed: 42→int, true→bool, {...}→JSON, else string
```

Saved queries live in `.mcli/queries/` (local, project-shareable) or `~/.config/mcli/queries/` (`--global`).

## Development

```sh
make build         # Build binary to bin/mcli
make test          # Run tests with -race
make lint          # golangci-lint
make vet           # go vet
make fmt-check     # Check formatting
make schema        # Refresh monday.com GraphQL schema (requires token)
```

## License

MIT
