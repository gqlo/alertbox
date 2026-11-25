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

- `cmd/alertbox/main.go` — entry point; calls `alertbox.Run()`
- `run.go` — wires Redis store, HTTP routes, optional poller and Slack
- `store.go` — Redis persistence, deduplication, `StoredAlert` helpers
- `handlers.go` — HTTP handlers, embedded HTML dashboard template, webhook receiver
- `filters.go` — query-param filters, pagination, URL helpers
- `alertmanager.go` — Alertmanager API client and background polling loop
- `slack.go` — Slack `chat.postMessage` helper
- `details.go` — alert detail page handler
- `clusterinfo.go` / `oc.go` / `octoken.go` / `portforward.go` — `oc` cluster context
- `branding.go` — dashboard header/branding HTML
- `tests/` — external test package (`alertbox_test`)
- `docs/` — operator guides; `README.md` is the entry point

Library code is `package alertbox` (import path `github.com/gqlo/alertbox`).

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

- Library code is `package alertbox` at repo root
- Entry point is `cmd/alertbox/main.go` (`package main`)
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

- Tests live in `tests/` as `package alertbox_test`
- Import `github.com/gqlo/alertbox` — do not use `package main`
- Shared fixtures in `tests/helpers_test.go` (`testStore`, `testServer`, `sampleAlert`)
- Table-driven tests for filter/parsing logic; `net/http/httptest` for handler tests
- Redis tests use [miniredis](https://github.com/alicebob/miniredis) via
  `alertbox.NewAlertStoreWithRedis` — no live Redis required

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
