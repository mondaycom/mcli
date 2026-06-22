# Project Brief: mcli

*Version: 0.4*
*Created: 2026-05-08*
*Updated: 2026-06-22*

## Overview
`mcli` is a command-line interface for monday.com's GraphQL API. It exposes a curated set of typed commands for the most common Monday workflows plus a dynamic `mcli api` namespace that exposes every API operation from the embedded GraphQL schema (~250 operations). Designed to be used both by humans and LLM agents in steady state.

The goal is a single static Go binary that is **complete enough to be useful**, **coherent enough to be learnable**, and **machine-readable enough to be automated**.

## Core Requirements
- Authenticate via API token (env var, OS keychain, or age-encrypted file).
- Cover the most common Monday operations: boards, items, workspaces, folders, groups, columns, webhooks.
- Expose every API operation dynamically via `mcli api <operation>` without writing GraphQL.
- Output structured JSON by default; terse, CSV, and pretty modes for human and LLM consumers.
- Emit a self-describing skill doc (`mcli skill`) for LLM consumption.
- Distribute as a single static binary (darwin + linux, arm64 + amd64).

## Success Criteria
- An LLM agent can discover, inspect, and invoke any monday.com API operation via `mcli api list` / `mcli api describe` / `mcli api <op>` without reading Monday's API docs.
- A human can discover and run common Monday workflows without leaving the terminal.
- Coherence: where Monday's API is inconsistent (column value encoding, pagination styles), mcli presents a uniform surface.
- Rate-limit and complexity-budget awareness: no silent truncation or cryptic failures.

## Current Capabilities (as of 2026-06-22)

### Auth & Config
- Token storage: OS keychain (macOS), age-encrypted file (Linux/fallback) — never plaintext
- `mcli auth login` / `mcli auth logout` / `mcli auth status`
- `mcli config set/get output-mode` — persistent default output mode
- `mcli config set/get api-version` — set monday.com API version; fetches + caches schema on set
- `MONDAY_API_TOKEN` / `MONDAY_API_URL` / `MONDAY_API_VERSION` env overrides

### Typed Commands
- **Boards**: `list`, `get`, `create`, `delete`, `archive`
- **Items**: `list`, `get`, `create`, `update`, `move`, `delete`, `archive` (subitems via `--parent`)
- **Workspaces**: `list`, `get`, `create`, `update`, `delete`
- **Folders**: `list`, `create`, `update`, `delete`
- **Board groups**: `list`, `create`, `update`, `delete`
- **Board columns**: `list`, `create`, `delete`
- **Webhooks**: `list`, `create`, `delete`
- **Raw GraphQL**: `mcli query` / `mcli mutation` with variables

### Dynamic API Namespace (`mcli api`)
- `mcli api list [--type query|mutation]` — browse all ~250 API operations
- `mcli api describe <name>` — inspect operation signature or type definition
- `mcli api <operation> [--arg k=v]...` — execute any operation; JSON scalars auto-coerced
- Schema embedded at build time; `mcli config set api-version` refreshes cache from live API

### Output Modes
- `--json` / `--pretty` / `--terse` / `--csv` flags on all commands
- `mcli config set output-mode <mode>` for persistent default

### Background Daemon
- `mcli daemon start/stop/status` — background service for webhook ingestion
- Cloudflare Quick Tunnel for public URL when no external URL configured

## Deferred / Planned
- `mcli search` — cross-entity search (boards, items, docs)
- `mcli item find` — server-side column filter
- `mcli item updates` — read item updates/comments
- `mcli item description` / `mcli doc` — read/write markdown content
- Selection-set intelligence for `mcli api` (smarter auto-selection)

## Timeline / Milestones
- 2026-05-08: Phase 0 — foundations (axioms, ADRs, plan) complete.
- 2026-05-xx: v0 core — auth, boards, items, raw query, skill doc.
- 2026-06-xx: Phase 4.5 — workspaces, folders, board structure, webhooks, daemon.
- 2026-06-21: Dynamic API namespace (`mcli api`) — ~250 operations from embedded schema.
- 2026-06-22: Configurable API endpoint + version with schema auto-refresh.

## Stakeholders
- Arnon Rotem-Gal-Oz — owner / primary user.

---
Update this brief only when goals or scope change significantly.
