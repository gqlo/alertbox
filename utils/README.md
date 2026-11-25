# Alertbox systemd install

Run Alertbox as a systemd service on Linux. Files in this directory:

| File | Purpose |
|------|---------|
| `install-alertbox.sh` | Build, install, enable, and restart (one command for updates) |
| `alertbox.service` | systemd unit installed to `/etc/systemd/system/` |
| `alertbox.env.example` | Template for `/etc/alertbox/alertbox.env` |

## Prerequisites

- Go 1.24+ (to build)
- Redis running (`redis.service` or remote `REDIS_ADDR`)
- For Alertmanager polling with `oc port-forward`: `oc` logged in on the install user; `install-alertbox.sh` configures `User` and `KUBECONFIG` automatically when `~/.kube/config` exists

## Quick install

From the repository root:

```bash
./utils/install-alertbox.sh
```

That script:

1. Runs `go test ./...` (unless `--no-test`)
2. Builds `./cmd/alertbox` into a temporary binary
3. Installs the binary to `/usr/local/bin/alertbox` (replaces an existing install)
4. Copies `deploy/openshift/` to `/opt/alertbox/deploy/openshift`
5. Creates `/etc/alertbox/alertbox.env` from `alertbox.env.example` **only if it does not exist**
6. Sets `ALERTBOX_DEPLOY_DIR=/opt/alertbox/deploy/openshift` in the env file (every install)
7. Installs `alertbox.service` to `/etc/systemd/system/`
8. Validates prerequisites (`oc whoami`, `oc apply` RBAC when polling needs `oc`; token file when set)
9. Runs `systemctl daemon-reload`, `enable`, and `restart` (or `start` if not running)
10. Waits until the service stays active for 8+ seconds, then prints `systemctl status`

Install **fails with an error** (not just a warning) if kubeconfig is missing, `oc` is not logged in, RBAC apply fails, or the service crashes on startup.

Open [http://localhost:8083/](http://localhost:8083/) (or your `LISTEN_ADDR`).

## Updating after code changes

Run the same script again:

```bash
./utils/install-alertbox.sh
```

Each run **rebuilds** from the current repo and **overwrites** `/usr/local/bin/alertbox` via `install` (no separate uninstall). OpenShift RBAC manifests are refreshed under `/opt/alertbox/deploy/openshift`. The service is restarted so the new binary is loaded.

Other `alertbox.env` settings are **not** overwritten on updates (only `ALERTBOX_DEPLOY_DIR` is refreshed).

## Script options

| Option | Effect |
|--------|--------|
| `--no-test` | Skip `go test ./...` before build |
| `--no-start` | Install binary and unit only; do not enable or restart the service |
| `-h`, `--help` | Show usage |

Examples:

```bash
./utils/install-alertbox.sh --no-test          # faster iteration while developing
./utils/install-alertbox.sh --no-start       # install only, start manually later
```

## Configuration

Edit `/etc/alertbox/alertbox.env` (see `alertbox.env.example` for all variables). See [configuration.md](../docs/configuration.md) for details.

After changing the env file:

```bash
sudo systemctl restart alertbox
```

## systemd paths

| Path | Description |
|------|-------------|
| `/usr/local/bin/alertbox` | Installed binary |
| `/opt/alertbox/deploy/openshift` | OpenShift RBAC manifests (`oc apply -f`) |
| `/etc/alertbox/alertbox.env` | Environment variables (`EnvironmentFile` in unit) |
| `/etc/alertbox/kubeconfig` | Flattened kubeconfig for systemd (from installer `$KUBECONFIG` at install time) |
| `/etc/systemd/system/alertbox.service` | systemd unit |
| `/etc/systemd/system/alertbox.service.d/install.conf` | Auto-generated `User` / `KUBECONFIG` for `oc` |

## Local dev with `oc` port-forward

When you run `install-alertbox.sh`, the script detects a working `oc` login using:

1. `ALERTBOX_INSTALL_KUBECONFIG` (if set)
2. **`$KUBECONFIG` from your shell** (full value, including colon-separated paths)
3. `oc whoami` in the current shell
4. Login shell / `~/.kube/config` fallback

It materializes credentials to `/etc/alertbox/kubeconfig` and writes `/etc/systemd/system/alertbox.service.d/install.conf` with:

- `User` / `Group` — the install user (`SUDO_USER`, `ALERTBOX_INSTALL_USER`, or `root`)
- `KUBECONFIG` — `/etc/alertbox/kubeconfig`

Override manually:

```bash
export KUBECONFIG=/path/to/your/kubeconfig   # or export before install
oc login ...
./utils/install-alertbox.sh
```

Or:

```bash
export ALERTBOX_INSTALL_USER=youruser
export ALERTBOX_INSTALL_KUBECONFIG=/home/youruser/.kube/config
./utils/install-alertbox.sh
```

If `oc` is not configured, set `ALERTMANAGER_TOKEN_FILE` in `/etc/alertbox/alertbox.env` or disable polling with `ALERTMANAGER_URL=`.

## Manual service commands

```bash
sudo systemctl status alertbox
sudo systemctl restart alertbox
sudo systemctl stop alertbox
journalctl -u alertbox -f
```

## Manual install (without script)

```bash
go build -o alertbox ./cmd/alertbox
sudo install -m 0755 alertbox /usr/local/bin/alertbox
sudo install -d -m 0750 /etc/alertbox
sudo install -m 0640 utils/alertbox.env.example /etc/alertbox/alertbox.env
sudo install -m 0644 utils/alertbox.service /etc/systemd/system/alertbox.service
sudo systemctl daemon-reload
sudo systemctl enable --now alertbox
```
