# Plan: Configurable API Version with Schema Auto-Refresh

**Status**: COMPLETED
**Created**: 2026-06-22

## Overview

`mcli config set api-version <version>` saves the version, fetches the schema for that version from the Monday.com API, and caches it locally. All subsequent `mcli api` commands use the cached schema and send the correct `API-Version` header. Falls back to the embedded schema when no local cache exists.

If the version does not exist or the fetch fails, the version is reverted to the default and an error is reported — the CLI never enters a broken state.

---

## Phase 1: Config struct + API-Version header
**Status**: completed

Wire the version through config into every API request.

### Tasks:
- [x] **1.1** Add `APIVersion string` (`yaml:"api_version,omitempty"`) to `Config` struct in `internal/config/config.go`
- [x] **1.2** Add `config.ResolveAPIVersion(cfg Config) string` — precedence: `MONDAY_API_VERSION` env > `cfg.APIVersion` > `"2026-07"`
- [x] **1.3** Update `httpx.NewClient(token, cliVersion, apiVersion string)` — add `apiVersion` param, store on `RetryTransport`, use instead of const
- [x] **1.4** Update `internal/cli/query.go:newQueryHTTPClient` — pass `config.ResolveAPIVersion(cfg)` to `httpx.NewClient`
- [x] **1.5** Update `internal/api/graphql/client.go:New` — add `WithAPIVersion(v string)` option; `newGQLClient()` in `client.go` passes `config.ResolveAPIVersion(cfg)`
- [x] **1.6** Add `api-version` case to `runConfigSet` (validate `^\d{4}-\d{2}$`, or `"default"` to reset) and `runConfigGet` in `internal/cli/config.go`

**Completion criteria**: `MONDAY_API_VERSION=2025-10 mcli query '{ me { id } }'` sends `API-Version: 2025-10`; `mcli config set api-version 2025-10 && mcli config get api-version` round-trips.

---

## Phase 2: Local schema cache
**Status**: completed

Make `Load()` prefer a locally-cached schema file over the embedded SDL.

### Tasks:
- [x] **2.1** Add `apischema.SetConfigDir(dir string)` — stores the dir path; called once from `cli.Execute()` via `resolveConfigDir()`. Resets the sync state so the next `Load()` re-resolves.
- [x] **2.2** Update `Load()` — when `configDir` is set, attempt `os.ReadFile(configDir + "/schema.graphql")` first; fall back to `rawschema.SDL` if the file is absent or unreadable
- [x] **2.3** Add `apischema.CachedSchemaPath(configDir string) string` — returns `filepath.Join(configDir, "schema.graphql")`

**Completion criteria**: placing a valid SDL at `~/.config/mcli/schema.graphql` causes `mcli api list` to reflect its contents; removing the file reverts to the embedded SDL.

---

## Phase 3: Schema fetch on `config set api-version`
**Status**: completed

Extract the introspection logic from the tool and wire it into `config set`.

### Tasks:
- [x] **3.1** Create `internal/api/schema/fetch.go` — extract `fetchSchema(ctx, token, endpoint, apiVersion string) (introspSchema, error)` and `renderSDL(schema) string` from `tools/introspect/main.go` (shared, no build tag). The introspect types (`introspType`, etc.) move here too.
- [x] **3.2** Update `tools/introspect/main.go` — replace inline logic with calls to the shared functions; read `MONDAY_API_VERSION` env (falls back to `"2026-07"`) for the `API-Version` header
- [x] **3.3** Update `runConfigSet` for `api-version` with tentative save → fetch → revert-on-error or write schema
- [x] **3.4** Handle the no-token case gracefully: save the config key but skip the fetch and warn
- [x] **3.5** `mcli config set api-version default` resets to empty and removes cached schema.graphql

**Completion criteria**: `mcli config set api-version 2026-08` (valid token) writes schema + confirms; `mcli config set api-version 9999-99` returns error, reverts to default, leaves no stale schema file.

---

## Phase 4: Tests
**Status**: completed

### Tasks:
- [x] **4.1** `internal/api/schema/schema_test.go` — `TestLoadResolved_localFile`: write a minimal SDL to a temp dir, call `SetConfigDir` + `Load()`, assert the local schema is used
- [x] **4.2** `internal/api/schema/fetch_test.go` — mock HTTP server returning a known introspection response; assert `fetchSchema` returns expected types and `renderSDL` produces valid SDL
- [x] **4.3** `internal/cli/config_test.go` — three cases: success, invalid format, 4xx revert

### Known limitation
Monday.com does not reject unknown version strings — it silently returns its current schema. The revert-on-error path fires on network/HTTP errors but not on unrecognised version values.

---

## Risks & Trade-offs

| Item | Note |
|------|------|
| `sync.Once` reset | `SetConfigDir` must reset the once-cell. Use a mutex + flag rather than `sync.Once` directly, or swap to an atomic pointer. |
| Token required for schema fetch | Handled in 3.4 — degrade gracefully, version still saves. |
| `tools/introspect` shares types with `internal/api/schema` | Introspect types move to `internal/api/schema/fetch.go`; introspect tool becomes a thin wrapper. |
| Local schema may drift | A stale `schema.graphql` silently shadows the embedded one. Acceptable — user opted in by running `config set`. |
| Revert on bad version | Config is written twice (once with new version, once with revert). Small window where config holds a bad version on crash, but schema file was never written so Load() falls back to embedded — safe. |

---

## File Inventory

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `APIVersion` field, `ResolveAPIVersion()` |
| `internal/httpx/client.go` | Add `apiVersion` param to `NewClient`, remove const |
| `internal/api/graphql/client.go` | Add `WithAPIVersion` option |
| `internal/cli/client.go` | Pass `ResolveAPIVersion` to graphql client |
| `internal/cli/query.go` | Pass `ResolveAPIVersion` to `httpx.NewClient` |
| `internal/cli/config.go` | Add `api-version` to set/get, schema fetch + revert logic |
| `internal/cli/root.go` | Call `apischema.SetConfigDir(resolveConfigDir())` in init |
| `internal/api/schema/schema.go` | Add `SetConfigDir`, update `Load()` |
| `internal/api/schema/fetch.go` | New — shared introspect types + `fetchSchema` + `renderSDL` |
| `tools/introspect/main.go` | Thin wrapper using shared functions |
| `internal/api/schema/schema_test.go` | Add local file override test |
| `internal/api/schema/fetch_test.go` | New — mocked HTTP fetch test |
| `internal/cli/config_test.go` | New — end-to-end config set tests |
