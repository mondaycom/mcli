# ADR-003: LLM skill API — `mcli describe` and generated `SKILL.md`

## Status
Active

## Context
Axiom A2 declares LLM-native operation a first-class use case. That means an agent without prior knowledge of mcli should be able to:

1. Load a single artifact that describes every command, flag, input, and output shape.
2. Construct a valid invocation on the first try.
3. Parse the result without string-scraping.

Command help text designed for humans is insufficient for this. We need a machine-readable manifest and a companion prose document, both generated from the same source of truth (the cobra command tree + annotated JSON schemas) so they cannot drift.

This ADR fixes:
- The shape of the `mcli describe` output.
- Where golden examples live and how they are validated.
- How we generate the `SKILL.md` that ships alongside the binary.

## Alternatives

### Manifest shape
- **OpenAPI-for-CLIs (bespoke)** — a JSON document tailored to CLI semantics (positional args, flag types, stdin, exit codes, output JSON schema). *Chosen.*
- **Raw cobra `help --json`** — too thin; omits output schemas and example semantics.
- **MCP server** — tempting (mcli-as-MCP-server), but scope creep for v0 and orthogonal to the CLI itself. Deferred.

### Example storage
- **Per-command `examples/` directory** with pairs of invocation + expected output. *Chosen.*
- Inline in Go source — harder to validate, harder to diff.

### SKILL.md generation
- **Generated at build time from the manifest** — guarantees the human and machine docs agree. *Chosen.*
- Hand-written — drifts fast; violates the single-source-of-truth goal.

## Decision

### `mcli describe`

`mcli describe` emits a JSON manifest of the entire command tree. `mcli describe <command>` narrows to one command.

Top-level shape (schema version `1`):

```json
{
  "manifestVersion": "1",
  "binary": "mcli",
  "binaryVersion": "0.1.0",
  "exitCodes": {
    "0": "success",
    "1": "usage error",
    "2": "api error",
    "3": "auth error",
    "4": "rate limited",
    "5": "internal error"
  },
  "errorCodes": ["USAGE", "AUTH", "API", "RATE_LIMITED", "NOT_FOUND", "CONFLICT", "INTERNAL"],
  "globalFlags": [ { "name": "json", "type": "bool", "description": "..." } ],
  "commands": [
    {
      "path": ["board", "list"],
      "summary": "List boards",
      "description": "...",
      "positionals": [],
      "flags": [
        { "name": "limit", "type": "int", "default": 25, "description": "..." },
        { "name": "workspace", "type": "string", "description": "..." }
      ],
      "stdin": { "accepts": false },
      "output": {
        "contentType": "application/json",
        "schema": { "$ref": "#/schemas/BoardList" }
      },
      "examples": [
        {
          "title": "List the first 10 boards in workspace 42",
          "invocation": ["mcli", "board", "list", "--workspace", "42", "--limit", "10"],
          "output": { "items": [{ "id": "...", "name": "..." }], "cursor": null }
        }
      ]
    }
  ],
  "schemas": {
    "BoardList": { "type": "object", "properties": { ... } }
  }
}
```

Requirements:
- `manifestVersion` is semver-major. Breaking changes to the manifest schema itself bump this.
- Every command MUST have at least one example. An empty `examples` array fails CI.
- Every command's `output.schema` MUST be a `$ref` into `schemas`, not inline. This forces shared types and lets agents cache schema definitions.
- `examples[].output` is validated against `output.schema` in CI — so the examples stay honest.

### Examples on disk

Each command has an `examples/` directory next to its Go source, e.g. `internal/api/boards/examples/list.json`. Each file looks like:

```json
{
  "title": "...",
  "invocation": ["mcli", "board", "list", "--workspace", "42"],
  "output": { ... }
}
```

Examples are embedded into the binary via `go:embed`. A CI check asserts that every command referenced in the cobra tree has at least one example file.

### Generated `SKILL.md`

`mcli describe --format=skill-md` renders the manifest as a Markdown document tailored for LLM loading:
- One `##` per command path.
- Argument/flag tables.
- One fenced JSON block per example, with the invocation and the expected output.
- A top-level "How to call me" preamble that documents exit codes, error format, and TTY behavior.

The file is committed as `docs/SKILL.md` and regenerated on every release by CI. A drift check in CI fails if `docs/SKILL.md` is not in sync with the manifest.

### Output-schema source of truth

Output JSON schemas are authored as Go structs with struct tags plus a JSON Schema generator (e.g. `invopop/jsonschema` or equivalent — final choice deferred to Phase 2). The Go struct is the source of truth; the schema is derived. This means:
- Adding a field to a command's response updates the schema automatically.
- Renaming or removing a field is a breaking change and requires a `manifestVersion` bump (only when the change is incompatible).

## Consequences

### Positive
- One document (`SKILL.md`) is sufficient for an LLM to use mcli. This directly serves A2.
- Examples are validated against schemas → we cannot ship hallucinated examples.
- Manifest versioning gives us a clean deprecation path when the output contract needs to change.

### Negative
- Every new command costs at least: a cobra definition, a Go response struct, a `.graphql` query, and one example JSON. Accepted — this is the price of honest self-description, and the friction keeps us from shipping shallow commands.
- The SKILL.md CI drift check slows release slightly. Acceptable; it is a regenerate-and-commit, not a hand-edit.
- We carry a JSON-schema generator dependency. Evaluated under A10 at Phase 2.

---
Supersedes: none.
