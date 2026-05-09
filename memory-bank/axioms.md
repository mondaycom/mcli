# Project Axioms: mcli

*Ground truths for the mcli project. These are decisions that are rarely re-litigated; every contributor (human or AI) should read this before proposing designs or writing code.*

---

## A1. Coherence beats mirroring

monday.com's GraphQL API has real quirks: `column_values` are JSON-string blobs with per-column-type shapes, pagination switches between cursor and page/limit depending on the field, items and subitems share a type, and some fields are nullable for historical rather than semantic reasons.

**mcli does not mirror these quirks.** The CLI surface is a deliberately designed vocabulary. Where Monday's API is inconsistent, mcli chooses one shape and documents the translation in the adjacent ADR or command doc.

Consequence: every Monday quirk that leaks into the CLI surface is a bug. When in doubt, normalize; when normalization is impossible, document the leak loudly.

## A2. LLM-native is a first-class use case

mcli is designed to be driven by LLM agents as fluently as by humans. This is not a nice-to-have — it shapes the output format, the help text, and the manifest.

Concretely:
- Structured JSON output by default; `--pretty` (or TTY detection) for humans.
- Every command is self-describing via `mcli describe`.
- Errors are structured (`{"error": {...}}`) with stable `code` fields.
- No interactive prompts unless `stdin` is a TTY and the user did not pass `--no-input`.
- Exit codes are stable and documented.

## A3. One static binary

mcli ships as a single static Go binary. No runtime dependencies. No Node, no Python, no system libraries beyond libc.

This rules out: embedding a JS interpreter, dynamic plugin loading, calling out to `jq`/`curl`. If we need JSON-path selection, we build it in.

## A4. Completeness is a vector, not a gate

Monday's API is large. We will never reach 100% coverage in v0, and chasing it would delay release indefinitely. Instead, each release adds one resource domain (boards → items → updates → files → webhooks → …) with full depth in that domain.

The raw `mcli query` escape hatch exists precisely so that missing coverage is never a hard block.

## A5. Rate limits and complexity budgets are user-visible

Monday enforces a per-minute complexity budget. mcli:
- Surfaces complexity cost in `--verbose` mode.
- Retries with backoff on 429 / complexity-exceeded.
- Never silently drops or truncates paginated results. If a query hits the budget, the error is explicit and the partial cursor is returned.

## A6. Stable output contract

Once a command's output shape is shipped, it is part of the contract. Breaking changes require a version bump and a migration note. Adding fields is non-breaking; removing or renaming fields is breaking.

This applies to `mcli describe` output as well — the manifest schema is itself versioned.

## A7. Test against the real API for contract coverage

Unit tests run against recorded fixtures. **Contract tests** run against the real Monday API behind a build tag and an env var, against a dedicated sandbox account. A green unit suite with a stale fixture is not sufficient confidence for a release.

## A8. No TODOs in shipped code

Ship or don't. If something isn't done, it doesn't get merged with a `// TODO`. Open an issue or a follow-up plan phase instead. (Exception: explicit `// TODO(#issue-number)` tied to a tracked ticket.)

## A9. Pure functions and specific types

Prefer pure functions, immutability, and named type aliases over bare primitives (`type ItemID string`, not `string`). Makes the LLM-generated code easier to verify and the human code easier to read.

## A10. Dependencies are a liability

Every third-party Go module added is a long-term maintenance cost. Before adding one:
1. Confirm the stdlib cannot do the job reasonably.
2. Confirm the license is permissive (MIT, BSD, Apache-2.0, MPL-2.0). Reject GPL/AGPL unless explicitly approved.
3. Document the dependency and its role in an ADR or the relevant plan phase.

## A11. Secrets at rest

API tokens and other secrets MUST NOT be persisted to disk in plaintext, even with restrictive file modes (e.g. 0600).

Approved storage backends:
1. Platform keychain (macOS Keychain, Linux Secret Service, Windows Credential Manager) — default.
2. Symmetric encryption with a user passphrase — fallback when no keychain is available.

Mode bits alone are not sufficient because they do not protect against backups, cloud sync (iCloud, Dropbox, Google Drive), dotfiles repositories, accidental `cat`/screen-share, or any process running as the same user.

Consequence: any code path that writes a credential to disk goes through `internal/secrets`. Plaintext on disk is a release blocker.

## A12. LLM-primary use

mcli's primary runtime caller is an LLM agent, not a human at a terminal. The expected lifecycle is: a human runs setup once interactively (e.g. `auth login`); LLMs invoke the CLI programmatically thereafter.

Consequence:
- The read path (every command that is not initial setup) MUST work without TTY interaction — no passphrase prompts, no confirmations, no spinners.
- All inputs must be expressible via flags + env vars + JSON I/O.
- Interactive UX is acceptable only at one-time setup steps and must always have a non-interactive equivalent.
- When trading off "nice for humans" vs "nice for agents", lean toward the agent.

---
*Update this file only when a new ground truth is established or an existing one is superseded. Superseded axioms are struck through, not deleted, and reference the ADR that replaced them.*
