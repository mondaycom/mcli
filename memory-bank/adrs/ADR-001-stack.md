# ADR-001: Implementation stack — Go, cobra, genqlient

## Status
Active

## Context
mcli is a CLI for monday.com's GraphQL API. The stack choice affects distribution, type safety against API drift, contributor onboarding, and runtime dependencies.

Per axiom A3, we ship a single static binary. Per A1, coherence with Monday's API — including detecting schema drift early — is a first-class concern. Per A10, every dependency is a liability.

Three layers need a decision:
1. **Language / runtime**.
2. **CLI framework** (subcommands, flag parsing, help generation).
3. **GraphQL client** (authenticated HTTP to `api.monday.com/v2`, with typing).

## Alternatives

### Language
- **Go** — single static binary, cross-compile trivially, mature CLI ecosystem, strong stdlib. No official Monday SDK (OK — we use raw GraphQL). *Chosen.*
- **TypeScript/Node** — official `monday-sdk-js` exists but is a thin GraphQL wrapper. Requires Node runtime on every user machine. Rejected (violates A3).
- **Python** — rich CLI ergonomics (Typer). Requires Python runtime. Rejected (violates A3).
- **Rust** — similar benefits to Go. Rejected for contributor onboarding cost and slower iteration in a greenfield project.

### CLI framework (Go)
- **`spf13/cobra`** — de-facto standard, used by kubectl, gh, docker, hugo. Rich subcommand tree, shell completion generation, integrates with `spf13/viper` for config. Large but well-maintained, Apache-2.0. *Chosen.*
- **`urfave/cli`** — lighter, simpler API. Weaker support for deeply nested subcommands and help generation.
- **`alecthomas/kong`** — struct-tag-driven, elegant for small CLIs. Smaller ecosystem, less common pattern for LLMs to recognize.
- **stdlib `flag`** — too primitive for a multi-noun CLI with nested subcommands.

### GraphQL client (Go)
- **`Khan/genqlient`** — code generation from the server's GraphQL schema. Typed queries, compile-time detection of schema drift, MIT license. Build step required (`go generate`). *Chosen.*
- **`hasura/go-graphql-client`** — runtime reflection-based client. No codegen. Less type safety; schema drift only surfaces at runtime.
- **`machinebox/graphql`** — minimal raw-string client. No typing at all. Useful only as an escape hatch.
- **Hand-rolled `net/http`** — simplest, no third-party dep, but we'd reinvent variable encoding, error parsing, and type mapping for every query.

## Decision

- **Language:** Go (1.22+, using the current toolchain directive).
- **CLI framework:** `spf13/cobra` for subcommands and help; `spf13/viper` deferred — we start with env vars + a simple YAML config and only adopt viper if we need layered config sources.
- **GraphQL client:** `Khan/genqlient` for all curated commands. The raw `mcli query` command uses a minimal `net/http` + `encoding/json` path (no typing needed for user-supplied queries).

Queries are stored as `.graphql` files under `internal/api/<domain>/queries/` and compiled to typed Go by `go generate`. The Monday schema is checked into `schema/monday.graphql` and refreshed by a `make schema` target.

## Consequences

### Positive
- Single static binary, cross-compiled to darwin/linux × amd64/arm64 from CI.
- Schema drift is a compile error, not a runtime surprise — directly serves axiom A1 (coherence).
- `cobra` gives us shell completions and structured help for free, which we can also consume when generating the `mcli describe` manifest (ADR-003).
- All chosen dependencies are permissive-licensed (Apache-2.0, MIT, BSD-3) — compatible with A10.

### Negative
- `genqlient` adds a `go generate` build step. Contributors must run it after query/schema changes. Mitigated by CI check that re-runs generate and fails on diff.
- Cobra is heavier than strictly necessary for a small CLI; accepted for ecosystem familiarity.
- The Monday schema is versionless — we pin by checking the fetched schema into the repo and refreshing deliberately rather than continuously.
- `mcli query` (raw) and curated commands use two different client paths. Acceptable because their use cases are disjoint.

---
Supersedes: none.
