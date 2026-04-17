# sing-box-agent Specification

**Version:** 1.2.0-draft  
**Date:** 2026-02-19  
**Status:** Ready for Developer Handoff

---

## 1. Overview

### 1.1 Purpose

A lightweight Go service that embeds sing-box as a library and provides a REST API for remote management. Designed to replace 3x-ui panels and SSH-based config management while maintaining multi-server architecture.

### 1.2 Goals

- **Runtime user management** without process restarts
- **Full protocol support** via sing-box (VLESS, Hysteria2, ShadowTLS, TUIC, etc.)
- **Multi-server architecture** — one agent per VPN server, controlled centrally
- **Integration** with existing central control plane (Fastify reference impl) and PostgreSQL infrastructure
- **Prometheus metrics** export for Grafana dashboards
- **Zero-dependency** deployment (single binary)
- **State consistency** — API is source of truth, agent applies desired state

### 1.3 Non-Goals

- Web UI (existing bot + web handle this)
- Database storage (delegated to central API)
- User authentication (delegated to central API)
- Multi-tenancy (one agent = one server)

---

## 2. Architecture

### 2.1 State Management Model

**CRITICAL: API is the Source of Truth**

```
PostgreSQL (central control plane (Fastify reference impl))         sing-box-agent
┌─────────────────────┐         ┌─────────────────────┐
│ Source of Truth     │         │ Desired State Cache │
│ - Users             │────────▶│ - Synced on startup │
│ - Inbounds          │         │ - Updated via API   │
│ - Configs           │         └──────────┬──────────┘
└─────────────────────┘                    │
                                           ▼
                                ┌─────────────────────┐
                                │ Actual State        │
                                │ - Runtime users     │
                                │ - Connections       │
                                │ - Traffic counters  │
                                └─────────────────────┘
```

**Bootstrap Flow (on agent startup):**

1. Agent starts
2. Agent loads base config from local file
3. Agent calls `GET /api/v1/servers/{serverId}/agent/desired-state` (PULL)
4. Agent applies desired state to sing-box
5. Agent sets readyz = true
6. Agent starts traffic reporting loop

**Sync Direction Model:**

| Trigger                      | Direction | Endpoint                   |
| ---------------------------- | --------- | -------------------------- |
| Agent startup                | PULL      | `GET /desired-state`       |
| User created/updated/deleted | PUSH      | `POST /sync/desired-state` |
| Periodic drift check (5m)    | PULL      | `GET /desired-state`       |

**Version Conflict Resolution:**

- Each sync carries monotonically increasing `version` number
- Agent rejects stale versions: if `request.version <= current_version`, return 409 `VERSION_CONFLICT`
- On conflict, API should re-fetch current state and retry

---

## 3. API Specification

### 3.1 Response Envelope

All responses follow this format (aligned with central control plane (Fastify reference impl)):

**Success:**

```json
{ "success": true, "data": { ... } }
```

**Error:**

```json
{ "success": false, "error": { "code": "USER_NOT_FOUND", "message": "..." } }
```

### 3.2 Error Codes

| Code                   | HTTP | Description                        |
| ---------------------- | ---- | ---------------------------------- |
| `INVALID_REQUEST`      | 400  | Malformed request                  |
| `UNAUTHORIZED`         | 401  | Missing/invalid auth               |
| `REPLAY_DETECTED`      | 401  | Replay attack (nonce/timestamp)    |
| `INBOUND_NOT_FOUND`    | 404  | Inbound doesn't exist              |
| `USER_NOT_FOUND`       | 404  | User (subId) not found             |
| `USER_EXISTS`          | 409  | User already exists                |
| `IDEMPOTENCY_CONFLICT` | 409  | Same idempotency key, diff payload |
| `VERSION_CONFLICT`     | 409  | Stale sync version                 |
| `SYNC_FAILED`          | 500  | Sync failed                        |
| `SERVICE_UNAVAILABLE`  | 503  | Agent not ready                    |

### 3.3 Authentication

**Required:** Bearer token + request signing

```http
Authorization: Bearer <agent-token>
X-Signature: <hmac-sha256(canonical_string, secret)>
X-Timestamp: 1735689600
X-Nonce: <uuid-v4>
```

**Request Signing Canonicalization:**

```
canonical_string = "{nonce}\n{timestamp}\n{method}\n{path}\n{body_hash}"
body_hash = SHA256(request_body).hexdigest()  // empty string if no body
signature = HMAC-SHA256(canonical_string, secret).hexdigest()
```

**Replay Protection:**

- Timestamp: Unix seconds, reject if |server_time - timestamp| > 300s
- Nonce: UUID v4, stored in LRU cache (24h TTL, 10k max entries)
- On replay attempt: return 401 UNAUTHORIZED with code `REPLAY_DETECTED`

### 3.4 Idempotency

All mutating endpoints support idempotency:

```http
POST /inbounds/vless-reality/users
Idempotency-Key: device-123-create-user
```

**Behavior:**
| Scenario | Response |
|----------|----------|
| Same key + same payload | 200/201 with original response (replay) |
| Same key + different payload | 409 `IDEMPOTENCY_CONFLICT` |
| New key | Normal processing, key cached for 24h |

- Keys stored in LRU cache (24h TTL, 10k max entries)
- Keys are per-server scope (not global)
- Body comparison: SHA256 hash equality

