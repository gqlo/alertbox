# How Alertbox works

**Scope:** what the running process does **today**. Planned VM context collection is documented separately in [vm-context.md](vm-context.md).

Alertbox is a single Go process that **ingests OpenShift/Kubernetes Alertmanager alerts**, **stores them in Redis**, and **serves an HTML dashboard** for triage. Optional pieces: Alertmanager polling (with `oc` port-forward + SA token), Slack notifications on webhook, and a cluster-info sidebar via `oc`.

## Big picture

```text
                    External                         alertbox process
  +---------------------------+     +------------------------------------------+
  | Alertmanager              |     | run.go (package alertbox) wires:         |
  | Slack API                 |     |                                          |
  | OpenShift API             |     |   internal/server (inbound HTTP)         |
  +---------------------------+     |     handlers, details, filters,          |
           ^         ^              |     snapshot, branding, clustermeta       |
           |         |              |          |  depends on                    |
           |         |              |   internal/client (outbound)             |
           |         |              |     alertmanager, portforward, oc,        |
           |         |              |     octoken, clusterinfo, slack           |
           |         |              |          |  depends on                    |
           |         |              |   internal/store ----------------+        |
           |         |              +----------------------------------|--------+
           |         |                                                 v
           |    (oc port-forward, SA token)                          Redis
           |                                                  alertbox:alerts
           |                                                  (+ poll dedup keys)
           +---- poll GET /api/v2/alerts
           +---- POST /webhook  (also optional Slack)
```

## Startup sequence

Entry: [`cmd/alertbox/main.go`](../cmd/alertbox/main.go) → [`Run()`](../run.go).

1. **Redis** — `NewAlertStore()` pings `REDIS_ADDR` (default `localhost:6379`). Fail fast if unreachable.
2. **HTTP server** — `NewServer(store, SLACK_TOKEN, SLACK_CHANNEL_ID)`.
3. **Poll session** (optional) — if polling is enabled, start a background goroutine; may start `oc port-forward` and mint an SA token.
4. **Routes** — register handlers and listen on `LISTEN_ADDR` (default `:8083`).

| Route | Handler | Role |
|-------|---------|------|
| `GET /` | `HandleAlertsPage` | Filterable alert list + cluster sidebar |
| `GET /alert?id=…` | `HandleAlertDetail` | Single alert labels/annotations |
| `POST /webhook` | `HandleWebhook` | Alertmanager push + optional Slack |

Polling is **on by default** (`ALERTMANAGER_URL` defaults to `https://localhost:9094`). Set `ALERTMANAGER_URL=""` for webhook-only mode (Go + Redis only).

## Core data model

| Type | Where | Meaning |
|------|-------|---------|
| `Alert` | [`store.go`](../internal/store/store.go) | Ingest shape from Alertmanager (labels, annotations, status, fingerprint, times) |
| `StoredAlert` | [`store.go`](../internal/store/store.go) | Persisted row: nano-id, `received_at`, `source` (`poll` or `webhook`), plus alert fields |
| `PolledAlert` | [`store.go`](../internal/store/store.go) | Fingerprint + `Alert` for poll sync |
| `AlertStore` | [`store.go`](../internal/store/store.go) | Redis wrapper: `Save`, `SyncPolled`, `List`, `Get` |
| `Server` | [`handlers.go`](../internal/server/handlers.go) | Store + Slack creds + thread map + cached cluster meta |
| `AlertFilters` | [`filters.go`](../internal/server/filters.go) | Query params for the dashboard |

Helpers on `StoredAlert` (used by filters/UI): `Namespace()`, `Pod()`, `Severity()`, `RunbookURL()`, `RowClass()`, etc.

## Redis: the shared hub

Both ingest paths write the same history list. The dashboard only reads Redis — it never talks to Alertmanager directly.

