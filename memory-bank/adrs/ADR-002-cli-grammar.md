# ADR-002: CLI grammar and output contract

## Status
Active

## Context
mcli must be usable by both humans and LLM agents (axiom A2). A consistent grammar and a stable output contract are prerequisites for both. This ADR fixes the surface-level conventions so that every subcommand we add follows the same shape.

Decisions in this document cover:
- Command grammar (noun-verb order, naming).
- Flag conventions (global vs command-local, short vs long).
- Output format (JSON-default, `--pretty`, TTY detection).
- Exit codes.
- Error format.
- Stdin and piping behavior.

## Alternatives

### Command grammar
- **`mcli <noun> <verb>`** (e.g. `mcli board list`, `mcli item create`). Used by `gh`, `kubectl`, `aws`. Scales to many resource domains. Easy to discover via `mcli <noun> --help`. *Chosen.*
- **`mcli <verb> <noun>`** (e.g. `mcli list boards`). More natural English, but verbs collide across nouns (`get`, `create` apply to many nouns) and make help scoping awkward.
- **Flat commands** (e.g. `mcli list-boards`). Doesn't scale past ~20 commands.

### Output format default
- **JSON always-on, `--pretty` for humans** — simplest machine contract, but poor human UX by default.
- **JSON when non-TTY, pretty when TTY** — auto-switch based on `isatty(stdout)`. Common in modern CLIs (`gh`, `docker`). *Chosen.*
- **Pretty default, `--json` for machines** — rejected; A2 requires LLMs to get structured output without memorizing a flag.

### Exit codes
- **Binary (0 / nonzero)** — simple but useless for programmatic branching.
- **Categorized** — distinct codes for user error vs API error vs auth vs rate-limit. *Chosen.*

## Decision

### Command grammar
- Noun-verb: `mcli <noun> <verb> [args] [flags]`.
- Nouns are singular (`board`, `item`, not `boards`). Verbs follow conventional English (`list`, `get`, `create`, `update`, `delete`, `move`).
- Top-level non-resource commands: `auth`, `query`, `describe`, `version`, `completion`.
- No aliases in v0 (we add them only when user friction justifies it).

### Flags
- Global flags (available on every command):
  - `--json` / `--pretty` — force output mode (overrides TTY detection).
  - `--config <path>` — override config file path.
  - `--token <token>` — override API token (env: `MONDAY_API_TOKEN`).
  - `--verbose` / `-v` — emit request/response metadata to stderr; include complexity cost.
  - `--no-input` — never prompt; fail instead.
  - `--help` / `-h` — standard cobra help.
- Short flags only for the above globals plus a small curated set per command. Long-form is always available.
- Repeated flags use `--flag=value` form and are order-preserving (e.g. `--col status=Done --col date=2026-05-08`).

### Output contract
- **Default:** JSON to stdout when stdout is not a TTY; compact pretty-printed (2-space indent) human table when stdout is a TTY.
- **`--json`** forces JSON. **`--pretty`** forces human.
- All stderr output is human-readable text (logs, progress, errors) — never JSON unless the whole process fails before a command runs.
- JSON output is a single top-level object per invocation, never a JSON stream. Bulk/list commands return `{"items": [...], "cursor": "..."}`. Subsequent pages are separate invocations; mcli does not stream multiple top-level objects to stdout.

### Exit codes
| Code | Meaning |
|-----:|:--------|
| 0 | Success |
| 1 | Usage / input error (bad flag, invalid argument) |
| 2 | API error (Monday returned GraphQL errors or non-2xx) |
| 3 | Auth error (missing or invalid token) |
| 4 | Rate-limit / complexity-budget exceeded after retries |
| 5 | Internal / unexpected error |

### Error format
When any command fails and `--json` is active (or stdout is non-TTY), the last line of stdout is a single JSON object:

```json
{"error": {"code": "RATE_LIMITED", "message": "Complexity budget exceeded", "retryAfter": 30, "requestId": "..."}}
```

`code` is drawn from a stable enumeration (`USAGE`, `AUTH`, `API`, `RATE_LIMITED`, `NOT_FOUND`, `CONFLICT`, `INTERNAL`). The same codes map to exit codes above. Human mode prints a red, structured error to stderr and exits.

### Stdin / piping
- Commands that accept bulk input (e.g. `mcli item create`) accept JSON on stdin when `-` is passed as a positional or when no positional is given and stdin is non-TTY.
- `mcli query` accepts the GraphQL document on stdin when `-f -` is passed.
- No command reads stdin implicitly without the `-` sentinel — this keeps piping explicit and avoids surprising LLM-generated pipelines.

### Config precedence
Flag > env var > config file > built-in default. Documented per-option.

## Consequences

### Positive
- LLMs get a predictable shape: `mcli describe` + structured JSON + stable codes means a single manifest load is enough to generate valid commands.
- Humans get pretty output by default in an interactive terminal without having to learn a flag.
- Shell pipelines (`mcli item list --board 123 | jq ...`) work correctly because non-TTY defaults to JSON.

### Negative
- TTY-sensitive default surprises users who `tee` output to a file expecting pretty text. Mitigation: `--pretty` is always available, and the man page / `describe` surface calls this out.
- Categorized exit codes mean we must be careful about classification at every error site. Accepted — the win for scripting and agents outweighs the code-review cost.
- Pinning JSON-per-invocation (not streaming) limits future streaming use cases. We will revisit in a later ADR if needed.

---
Supersedes: none.
