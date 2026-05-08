# Project Brief: mcli

*Version: 0.1*
*Created: 2026-05-08*

## Overview
`mcli` is a command-line interface for monday.com's GraphQL API. It exposes a curated, coherent set of commands for the most common Monday workflows (boards, items) plus a raw GraphQL escape hatch, and is designed to be usable both by humans and by LLM agents.

The goal is a single static Go binary that is **complete enough to be useful**, **coherent enough to be learnable**, and **machine-readable enough to be automated**.

## Core Requirements
- Authenticate against monday.com using an API token (env var or config file).
- Cover the most common Monday operations: boards (list/get/create) and items (list/get/create/update/move) with full subitem support.
- Provide a raw GraphQL escape hatch for anything not covered by curated commands.
- Output structured JSON by default for machine consumption; optional human pretty-print.
- Emit a self-describing manifest (`mcli describe`) for LLM consumption.
- Distribute as a single static binary (darwin + linux, arm64 + amd64).

## Success Criteria
- An LLM agent, given only the output of `mcli describe --json`, can construct valid commands for v0 operations without reading Monday's API docs.
- A human can discover and run common Monday workflows without leaving the terminal.
- Coherence: where Monday's API is inconsistent (e.g., column value encoding, pagination styles), mcli presents a uniform surface and documents the translation.
- Rate-limit and complexity-budget awareness: no silent truncation or cryptic failures.

## Scope

### In Scope (v0)
- Auth + config (API token).
- Boards: `list`, `get`, `create`.
- Items: `list`, `get`, `create`, `update`, `move` — including subitems via `--parent`.
- Raw query: `mcli query` with variables from flags/stdin/file.
- LLM skill API: `mcli describe` + generated `SKILL.md`.

### Out of Scope (v0 — deferred)
- Updates/comments, files, webhooks, teams, notifications, docs.
- Workspaces CRUD, groups CRUD.
- Mirror/formula columns (read-through only; no write semantics).
- OAuth flows — API token only for v0.
- Interactive TUI.

## Timeline / Milestones
- 2026-05-08: Phase 0 — foundations (axioms, ADRs, plan) complete.
- TBD: Phase 1 — scaffold + auth.
- TBD: Phase 7 — first tagged release.

## Stakeholders
- Arnon Rotem-Gal-Oz — owner / primary user.

---
Update this brief only when goals or scope change significantly.