| Key | Type | Purpose |
|-----|------|---------|
| `alertbox:alerts` | List | JSON `StoredAlert` records, newest first (`LPUSH`) |
| `alertbox:active-fingerprints` | Set | Fingerprints currently seen as active (poll) |
| `alertbox:fp-state` | Hash | fingerprint → last status (poll dedup) |
| `alertbox:fp-meta` | Hash | fingerprint → cached `Alert` JSON (for synthesize-resolved) |

Copying history between Redis instances: [redis-migration.md](redis-migration.md).

## Ingest path 1: poll (pull)

```text
  [port-forward if localhost]
            |
            v
  +----- poll interval -----+
  | GET Alertmanager alerts |
  |         |               |
  |         v               |
  |      SyncPolled         |
  |         |               |
  |         v               |
  | update fp-state / meta  |
  |         |               |
  |         v               |
  | LPUSH on change/resolve |
  +-----------+-------------+
              |
              +--> loop
```

**Files:** [`alertmanager.go`](../internal/client/alertmanager.go), [`portforward.go`](../internal/client/portforward.go), [`octoken.go`](../internal/client/octoken.go), [`store.go`](../internal/store/store.go).

**How pieces connect:**

1. **Auth** — Bearer from `ALERTMANAGER_TOKEN_FILE`, or auto `oc apply` of [`deploy/openshift/`](../deploy/openshift/) + `oc create token` for SA `alertbox`. Token is also stored for later `oc` cluster queries (`SetClusterAuthToken`).
2. **Reachability** — For localhost Alertmanager, [`portforward.go`](../internal/client/portforward.go) runs `oc port-forward` to `alertmanager-main` in `openshift-monitoring` (configurable).
3. **Fetch** — HTTP GET to Alertmanager API v2; map AM state → `firing` / `suppressed` / `resolved`.
4. **Dedup** — `SyncPolled` skips writes when fingerprint + status are unchanged. New or changed status → append to `alertbox:alerts` with `source: poll`. Fingerprints that disappear from the API → append a synthetic `resolved` entry using cached meta.

Details: [alert-polling.md](alert-polling.md).

## Ingest path 2: webhook (push)

```text
  Alertmanager POST /webhook
            |
            v
  Save each alert (source=webhook)
            |
            v
  Always LPUSH to Redis
            |
            v
  Optional Slack (daily thread + reply)
```

**Files:** [`handlers.go`](../internal/server/handlers.go) (`HandleWebhook`), [`store.go`](../internal/store/store.go) (`Save`), [`slack.go`](../internal/client/slack.go).

Every webhook alert is appended (`source: webhook`). There is **no** fingerprint dedup on this path — Alertmanager is expected to send meaningful transitions. Slack is **webhook-only**; the poller never posts to Slack.

Details: [webhook-ingest.md](webhook-ingest.md). Short comparison: [alert-flow.md](alert-flow.md).

## Read path: dashboard and detail

```text
  Browser ---- GET / --------> handlers.go dashboard
                 |                    |
                 |                    +--> filters.go
                 |                    +--> store.List --> Redis
                 |                    +--> clusterinfo.go --> oc.go
                 |
                 +-- GET /alert ----> details.go
                                          |
                                          +--> store.Get --> Redis
```

**List (`/`):** load alerts from Redis → [`filters.go`](../internal/server/filters.go) (severity, status, source, time range, namespace, alertname) → paginate → render embedded HTML in [`handlers.go`](../internal/server/handlers.go).
Sidebar shows namespace ranks and **cluster meta** (node counts, OCP/CNV/ODF versions) from [`clusterinfo.go`](../internal/client/clusterinfo.go), refreshed about every 5 minutes via `oc`.

**Detail (`/alert`):** [`details.go`](../internal/server/details.go) looks up one `StoredAlert` by `id` and shows overview, summary/description, runbook/generator links, labels, and annotations. No log/context panels yet (see [vm-context.md](vm-context.md)).

UI details: [dashboard.md](dashboard.md), [endpoints.md](endpoints.md).

## OpenShift helpers: how `oc` fits in

[`oc.go`](../internal/client/oc.go) runs the `oc` binary, preferring the in-memory cluster auth token when set.

