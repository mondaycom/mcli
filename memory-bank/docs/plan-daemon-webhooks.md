# Plan: mcli Daemon + Webhooks + Notifications

*Created: 2026-05-12*
*Status: not_started*

## Overview

Add a persistent daemon to mcli that receives monday.com webhook events via Cloudflare Quick Tunnels, stores them in a local SQLite inbox, and exposes them to LLM agents through `mcli notification list`. The daemon also manages webhook lifecycle — registering/unregistering hooks with monday.com and re-registering them when the tunnel URL changes.

**Architecture:**

```
monday.com → POST → Cloudflare Quick Tunnel → mcli daemon (localhost)
                                                    ↓
                                              SQLite event store
                                                    ↑
                                     mcli notification list (agent polls)
```

**IPC:** The daemon exposes a Unix socket (`~/.config/mcli/daemon.sock`) for CLI-to-daemon communication. CLI commands (`notification list`, `webhook create`, etc.) talk to the daemon over this socket.

**Key decisions:**
- Storage: SQLite via `modernc.org/sqlite` (pure Go, BSD, no CGO — preserves static binary)
- Tunnel: Cloudflare Quick Tunnels (`cloudflared` external binary, not bundled)
- Daemon gate: All daemon-dependent commands require explicit `mcli daemon start` — no auto-start. Agent must understand a persistent process is involved.
- Webhook registration: One webhook per `create` call (board + event pair). Matches monday's API 1:1.

## Phase 1: Daemon Core + Local IPC
**Status**: not_started

The daemon process that listens on localhost, serves a health/control API over a Unix socket, and manages its own lifecycle (PID file, signal handling).

### Tasks:
- [ ] **`internal/daemon/daemon.go`**: Core daemon struct — HTTP server on configurable port, Unix socket for IPC at `~/.config/mcli/daemon.sock`
- [ ] **`internal/daemon/ipc.go`**: IPC protocol over Unix socket — simple REST-style JSON (status, list-events, ack-event, register-webhook, etc.)
- [ ] **`internal/daemon/pidfile.go`**: PID file at `~/.config/mcli/daemon.pid` — create on start, remove on stop, detect stale PIDs
- [ ] **`mcli daemon start [--port N] [--detach] [--url <external-url>]`**: Start the daemon. If no `--url`, requires cloudflared (Phase 2). `--detach` forks to background.
- [ ] **`mcli daemon stop`**: Sends shutdown signal via IPC socket
- [ ] **`mcli daemon status`**: Reports running/stopped, PID, port, tunnel URL
- [ ] **`internal/cli/daemon_gate.go`**: Helper that checks daemon is running (socket exists + responds to ping); used by notification/webhook commands to gate with "daemon not running, run `mcli daemon start`"
- [ ] Unit tests: start/stop lifecycle, PID file handling, IPC round-trip

### Acceptance Criteria:
- [ ] `AC-P1-1`: `mcli daemon start` starts an HTTP server and creates PID file + socket
- [ ] `AC-P1-2`: `mcli daemon status` reports running state
- [ ] `AC-P1-3`: `mcli daemon stop` cleanly shuts down and removes PID/socket
- [ ] `AC-P1-4`: `mcli notification list` with daemon stopped prints "daemon not running" error with hint

---

## Phase 2: Cloudflare Quick Tunnel Integration
**Status**: not_started

Auto-expose the daemon's local port via Cloudflare Quick Tunnels. Persist the active tunnel URL. Detect URL changes and trigger webhook re-registration (Phase 4).

### Tasks:
- [ ] **`internal/daemon/tunnel.go`**: Launch `cloudflared tunnel --url http://localhost:<port>` as a subprocess, parse the assigned URL from its stderr output
- [ ] **Tunnel lifecycle**: Start with daemon, restart on crash, kill on daemon stop
- [ ] **URL persistence**: Write current tunnel URL to SQLite (or `~/.config/mcli/tunnel_url`) — consumed by webhook registration
- [ ] **URL change detection**: When tunnel restarts and URL changes, emit an internal event that Phase 4 hooks into for re-registration
- [ ] **Fallback `--url`**: If user passes `--url https://my.server/hooks`, skip cloudflared entirely (for production deploys)
- [ ] **Prerequisite check**: On `daemon start` without `--url`, verify `cloudflared` is in PATH; if missing, error with install instructions
- [ ] Unit tests: URL parsing from cloudflared output, fallback mode

### Acceptance Criteria:
- [ ] `AC-P2-1`: `mcli daemon start` launches cloudflared and reports the public URL
- [ ] `AC-P2-2`: `mcli daemon status` shows the active tunnel URL
- [ ] `AC-P2-3`: `mcli daemon start --url https://x` skips cloudflared
- [ ] `AC-P2-4`: If cloudflared is missing, daemon start fails with clear error message

---

## Phase 3: Webhook HTTP Handler + Event Store
**Status**: not_started

The HTTP handler that receives monday.com webhook POSTs, handles the challenge handshake, and stores events in SQLite.

