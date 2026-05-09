# Plan: mcli v0

*Created: 2026-05-08*
*Status: Phase 0 complete*

## Goal
Ship a Go-based CLI for monday.com's GraphQL API covering auth, boards (list/get/create), items (list/get/create/update/move + subitems), a raw GraphQL escape hatch, and an LLM skill API.

See `memory-bank/projectbrief.md` for scope and `memory-bank/axioms.md` for ground truths.

## Phase 0 — Foundations ✅

**Deliverables:**
- `memory-bank/projectbrief.md`
- `memory-bank/axioms.md`
- `memory-bank/adrs/ADR-001-stack.md`
- `memory-bank/adrs/ADR-002-cli-grammar.md`
- `memory-bank/adrs/ADR-003-llm-skill-api.md`
- `memory-bank/docs/plan-v0.md` (this file)

**Acceptance:**
- AC-0.1: All ADRs committed with status `Active`.
- AC-0.2: `axioms.md` covers coherence, LLM-native output, single binary, dependency policy.
- AC-0.3: Git repo initialized with a checkpoint commit.

## Phase 1 — Scaffold + auth + config ✅

**Deliverables:**
- `go.mod`, `cmd/mcli/main.go`, `internal/cli/root.go` with cobra skeleton.
- `mcli version` wired to a `-ldflags`-injected version string.
- `mcli auth login --token <t>` persists token at `~/.config/mcli/config.yaml` (mode 0600).
- Token resolution precedence: `--token` flag > `MONDAY_API_TOKEN` env > config file.
- `internal/config` package with typed config + load/save helpers.
- `internal/httpx` package: authenticated HTTP client with `User-Agent: mcli/<version>`, 429/5xx backoff, request ID logging.
- `Makefile` targets: `build`, `test`, `lint`, `vet`.
- `.golangci.yml` with a conservative rule set.
- GitHub Actions CI: build + test + lint on push/PR.

**Acceptance:**
- AC-1.1: `mcli version` prints the injected version.
- AC-1.2: `mcli auth login --token TEST` writes the expected file with mode 0600.
- AC-1.3: Config round-trip test: save → load returns structurally identical config.
- AC-1.4: `golangci-lint run` exits 0.

## Phase 2 — GraphQL client + schema

**Deliverables:**
- `schema/monday.graphql` — Monday GraphQL schema committed.
- `Makefile` target `make schema` that refreshes the schema from a live endpoint (requires token).
- `tools/gqlgen.go` wiring `Khan/genqlient` with generation config.
- `internal/api/graphql` package:
  - typed client wrapping the authenticated HTTP client,
  - error normalization (GraphQL errors in a 200 → structured `APIError`),
  - complexity-budget awareness (parse `X-Complexity-*` headers if present; extract from error payloads otherwise),
  - retry on `COMPLEXITY_BUDGET_EXHAUSTED` with exponential backoff.
- `go generate ./...` produces typed query bindings.
- CI check: `go generate` produces no diff.

**Acceptance:**
- AC-2.1: A trivial `me { id name }` query returns a typed struct.
- AC-2.2: A forced GraphQL error maps to a structured `APIError` with a stable `Code`.
- AC-2.3: `go generate` produces no diff in CI.

## Phase 3 — Boards

**Deliverables:**
- `mcli board list [--workspace <id>] [--limit <n>] [--cursor <c>]`
- `mcli board get <id>`
- `mcli board create --name <name> [--workspace <id>] [--kind public|private|share]`
- `internal/api/boards` package with typed request/response Go structs.
- `examples/` JSON per command (per ADR-003).
- Unit tests against recorded fixtures for each command.

**Acceptance:**
- AC-3.1: All three commands round-trip against recorded fixtures.
- AC-3.2: Each command has at least one validated example JSON.
- AC-3.3: `mcli describe board list` returns a manifest entry with a non-empty output schema.

## Phase 4 — Items + column value normalization

This is the coherence-hard phase. Monday's `column_values` are JSON-string blobs whose shape varies per column type. Our CLI must translate between clean flags and those blobs.

**Deliverables:**
- `internal/api/items/columns` package:
  - a registry of supported column types (text, long-text, status, date, datetime, people, dropdown, numeric, link, email, phone, timeline, tags, checkbox),
  - an `Encode(ColumnType, userValue) (jsonBlob, error)` and `Decode(ColumnType, jsonBlob) (userValue, error)` for each,
  - clear errors for unsupported column types in v0 (user told to use `mcli query`).
