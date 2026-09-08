---
name: update-docs
description: "Update mcli's documentation to match the code: the generated skill doc, README, examples, ADRs, and memory-bank plans. Use after any change that adds, renames, or removes a command, flag, error code, or output shape."
allowed-tools: default
---

# Skill: UPDATE-DOCS (mcli)

Bring mcli's documentation back in line with the code. Invoke after a change that
is user-visible, or standalone with `/update-docs`.

mcli is consumed primarily by LLM agents, not humans reading a wiki. A stale doc
is not cosmetic debt here — it actively makes agents emit wrong commands. Treat
documentation drift as a bug.

## The doc surfaces (in priority order)

### 1. `internal/cli/skill.md` — the skill doc (highest priority)

Plain markdown, embedded into the binary by `internal/cli/skill.go`. It is what
`mcli skill` / `mcli describe` prints, and the primary contract for LLM callers.
It MUST be updated when a change touches:

- a command or subcommand (added, renamed, removed)
- a flag that appears in the doc's usage lines
- an error code or its exit code (see `internal/errs/errs.go`)
- an output shape, output mode, or column-value write shape

Organise it by what an agent does, in frequency order — discover IDs, read, write,
recover from an error — not by the shape of the command tree. Board setup is rare;
writing items is not. Document the flags whose *absence* produces a misleading error
(`board create --workspace`, `board column create --defaults`) even when `--help`
calls them optional, and keep the "Failures Worth Knowing About In Advance" section
current: it is the highest-value section in the doc.

Guards that will fail CI if you forget:

- `TestSkillDoc_MatchesCommandTree` — every command path and flag the doc shows is
  resolved against the real cobra tree. This is the guard that did not exist when
  the doc drifted; `TestSkillDoc_AuditCatchesDrift` proves it can still fail.
- `TestSkill_ErrorCodesMatchErrs` — every code in `errs.AllCodes()` must appear in
  the doc annotated with the exit code `errs.ToExitCode` really returns.
- `TestSkill_ContainsGoals` / `TestSkill_ContainsSections` — required commands and
  section headings.
- `TestSkill_Concise` — a byte budget (`skillDocMaxBytes`), because bytes are what
  the doc costs an agent's context. If you are near it, cut or tighten an existing
  section rather than raising the cap.

Run `go test ./internal/cli/ -run TestSkill` after editing.

### 2. `README.md`

Human-facing entry point. Update the command reference and any example output
when commands change. Keep it consistent with the skill doc — they should never
disagree about a flag name.

### 3. `examples/`

JSON request/response examples (`item-create.json`, `board-get.json`, …) and the
end-to-end demos (`ecommerce-demo.md` / `.sh`). If an output shape or column-value
encoding changed, these become wrong and misleading. Verify affected examples still
reflect real output.

### 4. `memory-bank/adrs/` — validate, do not rewrite

ADR-001 (stack), ADR-002 (CLI grammar / exit codes), ADR-003 (LLM skill API),
ADR-004 (secret storage). ADRs record decisions as of a date; they are **not**
auto-updated. If a change contradicts an ADR, flag it for human review — either the
change is wrong or the ADR needs a superseding entry. Never silently edit an ADR to
match new code.

### 5. `memory-bank/` plans and docs

See the hygiene rules below — this repo has a specific convention.

## Plan hygiene rules (mcli-specific)

The committed part of `memory-bank/` describes **shipped reality**. It is not a
roadmap and not a scratchpad. Concretely:

- **Committed docs must not contain speculative or unstarted plans.** Aspirational
  plan docs rot into lies: readers cannot tell "not_started" from "done but never
  re-marked", which is exactly the failure this repo already hit.
- **Future / in-design work goes in `memory-bank/private/plans/`**, which is
  gitignored (`memory-bank/private/`). Design docs awaiting approval live there.
- **When a plan fully ships, delete it** rather than leaving a COMPLETED doc behind.
  Git history preserves it, and the code plus ADRs are the durable record.
- **Never trust a `Status:` marker.** Verify against the codebase before acting on
  or reporting any plan's status.

`memory-bank/axioms.md` and `memory-bank/projectbrief.md` are durable — update them
only when a project-level invariant or goal genuinely changes.

## Out of scope for this repo

- `schema/monday.graphql` is vendored, not documentation. Refresh it with
  `make schema` (runs `tools/introspect` against the live API), not by hand.
- `.claude/` is gitignored except for this skill; do not add docs there expecting
  them to be committed.

## Core workflow

### Step 1: Establish the diff

```bash
git diff HEAD --name-only          # default: uncommitted
git diff HEAD --stat
```

With `--since-push`:

```bash
git diff origin/$(git branch --show-current) --name-only
```

### Step 2: Map code changes to doc surfaces

For each changed file, ask which surfaces above it invalidates. The common cases:

| Changed | Update |
|---|---|
| `internal/cli/*.go` (command/flag) | skill doc, README, examples |
| `internal/errs/errs.go` | skill doc error codes (test enforces) |
| output/formatting code | skill doc output shapes, examples, README |
| `internal/api/**` behaviour | examples, possibly ADR-003 validation |

### Step 3: Apply or propose

With edit permission, make the changes and run the guard tests. Without it, present
a diff and wait for approval.

### Step 4: Report

```markdown
## Documentation Update Summary

### Applied
- ✅ `internal/cli/skill.go`: added `mcli item batch`, updated error-code line
- ✅ `README.md`: command reference row for `item batch`
- ✅ `examples/item-create.json`: refreshed to current output shape

### Verified clean
- `memory-bank/axioms.md` — no invariant changed

### Flagged for human review
- ⚠️ Change alters exit code for API errors — contradicts ADR-002, needs a
  superseding ADR or a revert

### Tests
- `go test ./internal/cli/ -run TestSkill` — pass
```

## Exit conditions

- Every invalidated surface is updated or explicitly reported as needing review
- Guard tests pass (`go test ./internal/cli/ -run TestSkill`)
- No committed doc describes unshipped work