### 3.5 Identity Model

- `subId` — subscription ID (primary identifier, matches Device.subId)
- `uuid` — VLESS/VMess UUID (protocol-specific)
- `inboundTag` — which inbound this user belongs to

### 3.6 Key Endpoints

```
GET /healthz
GET /readyz  → includes sync_status
GET /status  → includes sync state

GET /inbounds
GET /inbounds/{tag}
POST /inbounds
PUT /inbounds/{tag}
DELETE /inbounds/{tag}

GET /inbounds/{tag}/users
POST /inbounds/{tag}/users  → Idempotency-Key required
PUT /inbounds/{tag}/users/{subId}
DELETE /inbounds/{tag}/users/{subId}

GET /stats/traffic
GET /stats/online

POST /sync/desired-state  → API pushes state
GET /sync/status

POST /subscription/generate
GET /subscription/{subId}

POST /core/reload
POST /core/restart
GET /core/config  → sensitive fields REDACTED
```

---

## 4. Traffic Counter Semantics

- Counters are **cumulative** (not delta)
- Counter resets tracked with `reset_at` timestamp
- Response includes `counters_reset_at` field

---

## 5. Prometheus Metrics

```prometheus
singbox_agent_info{version="1.0.0", singbox_version="1.12.21"} 1
singbox_agent_sync_status{state="synced"} 1
singbox_agent_sync_timestamp_seconds 1735689600
singbox_inbound_users{tag="vless-reality", type="vless"} 100
singbox_inbound_connections{tag="vless-reality"} 45
singbox_inbound_traffic_up_bytes_total{tag="vless-reality"} 1073741824
singbox_inbound_traffic_down_bytes_total{tag="vless-reality"} 2147483648
```

**Per-user metrics DISABLED by default** (cardinality risk).

---

## 6. Integration with central control plane (Fastify reference impl)

### 6.1 PostgreSQL Schema Changes

```prisma
model Server {
  // ... existing fields ...
  agentEnabled      Boolean  @default(false)
  agentHost         String?
  agentPort         Int      @default(8080)
  agentMetricsPort  Int      @default(9090)
  agentToken        String?
  agentSecret       String?
  agentSyncVersion  Int      @default(0)
}

model DeviceServer {
  // ... existing fields ...
  agentSyncedAt     DateTime?
  agentLastError    String?
  counterResetAt    DateTime?
}
```

### 6.2 Upstream AgentClient

```typescript
export class AgentClient {
  async addUser(
    inboundTag: string,
    user: UserConfig,
    idempotencyKey: string
  ): Promise<{ subId: string; link: string }>;
  async removeUser(inboundTag: string, subId: string, idempotencyKey: string): Promise<void>;
  async pushDesiredState(
    state: DesiredState,
    version: number,
    idempotencyKey: string
  ): Promise<SyncResult>;
}
```

### 6.3 Upstream Webhook Routes

```
GET  /api/v1/servers/:serverId/agent/desired-state
POST /api/v1/servers/:serverId/agent/traffic
```

---

## 7. Security

### 7.1 Required

- **TLS 1.2+** — MUST be enabled for production (TLS 1.3 preferred)
- **Certificate validation** — Verify cert chain, reject self-signed unless explicitly trusted
- **Bearer token** — minimum 32 characters, stored in env (never in config/logs)
- **Request signing** — HMAC-SHA256 with nonce + timestamp
- **Replay protection** — reject requests with timestamp > 5 min old

### 7.2 Redaction

- `GET /core/config` — redact `private_key`, `password`, `short_id`
- Prometheus metrics — no tokens/keys in labels

---

## 8. Rollout Strategy

### 8.1 Per-Server Cutover

1. **Deploy agent** — install, verify health, not in DNS
2. **Dual-run** — new users on agent, existing on 3x-ui
3. **Migrate existing** — sync users, verify traffic
4. **Deprecate 3x-ui** — remove old panel

### 8.2 Rollback Triggers

| Condition            | Action                     |
| -------------------- | -------------------------- |
| Sync failure > 5 min | Alert, manual intervention |
| Drift detected       | Alert, trigger re-sync     |
| API error rate > 5%  | Alert, investigate         |
| User complaints      | Rollback to 3x-ui          |

---

## 9. Development Roadmap

### Phase 1: Core (2 weeks)

- sing-box embedding
- REST API with envelope
- Auth (token + signing)
- Idempotency
- VLESS + Reality
- Bootstrap sync
- Prometheus metrics

### Phase 2: Multi-Protocol (1 week)

- Hysteria2, ShadowTLS, TUIC

### Phase 3: Operations (1 week)

- Traffic webhook, subscription generation, graceful shutdown

### Phase 4: Integration (1 week)

- Upstream AgentClient, reconciler, DB migrations, rollback tooling

---

## Changelog

| Version     | Date       | Changes                                                                                                                                                                                         |
| ----------- | ---------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1.0.0-draft | 2026-02-19 | Initial specification                                                                                                                                                                           |
| 1.1.0-draft | 2026-02-19 | Oracle review: state sync model, idempotency, error codes, response envelope, subId identity, TLS required, request signing, rollout strategy                                                   |
| 1.2.0-draft | 2026-02-19 | Final Oracle review: idempotency semantics (same/diff payload), signing canonicalization, sync direction model (pull startup + push updates), version conflict resolution, TLS 1.2+ requirement |