- `mcli item list --board <id> [--group <id>] [--limit <n>] [--cursor <c>]`
- `mcli item get <id>`
- `mcli item create --board <id> [--group <id>] --name <name> [--col <col>=<value>]...` with repeated `--col` flags.
- `mcli item update <id> [--name <name>] [--col <col>=<value>]...`
- `mcli item move <id> --to-group <id> | --to-board <id>` (one of).
- `--parent <id>` flag on `item create` to create a subitem.
- Tests for every column type's Encode/Decode, including round-trip against the real API under a build tag.

**Acceptance:**
- AC-4.1: All item commands work against recorded fixtures.
- AC-4.2: Every supported column type has a round-trip test.
- AC-4.3: Unsupported column types fail with a structured error (code `USAGE`, message names the column type, suggests `mcli query`).
- AC-4.4: Subitems created via `--parent` appear under the parent on a subsequent `mcli item get`.

## Phase 5 — Raw query escape hatch

**Deliverables:**
- `mcli query [-f <file>|-f -] [--var key=value]... [--vars-file <json>]`
- Accepts query on stdin when `-f -`.
- Variables from `--var` flags (string by default; `--var-type key=int` for typed) OR from a JSON file via `--vars-file`.
- Output: `{"data": ..., "errors": [...], "extensions": {...}}` passthrough — no normalization for raw queries.
- Exit code 2 if `errors` is non-empty, 0 otherwise.

**Acceptance:**
- AC-5.1: `echo 'query { me { id } }' | mcli query -f -` works.
- AC-5.2: Variables from flags and from file produce identical requests (verified by captured request body).
- AC-5.3: A query with GraphQL errors exits 2.

## Phase 6 — LLM skill API

**Deliverables:**
- `mcli describe [command-path]` returns the manifest as defined in ADR-003.
- `mcli describe --format=skill-md` renders Markdown suitable for LLM loading.
- `docs/SKILL.md` generated + committed.
- CI drift check: regenerating SKILL.md produces no diff.
- CI schema check: every example JSON validates against its command's output schema.

**Acceptance:**
- AC-6.1: `mcli describe --json` emits valid JSON matching the ADR-003 shape.
- AC-6.2: Every command has at least one example; CI fails if any command has zero.
- AC-6.3: CI regenerates SKILL.md with zero diff.

## Phase 7 — Release

**Deliverables:**
- `README.md` with quickstart, install, 3-5 example invocations.
- GitHub Actions release workflow: on tag push, build darwin/linux × amd64/arm64, attach to GitHub Release.
- Install instructions: direct binary download + checksums. Homebrew tap deferred to post-v0.
- Versioning: start at `v0.1.0`. Semver applies once v1.0.0 ships.

**Acceptance:**
- AC-7.1: A tag push produces a GitHub Release with all four platform binaries.
- AC-7.2: The README quickstart works end-to-end against a real token.

## Out of scope (v0)
Explicitly deferred, each a candidate for its own later plan:
- Updates / comments.
- Files upload.
- Webhooks CRUD.
- Teams, users CRUD, notifications.
- Docs (Monday's "Docs" product).
- Workspace, group CRUD.
- Mirror / formula column *writes* (read-through is fine).
- OAuth.
- Homebrew tap.
- MCP server wrapper.

## Open questions
- ~~Binary name `mcli`~~ — resolved 2026-05-09 (chosen, in production).
- ~~Config path `~/.config/mcli/`~~ — resolved 2026-05-09 (XDG-aware).
- JSON-schema generation library for manifest: decide in Phase 2 (candidates: `invopop/jsonschema`).

## Phase 2 prerequisites (carry-overs from Phase 1)
- **httpx body preservation across retries**: `RetryTransport.RoundTrip` does not preserve the request body across retries. POST requests (all of GraphQL) will silently send an empty body on retry attempts. Must be fixed before Phase 2 wires real GraphQL calls. Fix: snapshot `req.GetBody` (or buffer the body) before the first attempt and replay on each retry. Add a test that posts a non-empty body and verifies the retry receives the same body.

## Progress log
- 2026-05-08: Phase 0 complete — axioms + ADR-001/002/003 + this plan.
- 2026-05-09: Phase 1 complete — scaffold + auth + config. Deps: cobra, yaml.v3. All AC met; go test -race green, golangci-lint clean.
- 2026-05-09: Phase 1 review fixes — error output path now honors ADR-002 (JSON to stdout in non-TTY/--json, human to stderr in --pretty/TTY); cobra error/usage rendering silenced; new internal/cli/output.go with TTY detection. R3 (httpx body preservation) carried as Phase 2 prerequisite.
