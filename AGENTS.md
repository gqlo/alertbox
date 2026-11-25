# AGENTS.md

Guidance for AI coding agents (Cursor, Claude Code, Codex, and others) working
in this repository. This is the single source of truth for project conventions;
`CLAUDE.md` is a symlink to this file.

---

## Project overview

**Repository:** [github.com/gqlo/alertbox](https://github.com/gqlo/alertbox)

alertbox is a **Go HTTP service** that ingests OpenShift / Kubernetes
Alertmanager alerts (poll + webhook), stores them in Redis, and serves a
filterable HTML dashboard for triage. Optional pieces: Slack notifications on
webhook, cluster-info sidebar via `oc`, and (planned) VM alert context
snapshots and AI analysis.

### Architecture

The code is split into three layered `internal/` packages. Dependencies only
point downward (`server` → `client` → `store`); nothing lower imports anything
above it.

- `cmd/alertbox/main.go` — entry point; calls `alertbox.Run()`
- `run.go` (root `package alertbox`) — wires store, server, poller; the only
  place that imports all three internal packages
- **`internal/store`** — domain types (`Alert`, `StoredAlert`, `PolledAlert`),
  `AlertStore` Redis persistence + dedup, and alert-time formatting (`timefmt.go`)
- **`internal/client`** — outbound integrations: Alertmanager API client and
  poll loop (`alertmanager.go`, `portforward.go`), Slack (`slack.go`), and `oc`
  cluster context (`oc.go`, `octoken.go`, `clusterinfo.go`)
- **`internal/server`** — inbound HTTP: handlers + dashboard template
  (`handlers.go`), filters/pagination (`filters.go`), detail page (`details.go`),
  snapshot cache (`snapshot.go`), branding (`branding.go`), cluster-meta caching
  (`clustermeta.go`)
- `docs/` — operator guides; `README.md` is the entry point

Each package owns its own `*_test.go` files. Tests that exercise unexported
behaviour (notably `internal/client`) are white-box (`package client`); the rest
are black-box (`package store_test`, `package server_test`).

### Ingest paths

1. **Poll (pull)** — when `ALERTMANAGER_URL` is set, polls `/api/v2/alerts` on an interval
2. **Webhook (push)** — `POST /webhook` accepts Alertmanager webhook JSON

Both write to the same Redis store and appear on the same dashboard.

### Dependencies

- **Runtime**: Go 1.24+, Redis, optional Alertmanager access, optional Slack bot token
- **Testing**: `go test ./...` — tests in `tests/` use miniredis and httptest
- **CI**: GitHub Actions — `go test`, `go vet`, `markdownlint-cli2`
- Prefer stdlib; the only external runtime dep is `github.com/redis/go-redis/v9`.
  Run `go mod tidy` after adding imports.

### Configuration

All config is via environment variables (`UPPER_SNAKE_CASE`) — see
`docs/configuration.md` (`REDIS_ADDR`, `ALERTMANAGER_URL`, `ALERTBOX_*`, etc.).

---

## Go coding standards

Applies to `**/*.go`.

### Package layout

- Three layered packages under `internal/`: `store` (domain + Redis), `client`
  (outbound integrations), `server` (inbound HTTP). Keep the dependency
  direction `server → client → store`; never import upward
- Shared types (`Alert`, `StoredAlert`) live in `internal/store` — the common
  vocabulary both other packages depend on
- Entry point is `cmd/alertbox/main.go` (`package main`); `run.go` holds `Run()`
- One primary concern per file (`store.go`, `handlers.go`, `filters.go`, …)
- Keep embedded HTML templates in `handlers.go` unless they grow large enough to split

### Error handling

Return errors from helpers; log and respond at HTTP boundaries:

```go
// BAD — swallow error in store layer
func (s *AlertStore) Save(ctx context.Context, alert Alert) {
    s.saveStored(ctx, stored) // ignored error
}

// GOOD — propagate from store, handle in handler
if err := s.store.Save(ctx, alert); err != nil {
    log.Printf("store alert: %v", err)
}
```

Fatal only in `main()` for unrecoverable startup failures (Redis ping, invalid config).

### Context and timeouts

Use `context.WithTimeout` for Redis, HTTP client, and handler operations:

```go
ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
defer cancel()
```

### HTTP handlers

- Check method first; return `405` for wrong method
- Set `Content-Type` before writing body
- Use `http.Error` for plain-text errors; log template/render failures server-side

### Naming

- Exported types only when needed outside the file (most types stay unexported)
- Redis key constants in `store.go` — prefix all keys with `alertbox:`
- Env vars: `UPPER_SNAKE_CASE` matching `docs/configuration.md`

### JSON and time

- Alert/webhook types use `json` tags matching Alertmanager field names
- `StoredAlert.ReceivedAt` uses `received_at` — dashboard date filters depend on this
- Methods on `StoredAlert` for derived fields: `Namespace()`, `Severity()`, `RunbookURL()`, `RowClass()`

### Build and run

```bash
go run ./cmd/alertbox
go build -o alertbox ./cmd/alertbox
```

Binary `alertbox` is gitignored — do not commit it.

---

## Redis store conventions

Applies to `store.go`.

### Key namespace

All keys use the `alertbox:` prefix. Do not introduce unprefixed keys.

| Key | Type | Purpose |
|-----|------|---------|
| `alertbox:alerts` | List | JSON `StoredAlert` records (newest first via `LPUSH`, trimmed to `ALERT_MAX`) |
| `alertbox:active-fingerprints` | Set | Currently firing fingerprints (poll dedup) |
| `alertbox:fp-state` | Hash | Last known status per fingerprint |
| `alertbox:fp-meta` | Hash | Cached labels/annotations (`Alert` JSON) for resolved detection |

### Poll dedup

`SyncPolled` tracks fingerprints across the three keys above. Skip writing when
fingerprint + status is unchanged. When a fingerprint disappears from the API
response, append a `resolved` entry using cached meta.

### Startup

`newAlertStore` pings Redis with a 3s timeout — fail fast if Redis is
unreachable. No in-memory fallback.

### Cross-version migration

`DUMP`/`RESTORE` fails across Redis major versions (e.g. 7.2 → 6.2). Use logical
copy (`LRANGE` → `LPUSH`) — see `docs/redis-migration.md`.

### Testing store changes

- List order (newest first) unchanged
- Nil maps on labels/annotations normalized to `{}` before marshal

---

## Testing

Applies to `tests/**/*.go`.

### File layout

- Tests live next to the package they cover (`internal/<pkg>/*_test.go`)
- `internal/client` uses white-box tests (`package client`) so unexported funcs
  can be tested directly — do not add exported wrappers just to test them
- `internal/store` and `internal/server` use black-box tests
  (`package store_test`, `package server_test`)
- Each package has its own `helpers_test.go` (`testStore`, `testServer`,
  `sampleAlert`, `unsetEnv`); when a local variable would shadow the `store`
  package, name it `st`
- Table-driven tests for filter/parsing logic; `net/http/httptest` for handler tests
- Redis tests use [miniredis](https://github.com/alicebob/miniredis) via
  `store.NewAlertStoreWithRedis` — no live Redis required

### Naming

Pattern: `Test<Function>_<Scenario>`.

```go
func TestFilterAlerts_BySeverity(t *testing.T) { ... }
func TestSyncPolled_ResolvedWhenMissing(t *testing.T) { ... }
func TestHandleWebhook_InvalidJSON(t *testing.T) { ... }
```

### Running tests

```bash
go test ./...
go test -race ./...
go test -race -cover ./...
```

Run the app separately: `go run ./cmd/alertbox`

### Test-fix discipline

When a test fails:

1. **Check the code logic first** — decide whether the failure is a real bug.
2. **Then analyze the test** — is the expectation outdated, or is it correctly
   catching a bug?
3. **Do not change tests solely to make them pass.** Don't relax assertions or
   rewrite expectations just to get green. If the test was correct, fix the
   code; if the test was wrong, fix the test with a clear justification.

Order: code logic → test analysis → fix the right thing.

---

## Pre-commit checks

Before creating any `git commit`, run these checks and fix any failures:

```bash
go test ./...                 # all tests must pass
go vet ./...                  # static analysis
markdownlint-cli2 "**/*.md"   # only when .md files changed
```

Rules:

- If any check fails, **fix the issue before committing** — do not commit broken code.
- Never use `--no-verify` to bypass hooks.
- If a hook rejects the commit, fix the reported issue and create a **new**
  commit (do not amend).
- After committing, run `git status` to verify it succeeded.

Install the hook with:

```bash
git config core.hooksPath hooks
```

Update `hooks/pre-commit` when the CI check scope changes.

---

## CI workflow

GitHub Actions in `.github/workflows/test.yaml`, three independent jobs:

| Job | Tool | What it checks |
|-----|------|----------------|
| `test` | `go test` | `go test -race ./...` |
| `vet` | `go vet` | `go vet ./...` |
| `lint-markdown` | `markdownlint-cli2` | Lints `**/*.md` |

Conventions:

- **Triggers**: push and pull_request to `main`; runner `ubuntu-latest`
- **Go version**: match `go.mod` (currently 1.24)
- **Redis in CI**: not required — unit tests use miniredis
- **Jobs stay independent and parallel** — never add dependencies between them
- Adding a linter (`staticcheck`, `golangci-lint`, …): add a **new job**, not
  inline in an existing one, so failures stay clearly attributable

---

## Documentation diagrams

Use plain-text ASCII diagrams in fenced `text` code blocks. **Do not** use
Mermaid, Graphviz, or other diagram DSLs — they often fail to render in
Cursor / VS Code preview. When editing docs, convert any Mermaid blocks to
ASCII instead of fixing Mermaid syntax.

````markdown
```text
  Alertmanager ---- poll ----> alertmanager.go ----+
       |                                           |
       +------ POST /webhook ----> handlers.go ----+
                                                   v
                                                Redis
```
````

---

## Debug logs

All debug logs (including agent/Cursor debug session logs) **must** go under
`logs/` (e.g. `logs/debug-*.log`) — never under `.cursor/`, the project root,
`tmp/`, or other ad-hoc locations. If a tool provisions a log path under
`.cursor/`, override it and use `logs/` instead. `logs/` and `.cursor/` are
gitignored.

---

## Behavioral guidelines

Guidelines to reduce common LLM coding mistakes. They bias toward caution over
speed; for trivial tasks, use judgment.

### 1. Think before coding

- State assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them — don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop, name what's confusing, and ask.

### 2. Simplicity first

Minimum code that solves the problem. Nothing speculative.

- No features beyond what was asked.
- No abstractions for single-use code, no unrequested "flexibility."
- No error handling for impossible scenarios.
- If 200 lines could be 50, rewrite it. Ask: "Would a senior engineer say this
  is overcomplicated?"

### 3. Surgical changes

Touch only what you must. Clean up only your own mess.

- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken; match existing style.
- Remove imports/variables/functions that **your** changes made unused; leave
  pre-existing dead code (mention it, don't delete it).
- Every changed line should trace directly to the request.

### 4. Goal-driven execution

Define success criteria and loop until verified.

- "Add validation" → "Write tests for invalid inputs, then make them pass."
- "Fix the bug" → "Write a test that reproduces it, then make it pass."
- "Refactor X" → "Ensure tests pass before and after."

For multi-step tasks, state a brief plan with a verify step for each.
