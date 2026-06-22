# Plan: Dynamic GraphQL API Commands + LLM Skill Exposure

**Status**: IN PROGRESS (Phases 1–4 complete, Phase 5 deferred)
**Created**: 2026-06-21  
**Updated**: 2026-06-22  
**Branch**: `worktree-feat+dynamic-api`  
**Depends on**: Phase 5 complete (search, docs, output modes)

## Motivation

mcli covers ~50 typed operations via hand-coded commands. Monday.com's API exposes ~90 query fields and ~160 mutation fields. Users (and LLM agents) needing uncovered operations must compose raw GraphQL manually via `mcli query`/`mcli mutation`. This plan adds a dynamic `mcli api` namespace that makes **every** API operation discoverable and executable without writing GraphQL.

Reference: [kevinhury/monday-cli](https://github.com/kevinhury/monday-cli) — generates CLI commands from schema. We go further: embedded schema, auto-selection sets, JSON coercion, and LLM skill integration.

## Design Principles

1. **Zero GraphQL knowledge required** — users pass args by name, get JSON back
2. **Friendly JSON handling** — Monday's `JSON` scalar requires stringified JSON on the wire (e.g. `column_values` must be `"{\"status\":{\"label\":\"Done\"}}"` not `{"status":{"label":"Done"}}`). The dynamic commands MUST auto-stringify any argument declared as `JSON` type in the schema, exactly like `coerceJSONVars` does today for `mcli query`. Users/LLMs pass natural JSON; the CLI handles the double-encoding internally.
3. **Built-ins remain canonical** — `mcli api` is additive; existing commands keep their ergonomic flags and formatted output
4. **LLM-first discovery** — agents can enumerate operations, read signatures, and invoke without human assistance
5. **Single binary, no network** — schema is `//go:embed`'d from the committed SDL

## Architecture

```
schema/monday.graphql (committed, refreshable via `make schema`)
       │
       ▼  go:embed + gqlparser v2
internal/api/schema/             ← NEW package
  schema.go                      load/cache parsed AST
  selection.go                   auto-generate selection sets
       │
       ▼
internal/cli/api.go              ← NEW: dynamic cobra command tree
       │
       ├─ mcli api <field_name> [--arg k=v]... [--select ...]
       ├─ mcli api list [--type query|mutation]
       └─ mcli api describe <field_or_type_name>
```

## JSON Coercion Detail

The existing `coerceJSONVars` regex-parses variable declarations to find `$var: JSON` and stringifies map/slice values. For the dynamic commands, we have **schema-level type info** — much better:

```
For each argument of the target field:
  1. Resolve its type from the parsed AST
  2. If the leaf type is the `JSON` scalar:
     - Accept the user's value as natural JSON (object, array, or string)
     - If it's already a string (user passed a raw string), use as-is
     - If it's a map/slice (parsed from --arg val='{...}'), json.Marshal → string
  3. All other scalars: use existing parseVarValue auto-typing (int, bool, etc.)
```

This means: `--arg column_values='{"status":{"label":"Done"}}'` just works. No double-quoting, no escape gymnastics. The CLI knows from the schema that `column_values` is `JSON` and handles stringification.

## Phases

### Phase 1: Schema Package (foundation)
**Status**: completed  
**Effort**: ~1 day  
**Delivers**: Reusable parsed schema access for the whole codebase

New `internal/api/schema/` package:

| Function | Purpose |
|----------|---------|
| `Load() *ast.Schema` | Parse embedded SDL via gqlparser, cache result |
| `QueryFields() []FieldDef` | Root Query fields (name, description, args, return type) |
| `MutationFields() []FieldDef` | Root Mutation fields |
| `TypeDef(name string) *TypeInfo` | Lookup any type's fields/args/enums/description |
| `IsJSONScalar(argType *ast.Type) bool` | Resolve whether an arg needs JSON coercion |
| `DefaultSelection(typeName string, depth int) string` | Generate `{ id name ... }` for a return type |

```go
// FieldDef is the information needed to register a dynamic command.
type FieldDef struct {
    Name        string
    Description string
    Args        []ArgDef
    ReturnType  string    // e.g. "Board", "[Item]!", "JSON"
    IsQuery     bool      // false = mutation
}

type ArgDef struct {
    Name         string
    Description  string
    TypeString   string   // e.g. "ID!", "[String]", "JSON"
    IsRequired   bool
    IsJSON       bool     // true if leaf scalar is JSON
    DefaultValue string   // "" if none
}
```

**Embed strategy**:
```go
//go:embed schema/monday.graphql
var schemaSDL string
```

The embed directive lives in `internal/api/schema/embed.go` (or similar) and references the repo-root schema via a relative path from the module root.

---

### Phase 2: Dynamic Command Registration (`mcli api`)
**Status**: completed  
**Effort**: ~2-3 days  
**Delivers**: Every API operation executable via CLI

> **Implementation note**: Used `cobra.ArbitraryArgs` + `RunE` on the `api` parent (not 250 dynamic subcommands) to avoid startup cost and cobra routing complexity. `list` and `describe` are static subcommands.

New `internal/cli/api.go`:

**Registration strategy** — lazy to avoid startup cost:
```go
func newAPICmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "api",
        Short: "Execute any monday.com API operation by name",
    }
    // Subcommands registered only when 'api' is actually invoked
    cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
        registerDynamicCommands(cmd)
        return nil
    }
    // Static subcommands always present
    cmd.AddCommand(newAPIListCmd())
    cmd.AddCommand(newAPIDescribeCmd())
    return cmd
}
```

**Each dynamic command**:

```
mcli api create_notification --arg user_id=12345 --arg text="Hello" --arg target_id=67890 --arg target_type=Project
```

Execution flow:
1. Look up field definition from parsed schema
2. Parse `--arg` flags → map[string]any via `parseVarValue`
3. For each arg where `IsJSON == true`: if value is map/slice, `json.Marshal` it to string
4. Validate required args are present (report missing with helpful message)
5. Generate GraphQL operation string:
   ```graphql
   mutation($user_id: ID!, $text: String!, $target_id: ID!, $target_type: NotificationTargetType!) {
     create_notification(user_id: $user_id, text: $text, target_id: $target_id, target_type: $target_type) {
       id
     }
   }
   ```
6. Selection set: auto-generated from return type (depth 2) unless `--select` overrides
7. Delegate to `executeRawQuery` (reuses auth, retry, error normalization)

**Flags per dynamic command**:
- `--arg key=value` (repeatable) — auto-typed, JSON-coerced
- `--arg-file path.json` — bulk args from JSON file
- `--select fields` — override auto-selection (e.g. `"id,name,column_values{id,text,value}"`)
- `--depth N` — control auto-selection depth (default 2)
- `--dry-run` — print generated GraphQL without executing

**Help text generation**:
```
$ mcli api create_notification --help
Execute: mutation create_notification

Create a new notification.

Arguments:
  user_id         ID!                        (required) The user to notify
  text            String!                    (required) Notification text
  target_id       ID!                        (required) Target entity ID  
  target_type     NotificationTargetType!    (required) Target type (Project/Post)

Returns: Notification { id }

See also: No built-in equivalent.
Use --dry-run to preview the generated GraphQL.
```

**Built-in cross-reference**: For fields that have a built-in equivalent (e.g. `boards` → `mcli board list`), help text includes:
```
See also: mcli board list (structured output, pagination flags)
```

---

### Phase 3: Discovery & Browsing
**Status**: completed  
**Effort**: ~1 day  
**Delivers**: Schema exploration without leaving the terminal

#### `mcli api list`

```
$ mcli api list --type mutation | head
create_board          Create a new board                              [see: mcli board create]
create_column         Create a new column in a board                  [see: mcli board column create]
create_doc            Create a new doc                                
create_folder         Create a new folder                             [see: mcli folder create]
create_group          Create a new group in a board                   [see: mcli board group create]
create_item           Create a new item                               [see: mcli item create]
create_notification   Create a new notification                       
create_subitem        Create a new subitem                            [see: mcli item create --parent]
create_webhook        Create a new webhook                            [see: mcli webhook create]
...
```

- `--type query|mutation` — filter
- `--json` — machine-readable array
- `--no-builtins` — hide operations that have built-in equivalents
- Supports grep-friendly output (one line per operation)

#### `mcli api describe <name>`

Works for both operations and types:

```
$ mcli api describe Board
type Board {
  id: ID!                      The board's unique identifier
  name: String!                The board's name
  state: State!                The board's state (active/archived/deleted)
  board_kind: BoardKind!       The board's kind (public/private/share)
  columns: [Column!]!          The board's columns
  groups: [Group!]!            The board's groups
  items_page(...): ItemsResponse!  Items with cursor pagination
  ...
}
```

---

### Phase 4: LLM Skill & Subskill Update
**Status**: completed  
**Effort**: ~1 day  
**Delivers**: LLM agents can self-serve any API operation

Update `skillDoc` in `internal/cli/skill.go` to add:

```markdown
### Generic API (any operation)

For operations not covered by built-in commands, use `mcli api`:

1. Discover: `mcli api list` — see all ~250 available operations
2. Inspect: `mcli api describe <operation>` — see args, types, description  
3. Execute: `mcli api <operation> --arg key=value...`

JSON arguments (like column_values) are auto-handled — pass natural JSON,
no double-encoding needed:
  mcli api change_column_value --arg board_id=123 --arg item_id=456 \
    --arg column_id=status --arg value='{"label":"Done"}'

Override returned fields: `--select "id,name,column_values{id,text}"`
Preview query without executing: `--dry-run`

### Capability Categories

| Category | Example operations |
|----------|-------------------|
| Notifications | create_notification |
| Teams | add_teams_to_board, create_team, add_users_to_team |
| Apps | create_app, install_app, app_subscriptions |
| Forms | create_form, update_form_question |
| Automations | trigger_events, block_events |
| Dashboards | create_dashboard, create_widget |
| Portfolios | create_portfolio, connect_project_to_portfolio |
```

**Subskill concept**: The skill doc categorizes operations so LLMs can narrow their search. `mcli api list` output is grep-able, so an LLM can: `mcli api list | grep team` to find team-related operations.

---

### Phase 5: Selection Set Intelligence (stretch)
**Status**: not_started (deferred)  
**Effort**: ~1-2 days  
**Delivers**: Better defaults, less --select overriding needed

- **Smart scalar preference**: auto-select `id`, `name`, `state`, `kind` and other common scalars at depth 1
- **Pagination awareness**: if return type has `cursor`/`items_page` pattern, include cursor fields
- **Depth control**: `--depth 1` for IDs only, `--depth 3` for nested structures
- **Presets**: `--select @id` (just id), `--select @full` (depth 4, all scalars)
- **Connection pattern**: detect `[Type!]!` vs `TypeResponse { items, cursor }` and handle both

---

## Key Decisions

| # | Decision | Rationale |
|---|----------|-----------|
| 1 | `mcli api` namespace (not top-level) | Avoids collision with built-ins; clear "generic" boundary |
| 2 | JSON coercion from schema types (not regex) | More reliable than regex; we HAVE the parsed schema |
| 3 | `//go:embed` the SDL | Single binary; no runtime file or network dependency |
| 4 | Lazy command registration | ~250 commands would slow startup if always registered |
| 5 | Reuse `executeRawQuery` | Gets auth, retry, complexity handling, error codes for free |
| 6 | Auto-selection with override | LLMs rarely know field names upfront; smart defaults reduce roundtrips |
| 7 | `--dry-run` flag | Debugging and learning; LLM agents can verify before executing |

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| ~250 subcommands pollute help/completion | Lazy registration; `mcli api list` for discovery; top-level `mcli --help` just shows `api` as one entry |
| Schema embedded at build time may drift | CI refresh (`make schema`); `mcli api --schema-version` shows generation date; warn if >30 days |
| Deep return types → oversized queries | Cap at depth 2; document `--depth` and `--select` |
| Some mutations need complex nested JSON args | JSON coercion handles this transparently; `--arg-file` for very complex cases |
| Field name collisions between Query and Mutation | Monday's schema has none today; if one appears, prefix with `query/` or `mutation/` and add `--mutation` flag |

## Effort Summary

| Phase | Days | Ships independently? | Prerequisite |
|-------|------|---------------------|--------------|
| 1: Schema package | 1 | No (foundation) | — |
| 2: Dynamic commands | 2-3 | Yes (core MVP) | Phase 1 |
| 3: Discovery/browsing | 1 | Yes | Phase 1 |
| 4: LLM skill update | 1 | Yes | Phase 2 |
| 5: Selection intelligence | 1-2 | Yes (stretch) | Phase 1 |
| **Total** | **6-8 days** | | |

**MVP = Phases 1-3** (~4-5 days): parse schema, register dynamic commands with JSON coercion, add list/describe.  
**Full = Phases 1-4** (~5-6 days): add LLM skill documentation.  
**Polish = Phase 5** (~1-2 more days): smarter selection sets.

## File Inventory (new files)

```
internal/api/schema/
  embed.go              //go:embed directive + raw SDL string
  schema.go             Load(), QueryFields(), MutationFields(), TypeDef()
  selection.go          DefaultSelection(), depth-limited field picker
  coerce.go             IsJSONScalar(), CoerceArgs() — schema-aware JSON stringification
  schema_test.go        Unit tests against embedded schema

internal/cli/
  api.go                newAPICmd(), dynamic command registration, execution
  api_list.go           mcli api list
  api_describe.go       mcli api describe
  api_test.go           Integration tests (mocked HTTP)
```

## Non-Goals (explicitly out of scope)

- Replacing built-in commands — they stay for ergonomics and formatted output
- Subscriptions — monday.com doesn't expose GraphQL subscriptions publicly
- Schema hot-reload from live API — use `make schema` + rebuild
- Tab completion for dynamic arg values (enum completion could be Phase 6)
