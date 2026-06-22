# Phase 5: Search, Docs, Output Modes & Wrapper Promotion

## Overview

Promote high-value features from the Deno wrapper into the Go core binary, and add a token-efficient output mode for LLM consumers. After implementation, update the README and `skill` doc to reflect new commands.

## Context

The Deno wrapper (`mcli` shim) adds search, doc read/write, item description, item updates read, and item find on top of the Go binary (`_mcli`). These are the primary discovery and read-path commands for LLM-driven workflows. Promoting them eliminates the Deno runtime dependency and reduces latency.

The output system today has two modes: `ModeJSON` (non-TTY default) and `ModePretty` (TTY). LLM consumers pay per-token, so a `terse` mode (minimal formatting, no padding) and `csv` mode are needed.

## Phase 5a: Output Mode Infrastructure
**Status**: completed

**Goal**: Add `terse` and `csv` output modes with persistent config default.

### Tasks:
- [x] Add `output_mode` field to `Config` struct
- [x] Add `mcli config` command: `mcli config set output-mode <mode>` / `mcli config get output-mode`
- [x] Extend `OutputMode` enum: `ModeJSON`, `ModePretty`, `ModeTerse`, `ModeCSV`
- [x] Add `--terse` and `--csv` global flags to `rootCmd`
- [x] Update `resolveOutputMode` precedence: `--json` > `--csv` > `--terse` > `--pretty` > config default > TTY-detect
- [x] Add terse formatters to existing commands: `board list`, `board get`, `item list`, `item get`, `me`
- [x] Terse format spec: one line per entity, pipe or tab-separated key fields, no alignment padding
- [x] CSV format: standard RFC 4180, headers on first line
- [x] Unit tests for resolveOutputMode with all flag/config combinations

### Acceptance Criteria:
- [x] `AC-5a-1`: `mcli config set output-mode terse` persists; subsequent commands default to terse
- [x] `AC-5a-2`: `--json`, `--csv`, `--terse`, `--pretty` flags override config
- [x] `AC-5a-3`: Existing JSON/pretty behavior unchanged when no config set
- [x] `AC-5a-4`: Terse output for `item list` is ≤50% the token count of pretty output

## Phase 5b: Search Command
**Status**: pending

**Goal**: `mcli search <query> [-t boards|items|docs] [--limit N]`

### Tasks:
- [ ] Add `search.graphql` using monday's `search { boards(query:...) { results { ... } } items(...) docs(...) }` API
- [ ] Run genqlient codegen
- [ ] Implement `internal/cli/search.go` with all four output modes
- [ ] JSON: `{"boards":[...],"items":[...],"docs":[...]}`
- [ ] Pretty: grouped sections with aligned columns
- [ ] Terse: `board:12345:Board Name` / `item:67890:Item Name:board=12345` / `doc:111:Doc Name`
- [ ] CSV: type,id,name,context columns
- [ ] Register `newSearchCmd()` under `rootCmd` in `root.go`
- [ ] Unit tests with httptest fixtures

### Acceptance Criteria:
- [ ] `AC-5b-1`: `mcli search "foo" --json` returns boards/items/docs matching query
- [ ] `AC-5b-2`: `mcli search "foo" -t items` restricts to items only
- [ ] `AC-5b-3`: `mcli search "foo" --terse` produces one-line-per-result format

## Phase 5c: Item Find (server-side column filter)
**Status**: pending

**Goal**: `mcli item find --board <id> --column <col_id> --value <text>`

### Tasks:
- [ ] Add `ItemFind` GraphQL query: `boards(ids:[$boardId]) { items_page(query_params: {rules: [{column_id:$colId, compare_value:[$value], operator:contains_text}]}) { items { id name group { id title } } } }`
- [ ] Implement `internal/cli/item_find.go`
- [ ] People-column resolution: when column type is `people`, resolve name/email to compare_value via user lookup (depends on 5f)
- [ ] All four output modes
- [ ] Unit tests

### Acceptance Criteria:
- [ ] `AC-5c-1`: `mcli item find --board 123 --column status --value Done` returns matching items
- [ ] `AC-5c-2`: People-column lookup by email resolves correctly
- [ ] `AC-5c-3`: JSON output includes item id, name, group

## Phase 5d: Item Updates (read)
**Status**: pending

**Goal**: `mcli item updates <id> [--limit N]`

### Tasks:
- [ ] Add `ItemUpdates` GraphQL query: `items(ids:[$id]) { updates(limit:$limit) { id text_body created_at creator { id name } replies { id text_body created_at creator { id name } } } }`
- [ ] Implement as `newItemUpdatesCmd()` subcommand under `mcli item`
- [ ] Pretty: threaded display with creator + timestamp
- [ ] Terse: `update_id|creator_name|timestamp|first_line_of_body` per update
- [ ] JSON: full update objects
- [ ] CSV: id,creator,created_at,text_body
- [ ] Unit tests

