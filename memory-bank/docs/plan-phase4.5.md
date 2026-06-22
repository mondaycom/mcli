# Phase 4.5: Board Structure, Workspace, Delete + Skill Command

## Overview

Add the fundamental structural commands an LLM agent needs to fully manage a monday.com workspace hierarchy: workspaces, folders, board structure (groups/columns), delete/archive for boards+items, and the `mcli skill` command that outputs the LLM skill document.

Monday's hierarchy: Account → Workspace → Folder → Board → Group/Column → Item → Subitem.

## Phase 4.5a: Workspace + Folder commands
**Status**: completed

### Tasks:
- [x] **GraphQL queries/mutations**: `workspaces` (list), `create_workspace`, `update_workspace`, `delete_workspace`, `folders` (list), `create_folder`, `update_folder`, `delete_folder`
- [x] **`mcli workspace list`** `[--kind open|closed|template]`
- [x] **`mcli workspace get <id>`**
- [x] **`mcli workspace create`** `--name <name> --kind open|closed [--description <text>]`
- [x] **`mcli workspace update <id>`** `[--name] [--description] [--kind]`
- [x] **`mcli workspace delete <id>`**
- [x] **`mcli folder list`** `[--workspace <id>]`
- [x] **`mcli folder create`** `--name <name> [--workspace <id>] [--parent <folder-id>]`
- [x] **`mcli folder rename <id> <name>`**
- [x] **`mcli folder delete <id>`**
- [x] Unit tests per command (httptest fixtures)
- [x] Live smoke test at least workspace list + folder list

### Acceptance Criteria:
- [x] `AC-4.5a-1`: `mcli workspace list --json` returns workspaces with id/name/kind
- [x] `AC-4.5a-2`: `mcli workspace create + get + update + delete` round-trip works
- [x] `AC-4.5a-3`: `mcli folder list --workspace <id> --json` returns folders
- [x] `AC-4.5a-4`: All commands have unit tests; `go test -race` green

## Phase 4.5b: Board delete/rename/archive + group CRUD + column CRUD
**Status**: completed

### Tasks:
- [x] **GraphQL mutations**: `delete_board`, `archive_board`, `update_board(name)`, `create_group`, `delete_group`, `archive_group`, `update_group(title)`, `create_column`, `delete_column`, `change_column_title`, `change_column_metadata(description)`
- [x] **`mcli board rename <id> <name>`**
- [x] **`mcli board delete <id>`**
- [x] **`mcli board archive <id>`**
- [x] **`mcli board group list --board <id>`**
- [x] **`mcli board group create --board <id> --name <name> [--color <hex>]`**
- [x] **`mcli board group delete --board <id> --group <id>`**
- [x] **`mcli board group archive --board <id> --group <id>`**
- [x] **`mcli board group rename --board <id> --group <id> --name <name>`**
- [x] **`mcli board column list --board <id>`**
- [x] **`mcli board column create --board <id> --title <title> --type <type> [--defaults <json>]`**
- [x] **`mcli board column delete --board <id> --column <id>`**
- [x] **`mcli board column rename --board <id> --column <id> --title <title>`**
- [x] **`mcli board column describe --board <id> --column <id> --text <description>`**
- [x] Unit tests per command
- [x] Live smoke: create group, rename, archive; create column, rename, set description, delete

### Acceptance Criteria:
- [x] `AC-4.5b-1`: `mcli board delete` returns deleted board id+name; board no longer appears in list
- [x] `AC-4.5b-2`: Group CRUD round-trip: create → list → rename → archive
- [x] `AC-4.5b-3`: Column CRUD round-trip: create → list → rename → describe → delete
- [x] `AC-4.5b-4`: `board group list` and `board column list` return targeted subsets (not full board-get payload)
- [x] `AC-4.5b-5`: All commands unit-tested; `go test -race` green

## Phase 4.5c: Item delete/archive + `mcli skill`
**Status**: completed

### Tasks:
- [x] **GraphQL mutations**: `delete_item`, `archive_item`
- [x] **`mcli item delete <id>`**
- [x] **`mcli item archive <id>`**
- [x] **`mcli skill`** (alias: `mcli describe`) — generates and outputs the full LLM skill Markdown document describing all commands, their flags, output shapes, error codes, and primary-use-case workflows
- [x] Skill doc content: command reference table, JSON output schemas, workflow examples (create board → add columns → add items → update status), error handling patterns
- [x] CI check: `mcli skill` output is deterministic (regenerate + diff = no change)
- [x] Unit tests for delete/archive commands

### Acceptance Criteria:
- [x] `AC-4.5c-1`: `mcli item delete <id> --json` returns `{"id":"...", "name":"..."}` of deleted item
- [x] `AC-4.5c-2`: `mcli item archive <id> --json` returns archived item; item state becomes archived
- [x] `AC-4.5c-3`: `mcli skill` outputs valid Markdown with all registered commands
- [x] `AC-4.5c-4`: `mcli describe` is an alias that produces identical output to `mcli skill`
- [x] `AC-4.5c-5`: CI drift check passes (skill doc regeneration produces no diff)

## Design Notes

- **Destructive ops (delete)**: Per axiom A12, no interactive confirmation. Return deleted resource's id+name in response so LLM can verify.
- **Archive vs Delete**: Both exposed. Archive = soft-delete (recoverable). Help text should guide LLMs to prefer archive.
- **Column types**: `create_column --type` validates against the ColumnType enum. Help text lists valid types.
- **Column defaults**: JSON-passthrough pattern (same as `--col` on item write). The `defaults` field is type-specific config.
- **`mcli skill`**: Self-describing output per axiom A2. This is the single artifact an LLM loads to learn the full CLI. Must stay in sync with actual commands — generated, not hand-written.
- **Workspace update**: Single command with optional flags rather than separate rename/describe commands — fewer commands for the LLM to learn, one API call covers multiple changes.
