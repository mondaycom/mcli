# ADR-004: Secret storage — keychain default, age-encrypted file fallback

## Status
Active

## Context
mcli authenticates to monday.com with an API token. Phase 1 stored the token as plaintext YAML at `~/.config/mcli/config.yaml` with mode 0600. That is insufficient (axiom A11): file-mode bits do not protect against backups, cloud sync, dotfiles repositories, or any process with read access to the user's home directory.

mcli is also LLM-primary (axiom A12). The read path — every command an agent calls programmatically — MUST be fully non-interactive. Interactive UX is acceptable only at one-time setup.

This ADR fixes:
- Where API tokens are stored at rest.
- How `auth login` chooses a backend, and how the choice is persisted.
- How tokens are read at runtime by every other command.
- Where passphrases come from (file backend only).

## Alternatives

### Storage backend
- **OS keychain only** — macOS Keychain, Linux Secret Service, Windows Credential Manager via `zalando/go-keyring`. Best UX. On Linux without a running secret-service daemon (CI, headless containers), keychain is unavailable. Rejected as the *only* backend because it locks out headless environments.
- **Encrypted file only** — works everywhere but requires passphrase handling on every read. Rejected as the only backend because it forces users with a perfectly good keychain to manage a passphrase.
- **Keychain + encrypted-file fallback** — keychain by default, encrypted file when explicitly chosen. *Chosen.*
- **Plaintext file with 0600** — what Phase 1 had. Rejected (violates A11).
- **Env-only (no on-disk storage at all)** — simplest but hostile to first-time human setup. Rejected as the only mechanism, but `MONDAY_API_TOKEN` env still works as a runtime override.

### Encryption library for the file backend
- **`filippo.io/age`** — modern, audited, designed for passphrase-or-recipient encryption, BSD-3-Clause, pure Go, small API. *Chosen.*
- **`golang.org/x/crypto/nacl/secretbox`** — primitive; would require us to derive a key with scrypt/argon2 and roll our own framing. More code we'd own. Rejected.
- **OpenSSL CLI shell-out** — non-starter (axiom A3 — single static binary, no runtime deps).

### Passphrase source for the file backend
- **`MCLI_PASSPHRASE` env var only** — *chosen.*
- **Interactive prompt with `golang.org/x/term`** — violates axiom A12 (LLM-primary, non-interactive read path) and adds a dep purely for human ergonomics. Rejected.
- **Hybrid: env var or prompt** — splits behavior between TTY and non-TTY, breeds bugs. Rejected.

### Backend selection mechanism
- **Explicit, persisted in config** — user picks at `auth login` via `--store keychain|file`; choice is recorded in `~/.config/mcli/config.yaml` under `secret_store`. *Chosen.*
- Silent fallback (try keychain, fall back to file on failure) — surprising; users may not know which backend is actually holding their secret. Rejected.

## Decision

### Backends
- **`keychain`** (default) — `github.com/zalando/go-keyring` (MIT). Service `"mcli"`, account `"monday-api-token"`. No passphrase. Returns `ErrUnavailable` when the OS keyring API is missing or fails (e.g. no Secret Service daemon).
- **`file`** — age passphrase encryption (`filippo.io/age`, BSD-3-Clause). File at `$XDG_CONFIG_HOME/mcli/credentials.age` (mode 0600, dir 0700). Passphrase from `MCLI_PASSPHRASE` env var; if unset, read returns `AUTH` error and login returns `USAGE` error.

### Configuration layout
`~/.config/mcli/config.yaml` no longer holds a token. It holds non-secret preferences:

```yaml
secret_store: keychain   # or "file"; absent until `auth login` succeeds
```

When `Load` encounters a legacy `token:` field, the field is silently dropped, the file is rewritten, and a one-line warning is emitted to stderr.

### `auth login`
```
mcli auth login --token <token> [--store keychain|file]
```
- Validates `--token` is non-empty.
- Backend defaults to `keychain`. If `--store file`, requires `MCLI_PASSPHRASE` env var.
- Writes the token to the chosen backend.
- Persists `secret_store: <chosen>` in config.yaml.
- Idempotent: rerunning replaces the stored token.

### `auth logout`
```
mcli auth logout
```
- Reads `secret_store` from config.
- Deletes the secret from that backend.
- Clears `secret_store` from config.

### `auth status`
```
mcli auth status
```
- Prints the active backend and whether a secret is present. NEVER prints the token.

### Token resolution at runtime
Used by every command that needs to authenticate. Precedence:
1. `--token` flag.
2. `MONDAY_API_TOKEN` env var.
3. The configured secret store (read via `internal/secrets`).
4. If none yields a token: `AUTH` error pointing the user at `mcli auth login` or the env var.

The keychain and file backends are never tried unless config records a `secret_store`. We do not silently probe.

### Secret store interface (internal/secrets)
```go
type Store interface {
    Put(token APIToken) error
    Get() (APIToken, error)
    Delete() error
    Available() error // nil if usable; non-nil error explains why not
}
```

## Consequences

### Positive
- Tokens are no longer recoverable from a stolen `~/.config` snapshot, satisfying A11.
- Read path is fully non-interactive — keychain reads need no input; file reads consume `MCLI_PASSPHRASE` env. Satisfies A12.
- Backend choice is explicit and visible (`mcli auth status`); no surprise failovers.
- Headless environments are first-class: `--store file` + `MCLI_PASSPHRASE` works in CI without a keyring.

### Negative
- Two new third-party deps (`zalando/go-keyring` MIT, `filippo.io/age` BSD-3-Clause). Both reviewed under A10.
- File backend requires the user to manage `MCLI_PASSPHRASE` themselves (shell, direnv, 1Password CLI wrapper). For LLM agents this is a wrapper-script concern, not an mcli concern.
- Linux behavior depends on a running Secret Service daemon. Documented in `auth status` and in errors.
- A small migration cost: any existing plaintext config files from Phase 1 are wiped of their `token:` field on first read. The user re-runs `auth login` once.

### Out of scope (deferred)
- Passphrase agents (ssh-agent-style caching). Not needed for v0.
- Hardware-key-backed storage (YubiKey, TPM).
- 1Password / Bitwarden CLI integration. Users can wrap mcli themselves by exporting `MONDAY_API_TOKEN` from their secret manager.
- Per-account / multi-tenant tokens. Single-token model in v0.

---
Supersedes: none. Amends ADR-002 (token precedence updated to consult the secret store; `MCLI_PASSPHRASE` added to documented env vars).
