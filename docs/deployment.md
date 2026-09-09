# Deployment Guide

This guide covers the two supported deployment patterns — **Docker** and
**systemd** — plus the configuration knobs they share.

## Prerequisites

- Linux host (any modern distro) **or** a container runtime.
- [sing-box](https://github.com/SagerNet/sing-box) ≥ 1.12 installed on the
  host (systemd path) or fetched automatically inside the image (Docker
  path).
- A client that speaks the agent's [REST API](./integration-guide.md) for
  day-to-day operations.

## Option A — Docker

The project ships a multi-stage Dockerfile that builds the Go binary and
combines it with sing-box in a single Alpine image. sing-box runs in the
background; the agent reloads it with `SIGHUP` via a PID file, so systemd
is not required.

### Build

```bash
docker build -t sing-box-agent:local .
```

Build arguments:

| Arg               | Default | Purpose                                      |
|-------------------|---------|----------------------------------------------|
| `GO_VERSION`      | `1.24`  | Go toolchain version used in the build stage |
| `SINGBOX_VERSION` | `1.12.0`| sing-box release bundled in the runtime stage|
| `VERSION`         | `dev`   | Stamped into the binary (`-X main.Version`)  |
| `GIT_COMMIT`      | `unknown` | Stamped into the binary                    |
| `BUILD_TIME`      | `unknown` | Stamped into the binary                    |

### Run with `docker run`

```bash
# Generate secrets
TOKEN=$(openssl rand -hex 32)
SECRET=$(openssl rand -hex 32)

docker run --rm -d \
  --name sing-box-agent \
  -p 8080:8080 -p 9090:9090 \
  -v "$PWD/deploy/example/sing-box-config.json:/etc/sing-box/config.json:ro" \
  -e SINGBOX_AGENT_TOKEN="$TOKEN" \
  -e SINGBOX_AGENT_SECRET="$SECRET" \
  sing-box-agent:local

curl http://127.0.0.1:8080/healthz   # OK
```

### Run with Compose

A minimal `deploy/docker/docker-compose.yml` is included. Adjust the
published ports for your inbounds and make sure the mounted configs
contain real secrets before bringing it up:

```bash
docker compose -f deploy/docker/docker-compose.yml up --build
```

### Environment-only configuration

Every YAML field can be set via env vars (see the table below), so you can
skip mounting a config file altogether and drive the agent with just:

```bash
-e SINGBOX_AGENT_TOKEN=...  \
-e SINGBOX_AGENT_SECRET=... \
-e SINGBOX_AGENT_LOG_LEVEL=info
```

## Option B — systemd

Use this if you prefer a bare-metal install, already run sing-box under
systemd, or need to integrate with existing host logging / journald.

The bundled helper at [`deploy/example/deploy.sh`](../deploy/example/deploy.sh)
automates the standard install:

```bash
cd deploy/example
# Build a native binary
go build -trimpath -ldflags "-s -w" -o sing-box-agent ../../cmd/agent
# (optionally) edit agent-config.yaml to set a real token/secret
sudo ./deploy.sh
```

The script:

1. Backs up any existing `/etc/sing-box/config.json`.
2. Installs configs to `/etc/sing-box/` and `/etc/sing-box-agent/`.
3. Installs the binary at `/usr/local/bin/sing-box-agent`.
4. Writes `/etc/systemd/system/sing-box-agent.service` with reasonable
   hardening defaults (`NoNewPrivileges`, `ProtectSystem=strict`, etc.).
5. Enables and starts `sing-box` and `sing-box-agent`.

### Manual unit file

If you want to manage the unit yourself:

```ini
[Unit]
Description=sing-box management agent
After=network-online.target sing-box.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/sing-box-agent -config /etc/sing-box-agent/config.yaml
Restart=on-failure
RestartSec=5
User=root
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/etc/sing-box /etc/sing-box-agent /var/lib/sing-box-agent
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now sing-box-agent
```

## Configuration reference

| YAML field            | Env var                             | Default                        |
|-----------------------|-------------------------------------|--------------------------------|
| `api_port`            | `SINGBOX_AGENT_API_PORT`            | `8080`                         |
| `metrics_port`        | `SINGBOX_AGENT_METRICS_PORT`        | `9090` (dedicated listener; `/metrics` also stays on the API port) |
| `metrics_username`    | `SINGBOX_AGENT_METRICS_USERNAME`    | — (auth off; set with password) |
| `metrics_password`    | `SINGBOX_AGENT_METRICS_PASSWORD`    | — (auth off; set with username) |
| `token`               | `SINGBOX_AGENT_TOKEN`               | **required** (≥ 32 chars)       |
| `secret`              | `SINGBOX_AGENT_SECRET`              | **required**                   |
| `singbox_config_path` | `SINGBOX_AGENT_SINGBOX_CONFIG_PATH` | `/etc/sing-box/config.json`    |
| `log_level`           | `SINGBOX_AGENT_LOG_LEVEL`           | `info`                         |
| `stats_api_address`   | `SINGBOX_AGENT_STATS_API_ADDRESS`   | `127.0.0.1:9091`               |
| `reload_strategy`     | `SINGBOX_AGENT_RELOAD_STRATEGY`     | `systemctl`                    |
| `reload_target`       | `SINGBOX_AGENT_RELOAD_TARGET`       | `sing-box`                     |
| `reload_command`      | `SINGBOX_AGENT_RELOAD_COMMAND`      | —                              |
| `tls_cert_path`       | `SINGBOX_AGENT_TLS_CERT_PATH`       | —                              |
| `tls_key_path`        | `SINGBOX_AGENT_TLS_KEY_PATH`        | —                              |
| `fastify_base_url`    | `SINGBOX_AGENT_FASTIFY_URL`         | —                              |
| `server_id`           | `SINGBOX_AGENT_SERVER_ID`           | —                              |

### Reload strategies

`reload_strategy` tells the agent how to apply sing-box config changes:

| Strategy    | When to use                                                          | Target                                     |
|-------------|----------------------------------------------------------------------|--------------------------------------------|
| `systemctl` | Host with systemd (default).                                         | Systemd unit name (default: `sing-box`).   |
| `signal`    | Container / non-systemd host where sing-box writes a PID file.       | Path to the PID file.                      |
| `command`   | Custom orchestrator (runit, s6, supervisord, shell script…).         | Unused; set `reload_command` instead.      |

When `reload_strategy=signal` the agent also uses the PID file to answer
`/readyz` correctly, so readiness probes work inside containers.

## Observability

- `GET /healthz` — liveness (always 200 while the process is up).
- `GET /readyz`  — readiness (503 if sing-box is not running or sync failed).
- `GET /status`  — version, uptime, last sync state.
- `GET /metrics` — Prometheus metrics (default port `9090`).

Structured logs go to stdout/stderr. For Docker use your runtime's log
driver; for systemd hosts they show up in `journalctl -u sing-box-agent`.

## Troubleshooting

### The agent won't start

- Run with `SINGBOX_AGENT_LOG_LEVEL=debug` and re-read the logs — every
  config validation error is printed before the server starts.
- Verify the `token` is at least 32 characters and `secret` is set.
- Ensure `singbox_config_path` exists and is readable.

### API returns `401 UNAUTHORIZED` / `INVALID_SIGNATURE`

- Confirm the caller is sending `Authorization: Bearer <token>` plus
  `X-Signature`, `X-Timestamp`, `X-Nonce`.
- The canonical string is
  `{nonce}\n{timestamp}\n{METHOD}\n{path}\n{sha256(body)}`
  (empty body → empty hash).
- Client and server clocks must agree to within 300 seconds.

### `readyz` says `"singbox":"stopped"` in Docker

- Make sure `reload_strategy` is `signal` and `reload_target` points at a
  PID file the entrypoint actually writes (default: `/run/sing-box.pid`).

### Subscription / user creation fails

- Check that the targeted `inboundTag` exists in the sing-box config.
- Inspect `/inbounds` to see what the agent currently knows about.
- For idempotent endpoints, reuse the same `Idempotency-Key` on retries.