| Concern | When it runs | Module |
|---------|--------------|--------|
| SA token mint + RBAC apply | Poll client setup; token refresh on 401; lazy before cluster meta if needed | [`octoken.go`](../internal/client/octoken.go) |
| Port-forward to Alertmanager | Poll session start + recovery when URL is localhost | [`portforward.go`](../internal/client/portforward.go) |
| Cluster sidebar | Dashboard render (cached) | [`clusterinfo.go`](../internal/client/clusterinfo.go) |

Webhook-only mode does not require `oc` unless you also want the cluster sidebar.

## File map (how code is split)

Code is organized into three layered `internal/` packages; dependencies point
downward only (`server` → `client` → `store`).

| Package / File | Responsibility |
|----------------|----------------|
| `cmd/alertbox/main.go` | Process entry |
| `run.go` (`package alertbox`) | Wire store, poller, routes, listen |
| **`internal/store`** | |
| `store.go` | Types (`Alert`, `StoredAlert`, `PolledAlert`) + Redis persistence / poll dedup |
| `timefmt.go` | Absolute / relative time helpers |
| **`internal/client`** | Outbound integrations |
| `alertmanager.go` | Poll HTTP client, loop, error recovery |
| `portforward.go` | `oc port-forward` lifecycle |
| `octoken.go` | Deploy RBAC + mint SA token |
| `oc.go` | Shared `oc` execution |
| `clusterinfo.go` | Sidebar cluster versions / node counts |
| `slack.go` | Slack `chat.postMessage` |
| **`internal/server`** | Inbound HTTP |
| `handlers.go` | Dashboard template + webhook + Slack orchestration |
| `details.go` | Alert detail page |
| `filters.go` | Query filters, ranking, pagination |
| `snapshot.go` | In-memory filtered-view cache |
| `branding.go` | Header / branding HTML |
| `clustermeta.go` | Cached cluster-meta refresh on the `Server` |
| `deploy/openshift/` | Manifests applied for local SA token |

Tests live beside each package (`internal/<pkg>/*_test.go`). Module import path
root: `github.com/gqlo/alertbox`; the internal packages are
`github.com/gqlo/alertbox/internal/{store,client,server}`.

## Configuration that shapes the live flow

| Area | Key env vars |
|------|----------------|
| Process | `REDIS_ADDR`, `LISTEN_ADDR` |
| Poll | `ALERTMANAGER_URL`, `ALERTMANAGER_POLL_INTERVAL`, `ALERTMANAGER_TLS_INSECURE`, `ALERTMANAGER_TOKEN_FILE` |
| Port-forward | `ALERTMANAGER_PORT_FORWARD`, `ALERTMANAGER_PF_*` |
| Token mint | `ALERTBOX_NAMESPACE`, `ALERTBOX_SA`, `ALERTBOX_TOKEN_DURATION`, `ALERTBOX_DEPLOY_DIR` |
| Slack | `SLACK_TOKEN`, `SLACK_CHANNEL_ID` |

Full list: [configuration.md](configuration.md).

## What is connected vs not yet

| Connected today | Not implemented yet |
|-----------------|---------------------|
| Alertmanager ↔ poller / webhook ↔ Redis ↔ dashboard | Per-alert log / VMI / PVC / node context ([vm-context.md](vm-context.md)) |
| Optional Slack on webhook | Slack on poll |
| `oc` for token, port-forward, cluster meta | AI summarization |

## Related docs

| Doc | Topic |
|-----|-------|
| [alert-flow.md](alert-flow.md) | Short poll vs webhook diagram |
| [alert-polling.md](alert-polling.md) | Poll API, RBAC, dedup |
| [webhook-ingest.md](webhook-ingest.md) | Webhook receiver and testing |
| [dashboard.md](dashboard.md) | Filters and pagination |
| [endpoints.md](endpoints.md) | HTTP routes |
| [configuration.md](configuration.md) | Environment variables |
| [vm-context.md](vm-context.md) | Design for future VM context collection |
| [redis-migration.md](redis-migration.md) | Copy Redis alert history |