### Acceptance Criteria:
- [ ] `AC-5d-1`: `mcli item updates 12345 --json` returns updates with replies
- [ ] `AC-5d-2`: `--limit 5` restricts result count
- [ ] `AC-5d-3`: Terse output is one line per update (replies indented or appended)

## Phase 5e: Item Description + Doc Read/Write
**Status**: pending

**Goal**: `mcli item description <id> [--set <md> | --set-file <path>]` and `mcli doc <id> [--set <md> | --set-file <path>]`

### Tasks:
- [ ] Add GraphQL operations:
  - `export_markdown_from_doc(docId: $id)` for reading
  - `set_item_description_content(item_id: $id, markdown: $md)` for writing item descriptions
  - `add_content_to_doc_from_markdown(docId: $id, markdown: $md)` for writing docs
  - `delete_doc_blocks(block_ids: $ids)` for clearing doc before rewrite
  - `docs(ids: [$id]) { id blocks { id type content } }` for resolving doc structure
- [ ] Implement `internal/cli/item_description.go`:
  - Read: fetch column_values → find DirectDocValue → extract objectId → export_markdown_from_doc
  - Write: call set_item_description_content with markdown
- [ ] Implement `internal/cli/doc.go`:
  - Read: resolve doc (by id or object_id) → export_markdown_from_doc
  - Write: resolve doc → delete existing blocks → add_content_to_doc_from_markdown
  - Flags: `--set <markdown_string>` or `--set-file <path>` (read file contents)
- [ ] Output: read returns raw markdown (all modes); write returns confirmation
- [ ] Unit tests

### Acceptance Criteria:
- [ ] `AC-5e-1`: `mcli item description 12345` outputs markdown content
- [ ] `AC-5e-2`: `mcli item description 12345 --set "# Hello"` updates the description
- [ ] `AC-5e-3`: `mcli doc 999` outputs markdown; `mcli doc 999 --set-file ./content.md` updates it
- [ ] `AC-5e-4`: Doc pagination works (>200 blocks fetched correctly)

## Phase 5f: Enhance `me` + User Resolution
**Status**: pending

**Goal**: Richer `me` output; reusable user lookup for `item find`.

### Tasks:
- [ ] Expand `Me` GraphQL query: add `email`, `account { id name }`, `teams { id name }`
- [ ] Add `MeExtended` query (or expand existing) to fetch `favorites { object { id type } }` and `boards(limit:10, order_by:used_at) { id name workspace { id name } }`
- [ ] Add `internal/api/users/resolve.go`: `ResolveByEmail(email) → id` and `ResolveByName(name) → id` using `users(emails:...)` / `users(name:...)` API
- [ ] Update `me` formatters:
  - Pretty: teams, favorites, recent boards (tabular)
  - Terse: `@id name <email> | teams: t1,t2 | recent: ^bid1,^bid2`
  - JSON: full object
- [ ] Unit tests for user resolution + me output

### Acceptance Criteria:
- [ ] `AC-5f-1`: `mcli me --json` includes email, account, teams
- [ ] `AC-5f-2`: `mcli me` (pretty) shows recent boards and favorites
- [ ] `AC-5f-3`: User resolution by email works for item find people columns

## Phase 5g: README + Skill Doc Update
**Status**: pending

**Goal**: Update README.md and `skill` command output to document all new commands and output modes.

### Tasks:
- [ ] Update README.md:
  - Add search command to feature list
  - Add doc read/write commands
  - Add item description, item updates, item find
  - Document output modes (--json, --terse, --csv, --pretty) and config
  - Update installation/usage sections if needed
- [ ] Update `skillDoc` constant in `internal/cli/skill.go`:
  - Add search section
  - Add doc management section
  - Add item description and item updates sections
  - Add item find section
  - Document output mode config
- [ ] Verify `mcli skill` output renders correctly

### Acceptance Criteria:
- [ ] `AC-5g-1`: README documents all new commands with usage examples
- [ ] `AC-5g-2`: `mcli skill` output includes search, doc, description, updates, find
- [ ] `AC-5g-3`: Output modes section explains terse format for LLM consumers

## Dependencies

```
5a (output infra) ──→ 5b, 5c, 5d, 5e, 5f (all consume output modes)
5f (user resolve) ──→ 5c (item find needs people-column resolution)
5b, 5d, 5e can proceed in parallel once 5a lands
5g (docs) ──→ all other phases complete
```

## Out of Scope (remain in Deno wrapper)

- DuckDB caching layer (consider SQLite in a future phase)
- Rich board grid formatter (complex layout logic, still iterating)
- `setup-token` interactive wizard (already covered by `mcli auth login`)
- `ensure-label` (niche, rarely called)
