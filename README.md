# sing-box-agent

A lightweight Go service that exposes a signed REST API for managing a
[sing-box](https://github.com/SagerNet/sing-box) instance at runtime. Drop
it onto any Linux box and manage users, inbounds, and subscriptions with
`curl` — no panel, no SSH, no database.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)
![Go version](https://img.shields.io/github/go-mod/go-version/oglenyaboss/sing-box-agent?filename=go.mod)

> Русскоязычная версия README: [README.ru.md](./README.ru.md)

---

## Why

Existing sing-box panels (3X-UI, S-UI, X-UI, and friends) are great for
a single box but awkward once you run more than one server: web UIs are
clicky, SSH-driven config editing doesn't scale, and there's no clean
story for automation.

`sing-box-agent` is the opposite shape: no UI, no opinions about storage,
just a small authenticated HTTP API in front of a local sing-box. Run it
standalone on one server, or stack many agents behind your own control
plane — the contract is the same.

## What it does

- **Users and inbounds over HTTP** — create, update, delete without SSH.
- **Hot config reload** — apply changes without restarting sing-box.
- **Subscription links** — generate `v2ray`, `clash`, or `sing-box`
  subscriptions for clients.
- **Desired-state sync** (optional) — push a full config from a central
  plane; the agent reconciles atomically with rollback on failure.
- **Observability** — Prometheus metrics (`/metrics`), structured JSON
  logs, `/healthz` and `/readyz` probes.
- **Security built in** — bearer-token auth plus HMAC-SHA256 request
  signing with replay protection (timestamp skew + nonce cache).
- **Multi-protocol** — VLESS, VMess, Trojan, Shadowsocks, Hysteria2,
  TUIC, ShadowTLS, all through the same API.

## Architecture

```
          ┌────────────────┐        ┌──────────────────┐
 curl ───▶│  sing-box-     │──────▶│  sing-box core   │
          │  agent :8080   │ reload│  (process on the │
          │                │        │   same host)     │
          └───────┬────────┘        └──────────────────┘
                  │
                  ▼ /metrics /healthz

(Optional: swap `curl` for your own control plane.)
```

One agent runs per VPN host. Local persistence is limited to traffic
counters and the idempotency cache — everything else is either live
sing-box state or driven from outside.

## Requirements

- sing-box ≥ 1.12 installed on the host.
- Linux with `systemd` (default reload strategy) **or** any host where
  you can signal a PID / run a reload command.
- Go ≥ 1.24 if building from source.

## Quick start — standalone

This is the simplest possible setup: one agent, one sing-box, no control
plane. You manage users and inbounds with `curl`.

```bash
# 1. Build the agent (it does not link sing-box; a sing-box runtime
#    binary must still be installed separately on the host).
git clone https://github.com/oglenyaboss/sing-box-agent
cd sing-box-agent
make build

# 2. Generate real secrets for the sample config.
cp deploy/example/agent-config.yaml ./agent-config.yaml
sed -i "s/CHANGE-ME-MIN-32-CHARACTERS-LONG-TOKEN/$(openssl rand -hex 32)/" agent-config.yaml
sed -i "s/CHANGE-ME-HMAC-SECRET-KEY/$(openssl rand -hex 32)/" agent-config.yaml

# 3. Run it (adjust paths as needed).
./sing-box-agent -config ./agent-config.yaml &

# 4. Smoke test.
curl http://localhost:8080/healthz       # -> OK
```

From here, `curl` + the HMAC headers described in
[`docs/integration-guide.md`](./docs/integration-guide.md) is enough to
drive every CRUD endpoint.

## Quick start — Docker

For trying it end-to-end on a workstation, or for running in an
environment without systemd.

```bash
docker build -t sing-box-agent:local .

cp deploy/example/agent-config.yaml ./agent-config.yaml
sed -i "s/CHANGE-ME-MIN-32-CHARACTERS-LONG-TOKEN/$(openssl rand -hex 32)/" agent-config.yaml
sed -i "s/CHANGE-ME-HMAC-SECRET-KEY/$(openssl rand -hex 32)/" agent-config.yaml

docker run --rm -p 8080:8080 -p 9090:9090 \
  -v "$PWD/agent-config.yaml:/etc/sing-box-agent/config.yaml:ro" \
  -v "$PWD/deploy/example/sing-box-config.json:/etc/sing-box/config.json:ro" \
  sing-box-agent:local
```

See [`deploy/docker/docker-compose.yml`](./deploy/docker/docker-compose.yml)
for a reproducible local stack.

**Note:** the image produced by the Dockerfile contains two separate programs:
the MIT-licensed agent and a sing-box executable, which is GPLv3. The two
are not linked — the image merely bundles them side by side. Building and
running it for your own use is fine; if you redistribute the image, you must
comply with GPLv3 for the bundled sing-box binary (its sources are public).
The agent itself stays MIT. See the header of [`Dockerfile`](./Dockerfile)
for details.

## Quick start — systemd

```bash
cd deploy/example
cp ../../sing-box-agent .   # or your locally built binary
sudo ./deploy.sh
```

The script installs the agent binary, writes a unit file, copies the
example configs to `/etc/sing-box-agent/` and `/etc/sing-box/`, and
starts the `sing-box-agent.service`.

## Adding a central control plane (optional)

When you run more than a handful of servers, hand-rolled `curl` scripts
stop being fun. Point the agent at a control plane of your own — any
HTTP service that implements the contract in
[`docs/integration-guide.md`](./docs/integration-guide.md) will do. Set
`fastify_base_url` and `server_id` in the agent config and the agent
will:

- pull desired state from the plane at startup;
- push traffic counters and user-online stats on an interval;
- re-reconcile on drift.

The name `fastify_base_url` is historical — it's the name of the first
reference plane; you don't have to use Node or Fastify to implement one.

## Configuration

The agent reads YAML from `-config <path>` (default
`/etc/sing-box-agent/config.yaml`) and allows every field to be overridden
by an environment variable.

| Field                 | Env var                             | Default                        | Required | Notes                                      |
|-----------------------|-------------------------------------|--------------------------------|----------|--------------------------------------------|
| `api_port`            | `SINGBOX_AGENT_API_PORT`            | `8080`                         | no       | REST API port                              |
| `metrics_port`        | `SINGBOX_AGENT_METRICS_PORT`        | `9090`                         | no       | dedicated Prometheus listener (`/metrics` also stays on the API port) |
| `metrics_username`    | `SINGBOX_AGENT_METRICS_USERNAME`    | — (auth off)                   | no       | Basic-auth user for `/metrics` (both endpoints); auth active when both set |
| `metrics_password`    | `SINGBOX_AGENT_METRICS_PASSWORD`    | — (auth off)                   | no       | Basic-auth password for `/metrics`                                      |
| `token`               | `SINGBOX_AGENT_TOKEN`               | —                              | **yes**  | Bearer token, ≥ 32 chars                   |
| `secret`              | `SINGBOX_AGENT_SECRET`              | —                              | **yes**  | HMAC signing secret                        |
| `singbox_config_path` | `SINGBOX_AGENT_SINGBOX_CONFIG_PATH` | `/etc/sing-box/config.json`    | no       | Path to the sing-box config file           |
| `stats_api_address`   | `SINGBOX_AGENT_STATS_API_ADDRESS`   | `127.0.0.1:9091`               | no       | v2ray_api address for traffic statistics   |
| `reload_strategy`     | `SINGBOX_AGENT_RELOAD_STRATEGY`     | `systemctl`                    | no       | `systemctl` \| `signal` \| `command`       |
| `reload_target`       | `SINGBOX_AGENT_RELOAD_TARGET`       | `sing-box`                     | no       | service name, PID file, or executable      |
| `reload_command`      | `SINGBOX_AGENT_RELOAD_COMMAND`      | —                              | no       | shell command when strategy is `command`   |
| `tls_cert_path`       | `SINGBOX_AGENT_TLS_CERT_PATH`       | —                              | no       | enables TLS when paired with key           |
| `tls_key_path`        | `SINGBOX_AGENT_TLS_KEY_PATH`        | —                              | no       | enables TLS when paired with cert          |
| `fastify_base_url`    | `SINGBOX_AGENT_FASTIFY_URL`         | —                              | no       | optional central control-plane URL         |
| `server_id`           | `SINGBOX_AGENT_SERVER_ID`           | —                              | if above | identifies this agent to the control plane |

> On macOS / BSD use `reload.strategy: signal` or `command` — `systemctl`
> only exists on systemd Linux.

See the annotated [`deploy/example/agent-config.yaml`](./deploy/example/agent-config.yaml)
for a full reference.

## API overview

All authenticated requests require both headers:

```
Authorization: Bearer <token>
X-Signature:   <HMAC-SHA256 of canonical string>
X-Timestamp:   <unix seconds>
X-Nonce:       <random unique nonce>
```

The canonical string is `{nonce}\n{timestamp}\n{METHOD}\n{path}\n{sha256(body)}`.
Note that only the path is signed — the query string is **not** covered by
the signature.
| Endpoint group          | Purpose                               |
|-------------------------|---------------------------------------|
| `GET /healthz`          | liveness probe                        |
| `GET /readyz`           | readiness probe                       |
| `GET /status`           | version, uptime, sync state           |
| `GET /metrics`          | Prometheus metrics                    |
| `*   /inbounds[/…]`     | inbound CRUD                          |
| `*   /inbounds/{tag}/users[/…]` | user CRUD (idempotent creates) |
| `GET /stats/traffic`    | per-inbound cumulative counters       |
| `GET /stats/online`     | currently-connected users (**always returns an empty list** — online detection is not implemented yet; requires the sing-box clash api) |
| `POST /sync/desired-state` | push a full desired state          |
| `GET /sync/status`      | last applied version / timestamp      |
| `POST /subscription/generate` | produce a client config URL     |
| `GET /subscription/{id}`| fetch generated config                |
| `POST /core/reload`     | hot-reload sing-box from disk         |
| `POST /core/restart`    | restart the sing-box process          |
| `GET /core/config`      | current config (secrets redacted)     |

Full schema: [`docs/openapi.yaml`](./docs/openapi.yaml).

## Documentation

- [`docs/integration-guide.md`](./docs/integration-guide.md) — API
  contract, signing scheme, and the optional control-plane protocol.
- [`docs/deployment.md`](./docs/deployment.md) — systemd and Docker
  deployment patterns.
- [`docs/spec.md`](./docs/spec.md) — detailed specification.

## Development

See [CONTRIBUTING.md](./CONTRIBUTING.md) for setup, style, and testing
instructions.

## Security

See [SECURITY.md](./SECURITY.md) for the disclosure policy and
operator-hardening recommendations.

## License
Released under the [MIT License](./LICENSE).

The agent does **not** import sing-box as a Go library and never links
against it: the compiled agent binary is pure MIT. sing-box itself is
GPLv3-licensed and is managed by the agent as a separate process
(systemd, signal, or a custom reload command).

The Docker image is a special case: it bundles a separately built
sing-box executable (GPLv3) next to the MIT-licensed agent. This is
aggregation, not linking — the agent remains MIT. If you redistribute
the image, you only need to comply with GPLv3 for the bundled sing-box
binary, whose sources are publicly available. Running the image for
your own use is unrestricted.
