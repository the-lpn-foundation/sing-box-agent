# sing-box-agent

A lightweight Go service that exposes a signed REST API for managing a
[sing-box](https://github.com/SagerNet/sing-box) instance at runtime. It
replaces panel-based management (S-UI, 3X-UI, etc.) with a declarative,
programmable control surface that scales across many servers.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)
![Go version](https://img.shields.io/github/go-mod/go-version/oglenyaboss/sing-box-agent?filename=go.mod)

---

## What it does

- **Remote management of inbounds and users** — CRUD over HTTP, no SSH.
- **Desired-state sync** — push a complete config from a central control
  plane; the agent reconciles the local sing-box safely and atomically.
- **Subscription links** — generate `v2ray`, `clash`, or `sing-box`
  subscription URLs for clients.
- **Observability** — Prometheus metrics, structured JSON logs, health
  and readiness probes.
- **Security built in** — bearer-token auth plus HMAC-SHA256 request
  signing with replay protection (timestamp skew + nonce cache).
- **Multi-protocol** — VLESS, VMess, Trojan, Shadowsocks, Hysteria2,
  TUIC, ShadowTLS, all through the same API.

## Architecture

```
    central control plane (optional)
    ──────────┬──────────
              │ HTTPS + HMAC
              ▼
      ┌────────────────┐        ┌──────────────────┐
      │  sing-box-     │──────▶│  sing-box core   │
      │  agent :8080   │ reload│  (process on the │
      │                │        │   same host)     │
      └───────┬────────┘        └──────────────────┘
              │
              ▼ Prometheus / health
```

One agent runs per VPN host. The agent never holds long-lived state — the
control plane is the source of truth; local persistence is limited to
traffic counters and idempotency caches.

## Requirements

- Go ≥ 1.24 (for building from source).
- sing-box ≥ 1.12 installed on the host.
- A Linux host with `systemd` **or** a container runtime (Docker,
  Kubernetes, etc.).

## Quick start

### Docker

```bash
# Build the image
docker build -t sing-box-agent:local .

# Copy the example config and generate real secrets
cp deploy/example/agent-config.yaml ./agent-config.yaml
sed -i "s/CHANGE-ME-MIN-32-CHARACTERS-LONG-TOKEN/$(openssl rand -hex 32)/" agent-config.yaml
sed -i "s/CHANGE-ME-HMAC-SECRET-KEY/$(openssl rand -hex 32)/" agent-config.yaml

# Run (mounts your config; reload strategy = signal to avoid systemd)
docker run --rm -p 8080:8080 -p 9090:9090 \
  -v "$PWD/agent-config.yaml:/etc/sing-box-agent/config.yaml:ro" \
  -v "$PWD/deploy/example/sing-box-config.json:/etc/sing-box/config.json:ro" \
  sing-box-agent:local

# Smoke test
curl http://localhost:8080/healthz     # -> {"status":"healthy"}
```

See [`deploy/docker/docker-compose.yml`](./deploy/docker/docker-compose.yml)
for a reproducible local stack.

### Systemd

```bash
cd deploy/example
# Build a native binary first (or drop one here) -- see below
cp ../../sing-box-agent .
sudo ./deploy.sh
```

The script installs the agent binary, writes a unit file, copies the
example configs to `/etc/sing-box-agent/` and `/etc/sing-box/`, and
starts the `sing-box-agent.service`.

### Build from source

```bash
make build                               # native binary
make release                             # linux+darwin × amd64+arm64 artefacts
```

Or with plain Go:

```bash
go build -trimpath -ldflags "-s -w" -o sing-box-agent ./cmd/agent
```

## Configuration

The agent reads YAML from `-config <path>` (default
`/etc/sing-box-agent/config.yaml`) and allows every field to be overridden
by an environment variable.

| Field                 | Env var                             | Default                        | Required | Notes                                      |
|-----------------------|-------------------------------------|--------------------------------|----------|--------------------------------------------|
| `api_port`            | `SINGBOX_AGENT_API_PORT`            | `8080`                         | no       | REST API port                              |
| `metrics_port`        | `SINGBOX_AGENT_METRICS_PORT`        | `9090`                         | no       | Prometheus `/metrics`                      |
| `token`               | `SINGBOX_AGENT_TOKEN`               | —                              | **yes**  | Bearer token, ≥ 32 chars                   |
| `secret`              | `SINGBOX_AGENT_SECRET`              | —                              | **yes**  | HMAC signing secret                        |
| `singbox_config_path` | `SINGBOX_AGENT_SINGBOX_CONFIG_PATH` | `/etc/sing-box/config.json`    | no       | Path to the sing-box config file           |
| `log_level`           | `SINGBOX_AGENT_LOG_LEVEL`           | `info`                         | no       | `debug` \| `info` \| `warn` \| `error`     |
| `reload.strategy`     | `SINGBOX_AGENT_RELOAD_STRATEGY`     | `systemctl`                    | no       | `systemctl` \| `signal` \| `command`       |
| `reload.target`       | `SINGBOX_AGENT_RELOAD_TARGET`       | `sing-box`                     | no       | service name, PID file, or executable      |
| `tls_cert_path`       | `SINGBOX_AGENT_TLS_CERT_PATH`       | —                              | no       | enables TLS when paired with key           |
| `tls_key_path`        | `SINGBOX_AGENT_TLS_KEY_PATH`        | —                              | no       | enables TLS when paired with cert          |
| `fastify_base_url`    | `SINGBOX_AGENT_FASTIFY_URL`         | —                              | no       | optional upstream control-plane URL        |
| `server_id`           | `SINGBOX_AGENT_SERVER_ID`           | —                              | if above | identifies this agent to the control plane |

See the annotated [`deploy/example/agent-config.yaml`](./deploy/example/agent-config.yaml)
for a full reference and [`docs/integration-guide.md`](./docs/integration-guide.md)
for the integration protocol.

## API overview

All authenticated requests require both headers:

```
Authorization: Bearer <token>
X-Signature:   <HMAC-SHA256 of canonical string>
X-Timestamp:   <unix seconds>
X-Nonce:       <random unique nonce>
```

The canonical string is `{nonce}\n{timestamp}\n{METHOD}\n{path}\n{sha256(body)}`.

| Endpoint group          | Purpose                               |
|-------------------------|---------------------------------------|
| `GET /healthz`          | liveness probe                        |
| `GET /readyz`           | readiness probe                       |
| `GET /status`           | version, uptime, sync state           |
| `GET /metrics`          | Prometheus metrics                    |
| `*   /inbounds[/…]`     | inbound CRUD                          |
| `*   /inbounds/{tag}/users[/…]` | user CRUD (idempotent creates) |
| `GET /stats/traffic`    | per-inbound cumulative counters       |
| `GET /stats/online`     | currently-connected users             |
| `POST /sync/desired-state` | push a full desired state          |
| `GET /sync/status`      | last applied version / timestamp      |
| `POST /subscription/generate` | produce a client config URL     |
| `GET /subscription/{id}`| fetch generated config                |
| `POST /core/reload`     | hot-reload sing-box from disk         |
| `POST /core/restart`    | restart the sing-box process          |
| `GET /core/config`      | current config (secrets redacted)     |

Full schema: [`docs/openapi.yaml`](./docs/openapi.yaml).

## Documentation

- [`docs/integration-guide.md`](./docs/integration-guide.md) — integration
  protocol for a central control plane.
- [`docs/deployment.md`](./docs/deployment.md) — systemd and Docker
  deployment patterns.
- [`docs/spec.md`](./docs/spec.md) — detailed specification
  (authoritative source of truth).
- [`AGENTS.md`](./AGENTS.md) — governance, safety, and operational policies.

## Development

See [CONTRIBUTING.md](./CONTRIBUTING.md) for setup, style, and testing
instructions.

## Security

See [SECURITY.md](./SECURITY.md) for the disclosure policy and
operator-hardening recommendations.

## License

Released under the [MIT License](./LICENSE).