### Tasks:
- [ ] **`internal/daemon/handler.go`**: HTTP handler at `/webhook` — handles monday's challenge verification (echo `{"challenge": token}`) and normal event POSTs
- [ ] **`internal/daemon/store.go`**: SQLite-based event store. DB at `~/.config/mcli/events.db`. Schema: `events(id TEXT PK, received_at TEXT, board_id TEXT, event_type TEXT, webhook_id TEXT, payload JSON, read INTEGER DEFAULT 0)`
- [ ] **Event schema**: Canonical event struct — `{id, received_at, board_id, event_type, webhook_id, payload, read}`
- [ ] **Retention/compaction**: Max events configurable (default 1000), oldest auto-pruned on write. Or TTL-based (7 days default).
- [ ] **Monday webhook auth**: Verify request structure. Research if monday signs payloads (HMAC) and implement if so.
- [ ] Unit tests: challenge handling, event storage CRUD, retention pruning

### Acceptance Criteria:
- [ ] `AC-P3-1`: POST to `/webhook` with challenge payload returns `{"challenge": ...}` (200 OK)
- [ ] `AC-P3-2`: POST to `/webhook` with event payload stores it in SQLite
- [ ] `AC-P3-3`: Events exceeding retention limit are pruned
- [ ] `AC-P3-4`: Stored events are queryable via store interface (list, filter by board/type/read-status)

---

## Phase 4: Webhook Registration + Persistence
**Status**: not_started

CRUD commands for monday.com webhooks. Persist registrations locally so they survive daemon restarts and tunnel URL changes.

### Tasks:
- [ ] **Webhook registry table**: SQLite table `webhooks(id TEXT PK, board_id TEXT, event TEXT, url TEXT, created_at TEXT)` — `id` is monday's webhook ID
- [ ] **`mcli webhook create --board <id> --event <type>`**: Registers with monday via GraphQL `create_webhook` using current tunnel URL. Persists to local registry. Requires daemon running.
- [ ] **`mcli webhook list [--board <id>]`**: Lists locally registered webhooks
- [ ] **`mcli webhook delete <id>`**: Calls monday's `delete_webhook` + removes from local registry
- [ ] **Re-registration on URL change**: When tunnel URL changes (detected in Phase 2), iterate all persisted webhooks, delete old + create new with new URL. Update local registry IDs.
- [ ] **`mcli webhook events`**: List the 21 available WebhookEventType values for discoverability
- [ ] Unit tests: CRUD operations, re-registration flow with mocked GraphQL

### Acceptance Criteria:
- [ ] `AC-P4-1`: `mcli webhook create --board 123 --event create_item` registers with monday and persists locally
- [ ] `AC-P4-2`: `mcli webhook list` shows registered webhooks with board/event/URL
- [ ] `AC-P4-3`: `mcli webhook delete <id>` removes from monday and local store
- [ ] `AC-P4-4`: On tunnel URL change, all webhooks are re-registered with new URL
- [ ] `AC-P4-5`: `mcli webhook events` prints available event types

---

## Phase 5: Notification Commands (Agent Interface)
**Status**: not_started

The commands the LLM agent actually uses to poll the inbox.

### Tasks:
- [ ] **`mcli notification list [--unread] [--board <id>] [--event <type>] [--since <time>] [--limit N]`**: Fetches events from daemon via IPC. JSON output: `{"items": [...], "unread_count": N}`
- [ ] **`mcli notification ack <id>` / `mcli notification ack --all`**: Mark events as read
- [ ] **`mcli notification count`**: Quick unread count (for heartbeat efficiency — cheaper than full list)
- [ ] **Daemon gate**: All notification commands check daemon is running first; if not, return `{"error":{"code":"DAEMON_REQUIRED","message":"...run mcli daemon start..."}}`
- [ ] **Skill doc update**: Add notifications section — "poll `mcli notification count` on heartbeat, fetch details with `mcli notification list --unread`"
- [ ] Unit tests: list/ack/count via mocked IPC

### Acceptance Criteria:
- [ ] `AC-P5-1`: `mcli notification list --unread --json` returns unread events
- [ ] `AC-P5-2`: `mcli notification ack <id>` marks event as read; subsequent `--unread` excludes it
- [ ] `AC-P5-3`: `mcli notification count` returns `{"unread": N}` with daemon running
- [ ] `AC-P5-4`: All notification commands return DAEMON_REQUIRED error when daemon is stopped
- [ ] `AC-P5-5`: Skill doc updated with notification workflow

---

## Phase 6: Integration Testing + Docs
**Status**: not_started

### Tasks:
- [ ] **End-to-end test**: daemon start → webhook create → simulate POST → notification list → ack → notification count = 0
- [ ] **README update**: Add daemon/webhook/notification section
- [ ] **plan-v0.md update**: Add phase entry
- [ ] **ADR**: ADR for daemon architecture decisions (IPC choice, storage format, tunnel strategy)

### Acceptance Criteria:
- [ ] `AC-P6-1`: Full lifecycle e2e test passes
- [ ] `AC-P6-2`: README documents the setup and agent polling workflow

---

## Dependencies

| Package | Purpose | License |
|---------|---------|---------|
| `modernc.org/sqlite` | Pure-Go SQLite (no CGO, preserves static binary) | BSD-3-Clause |
| (external) `cloudflared` | Cloudflare Quick Tunnels binary — user-installed prerequisite | Apache-2.0 |

## Open Questions

1. **Monday webhook signing** — does monday sign webhook payloads (HMAC)? If so we should verify. Need to check their docs.
2. **Daemon logging** — where do daemon logs go? stderr when foreground, file when detached?
3. **Event payload shape** — monday's webhook POST body varies by event type. Do we normalize or store raw?
