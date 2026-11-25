# Alert polling

Polling is **enabled by default** against `https://localhost:9094`. Alertbox starts `oc port-forward` automatically when the URL is localhost (or unset). Set `ALERTMANAGER_PORT_FORWARD=false` to manage port-forward yourself, or `ALERTMANAGER_URL=""` to disable polling entirely.

Every `ALERTMANAGER_POLL_INTERVAL` (default **10s**), Alertbox calls:

```http
GET {ALERTMANAGER_URL}/api/v2/alerts?active=true&silenced=true&inhibited=true&unprocessed=true
Authorization: Bearer {token}
```

This uses Alertmanager's default inclusive filters so **all alerts currently held in Alertmanager** are returned (firing, silenced, inhibited, and unprocessed). Alertmanager still drops resolved alerts after `resolve_timeout` (~5 minutes) — this is not long-term history.

## Sample response

Alertmanager returns a JSON array. See [alertmanager-api-response.json](alertmanager-api-response.json).

| Field | Used by Alertbox |
|-------|------------------|
| `fingerprint` | Deduplication — one stored row per alert instance |
| `labels` | Alert name, severity, namespace, pod, etc. |
| `annotations` | Summary, description, `runbook_url` link |
| `status.state` | `active`/`unprocessed` → firing, `suppressed` → suppressed, past `endsAt` → resolved |
| `startsAt` / `endsAt` | Alert detail page |
| `updatedAt` | Dashboard **Updated at** column (falls back to `received_at` when missing) |
| `receivers`, `generatorURL` | Ignored |

When an alert disappears from the API response on the next poll, Alertbox records a `resolved` entry using cached labels and annotations.

## Processing

For each returned alert:

1. Read `fingerprint`, `labels`, and `annotations`
2. Map Alertmanager state to stored status (`firing`, `suppressed`, or `resolved` when `endsAt` is in the past)
3. Pass to `AlertStore.SyncPolled`, which:
   - **New alert** (unknown fingerprint) → append to Redis with `source: poll`
   - **Unchanged alert** (same fingerprint + status) → skip (no duplicate row)
   - **Missing alert** (fingerprint was active, no longer in API response) → append a `resolved` entry

Fingerprint state is tracked in Redis:

| Key | Purpose |
|-----|---------|
| `alertbox:alerts` | List of JSON `StoredAlert` records (newest first) |
| `alertbox:active-fingerprints` | Set of currently firing fingerprints |
| `alertbox:fp-state` | Last known status per fingerprint |
| `alertbox:fp-meta` | Cached labels/annotations for resolved detection |

## OpenShift setup

### Local dev

Run from repo root (so `deploy/openshift/` is found), or set `ALERTBOX_DEPLOY_DIR`:

```bash
export REDIS_ADDR=localhost:6379
go run ./cmd/alertbox
```

On startup you should see:

```text
Applying OpenShift RBAC from .../deploy/openshift...
Creating token for serviceaccount/alertbox in alertbox (duration=8760h)...
Alertmanager auth: oc create token
```

On each start (when polling is enabled and `ALERTMANAGER_TOKEN_FILE` is unset), Alertbox automatically:

1. Runs `oc apply -f deploy/openshift/` (idempotent RBAC setup)
2. Runs `oc create token alertbox -n alertbox --duration=8760h`
3. Uses that bearer token for Alertmanager polling

Requires `oc login` with permission to apply the manifests and create tokens for the `alertbox` ServiceAccount.

**Optional manual script:** `./deploy/create-local-token.sh` runs the same steps and prints a token for curl testing (not required before `go run`).

### In-cluster

```bash
export ALERTMANAGER_URL=https://alertmanager-main.openshift-monitoring.svc:9094
export ALERTMANAGER_TOKEN_FILE=/var/run/secrets/kubernetes.io/serviceaccount/token
export ALERTMANAGER_TLS_INSECURE=true
export ALERTMANAGER_PORT_FORWARD=false
```

Set `ALERTMANAGER_TOKEN_FILE` to skip auto-minting.

The token needs permission to read Alertmanager. On OpenShift 4.15+, `cluster-monitoring-view` alone is **not** enough for the Alertmanager API (`alertmanagers/api`); bind `monitoring-alertmanager-view` in `openshift-monitoring` as well (included in `deploy/openshift/`).

### RBAC manifests

| File | Purpose |
|------|---------|
| `namespace.yaml` | Namespace `alertbox` |
| `serviceaccount.yaml` | ServiceAccount `alertbox` |
| `clusterrolebinding.yaml` | Grants `cluster-monitoring-view` to the SA |
| `rolebinding-alertmanager.yaml` | Grants `monitoring-alertmanager-view` in `openshift-monitoring` |
| `clusterrole-cluster-meta.yaml` | Lets the SA list nodes and read cluster version |
| `role-csv-*.yaml` / `rolebinding-csv-*.yaml` | Lets the SA read CNV/ODF operator CSV versions |

## Manual test

With port-forward running on 9094:

```bash
TOKEN=$(oc create token alertbox -n alertbox --duration=8760h)
curl -sk -H "Authorization: Bearer $TOKEN" \
  'https://localhost:9094/api/v2/alerts?active=true&silenced=true&inhibited=true&unprocessed=true' | jq .
```

## Disable polling

- `ALERTMANAGER_URL=""` — turn off polling entirely (webhook-only mode)
- `ALERTMANAGER_PORT_FORWARD=false` — use your own port-forward or in-cluster URL

See [configuration.md](configuration.md) for all Alertmanager-related environment variables.
