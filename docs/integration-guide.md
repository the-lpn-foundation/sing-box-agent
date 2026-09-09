# sing-box-agent Integration Guide

This document provides technical details for implementing a client to communicate with `sing-box-agent` instances. It's intended for AI agents and developers working on the central management API.

## 1. Overview
- `sing-box-agent` is a Go service deployed on each VPN server.
- It manages a local `sing-box` instance as a separate process (systemd unit / signal / custom reload command) and exposes a REST API for management.
- Architecture: **central control plane → sing-box-agent (one per server) → sing-box core**. (A Fastify-based reference implementation of the control plane is assumed throughout this guide, but any HTTP service implementing the contract will work.)
- The agent is stateless — the central API is the source of truth for the desired state.
- One agent manages exactly one server (no multi-tenancy).

## 2. Authentication
Protected endpoints require a two-layer authentication mechanism.

### Layer 1: Bearer Token
- Header: `Authorization: Bearer {token}`
- The token is a shared secret (minimum 32 characters), configured via `SINGBOX_AGENT_TOKEN` on the agent.

### Layer 2: HMAC-SHA256 Request Signing
Every request must include the following headers:
- `X-Signature`: HMAC-SHA256 signature.
- `X-Timestamp`: Unix seconds (integer).
- `X-Nonce`: UUID v4 (unique per request).

**Canonicalization logic:**
1. Compute `body_hash`: `SHA256(request_body).hexdigest()`. Use an empty string `""` if there's no request body (GET/DELETE).
2. Construct the `canonical_string`: `"{nonce}\n{timestamp}\n{METHOD}\n{path}\n{body_hash}"`.
3. Compute the `signature`: `HMAC-SHA256(canonical_string, secret).hexdigest()`.
   - The `secret` is configured via `SINGBOX_AGENT_SECRET`.
   - **Only the path is signed.** The query string is NOT covered by the signature — do not rely on the signature to authenticate query parameters of GET requests.

**Security constraints:**
- **Timestamp skew**: The agent rejects requests if the timestamp differs from the server time by more than ±300 seconds.
- **Replay protection**: Nonces are stored in an LRU cache (24h TTL, 10k max entries). Reusing a nonce results in a `401 REPLAY_DETECTED` error.

**Public endpoints (no auth required):**
- `GET /healthz`
- `GET /readyz`
- `GET /status`
- `GET /metrics`

## 3. Response Envelope
All API responses follow a consistent format.

### Success
```json
{
  "success": true,
  "data": { ... }
}
```

### Error
```json
{
  "success": false,
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable description"
  }
}
```

### Error Codes
| Code | HTTP Status | Description |
|------|-------------|-------------|
| INVALID_REQUEST | 400 | Malformed request or validation failure |
| UNAUTHORIZED | 401 | Invalid token or signature |
| REPLAY_DETECTED | 401 | Nonce reuse or timestamp too old |
| INBOUND_NOT_FOUND | 404 | Inbound with given tag doesn't exist |
| USER_NOT_FOUND | 404 | User with given subId doesn't exist |
| USER_EXISTS | 409 | User with this subId already exists in inbound |
| IDEMPOTENCY_CONFLICT | 409 | Same Idempotency-Key but different payload |
| VERSION_CONFLICT | 409 | Desired state version ≤ current version |
| SYNC_FAILED | 500 | Failed to apply desired state |
| SERVICE_UNAVAILABLE | 503 | Agent not ready (sing-box not started) |

## 4. Idempotency
- Header: `Idempotency-Key: {unique-string}`
- Applies to mutating operations: `POST`, `PUT`, `PATCH`, `DELETE`.
- Same key + same payload: Returns the original response (safe retry).
- Same key + different payload: Returns `409 IDEMPOTENCY_CONFLICT`.
- Payload matching is determined by `SHA256(method + "\n" + path + "\n" + body)`.
- Keys are cached for 24 hours in an LRU cache (10k entries max).
- Idempotency middleware is enabled on all mutating endpoints; requests without an `Idempotency-Key` header pass through unchanged.

## 5. API Endpoints Reference

### Health
- **GET /healthz**: Returns basic health status.
  - Response: plain text `OK` (200, `Content-Type: text/plain`). Not JSON.
- **GET /readyz**: Returns readiness status including sync state.
  - Response: `{"success": true, "data": {"ready": true, "sync_status": "synced"}}`
  - Returns `503` if the agent is not ready.
- **GET /status**: Detailed agent status.
  - Response: `{"success": true, "data": {"version": "1.2.0", "singbox_version": "1.12.21", "sync_status": "synced", "sync_timestamp": 1735689600, "uptime_seconds": 3600}}`

### Inbounds CRUD
- **GET /inbounds**: List all inbounds.
- **POST /inbounds**: Create an inbound.
  - Body: `{ "tag": "string", "type": "vless|vmess|trojan|shadowsocks|shadowtls|hysteria2|tuic", "listen": "string", "listen_port": int, "tls": {}, "transport": {} }`
  - Returns `201` on success.
- **GET /inbounds/{tag}**: Get a specific inbound by tag.
- **PUT /inbounds/{tag}**: Partial update of an inbound (tls, transport).
- **DELETE /inbounds/{tag}**: Delete an inbound. Returns `204 No Content`.

### Users CRUD
- **GET /inbounds/{tag}/users**: List all users for an inbound.
- **POST /inbounds/{tag}/users**: Create a user for an inbound.
  - Body: `{ "subId": "required", "uuid": "optional", "name": "string", "email": "string", "flow": "string", "enabled": true, "limitIp": int, "uploadLimit": int, "downloadLimit": int }`
  - **Note:** `limitIp`/`uploadLimit`/`downloadLimit` are accepted for XUI-convention compatibility and stored/echoed by the API, but are **not applied to the sing-box configuration** — sing-box has no native per-user limits. Quota enforcement is the caller's responsibility (e.g., poll `GET /stats/traffic` and disable users).
  - Returns `201` with `{ "subId": "id", "link": "vless://..." }`.
- **GET /inbounds/{tag}/users/{subId}**: Get a specific user by subscription ID.
- **PUT /inbounds/{tag}/users/{subId}**: Partial update of a user.
- **DELETE /inbounds/{tag}/users/{subId}**: Delete a user. Returns `204 No Content`.

### Stats
- **GET /stats/traffic**: Cumulative traffic statistics.
  - Query: `?inbound=tag&start=RFC3339&end=RFC3339`
  - Response: `{ "inbounds": [{ "tag": "tag", "upload": 1024, "download": 2048 }], "counters_reset_at": "..." }`
- **GET /stats/online**: List currently connected users.
  - **Note:** currently always returns `{ "count": 0, "users": [] }` — online-user detection is not implemented yet (it requires the sing-box clash API).
  - Intended shape once implemented: `{ "count": 5, "users": [{ "subId": "id", "inboundTag": "tag", "connected_at": "...", "remote_addr": "..." }] }`

### Sync
- **POST /sync/desired-state**: Push the full desired state.
  - Body: `{ "version": int, "inbounds": [{ ...nested users }] }`
  - The `version` must be strictly increasing. Returns `409 VERSION_CONFLICT` otherwise.
- **GET /sync/status**: Get current synchronization status and version.

### Core
- **POST /core/reload**: Force reload sing-box configuration from disk.
- **POST /core/restart**: Restart the sing-box core process.
- **GET /core/config**: Retrieve current configuration. Sensitive fields like private keys are returned as `"***REDACTED***"`.

### Subscription
- **POST /subscription/generate**: Generate a connection link for a specific user.
  - Body: `{ "subId": "id", "inboundTag": "tag", "format": "v2ray|clash|sing-box", "server": "hostname", "port": 443 }`
- **GET /subscription/{subId}**: Retrieve all available configurations for a user.

## 6. Sync Workflow (Desired State Model)
The agent follows a declarative configuration model where the central API defines the "Desired State".

1. **Source of Truth**: The central API maintains all users, inbounds, and server assignments.
2. **Push Update**: When a change occurs (e.g., a new user is added), the central API pushes the **complete** desired state for that server via `POST /sync/desired-state`.
3. **Version Control**: Every desired state payload includes a monotonically increasing `version`. The agent tracks the current version and rejects any push with a version less than or equal to the current one.
4. **Reconciliation**: Upon receiving the state, the agent compares it with its runtime state and generates an atomic plan to add, remove, or update inbounds and users.
5. **Startup Sync**: On boot, if `SINGBOX_AGENT_FASTIFY_URL` is configured, the agent attempts to pull its initial state from the central API.
6. **Drift Detection**: Every 5 minutes, the agent performs a "drift check" to ensure the runtime sing-box state matches the last successfully applied desired state.

## 7. Integration Checklist for Client Implementation
- [ ] Implement `HMAC-SHA256` signing function according to the canonicalization rules.
- [ ] Implement Bearer token injection into the `Authorization` header.
- [ ] Implement exponential backoff retry logic for `5xx` errors.
- [ ] Generate unique `Idempotency-Key` headers for all mutating operations.
- [ ] Maintain a per-server configuration store (host, port, token, secret).
- [ ] Build a "Desired State Builder" that aggregates all inbounds and users for a server and increments the version.
- [ ] Poll `GET /readyz` for server availability and sync status.
- [ ] Periodically collect traffic statistics via `GET /stats/traffic`.
- [ ] Handle `VERSION_CONFLICT` by fetching the current version from `GET /sync/status` and correcting the next push.

## 8. Code Examples

### Request Signing (TypeScript/Node.js)
```typescript
import crypto from 'crypto';
import { v4 as uuidv4 } from 'uuid';

function signRequest(
  method: string,
  path: string,
  body: string | null,
  secret: string
) {
  const nonce = uuidv4();
  const timestamp = Math.floor(Date.now() / 1000).toString();
  
  const bodyHash = body 
    ? crypto.createHash('sha256').update(body).digest('hex') 
    : '';
    
  const canonicalString = `${nonce}\n${timestamp}\n${method.toUpperCase()}\n${path}\n${bodyHash}`;
  
  const signature = crypto
    .createHmac('sha256', secret)
    .update(canonicalString)
    .digest('hex');
    
  return {
    'X-Signature': signature,
    'X-Timestamp': timestamp,
    'X-Nonce': nonce
  };
}
```

### Authenticated Request
```typescript
async function callAgent(server, method, path, payload = null) {
  const body = payload ? JSON.stringify(payload) : null;
  const headers = {
    'Authorization': `Bearer ${server.token}`,
    'Content-Type': 'application/json',
    ...signRequest(method, path, body, server.secret)
  };

  if (['POST', 'PUT', 'PATCH', 'DELETE'].includes(method)) {
    headers['Idempotency-Key'] = `key-${Date.now()}`;
  }

  const response = await fetch(`${server.url}${path}`, {
    method,
    headers,
    body
  });

  const result = await response.json();
  if (!result.success) {
    throw new Error(`Agent Error [${result.error.code}]: ${result.error.message}`);
  }
  return result.data;
}
```

## 9. OpenAPI Specification
The full OpenAPI 3.0.3 specification is included below. It can be used with tools like `openapi-generator` or `orval` to generate typed clients.

```yaml
openapi: 3.0.3
info:
  title: sing-box-agent API
  version: 1.2.0
  description: |
    A lightweight Go service that manages a local sing-box instance (as a separate process) and provides a REST API for remote management.
    Designed to replace 3x-ui panels and SSH-based config management while maintaining multi-server architecture.

    **Key Features:**
    - Runtime user management without process restarts
    - Full protocol support via sing-box (VLESS, Hysteria2, ShadowTLS, TUIC, etc.)
    - Multi-server architecture — one agent per VPN server, controlled centrally
    - Prometheus metrics export for Grafana dashboards
    - Zero-dependency deployment (single binary)
    - State consistency — API is source of truth, agent applies desired state

  contact:
    name: sing-box-agent
  license:
    name: MIT

servers:
  - url: http://localhost:8080
    description: Local development
  - url: https://agent.example.com
    description: Production server

security:
  - BearerAuth: []
  - RequestSigning: []

tags:
  - name: Health
    description: Health check endpoints
  - name: Inbounds
    description: Inbound configuration management
  - name: Users
    description: User management within inbounds
  - name: Stats
    description: Traffic and connection statistics
  - name: Sync
    description: State synchronization
  - name: Core
    description: Core sing-box operations
  - name: Subscription
    description: Client subscription management

paths:
  /healthz:
    get:
      tags:
        - Health
      summary: Health check
      description: Basic health check endpoint
      operationId: healthz
      responses:
        '200':
          description: Service is healthy
          content:
            text/plain:
              schema:
                type: string
              example: OK

  /readyz:
    get:
      tags:
        - Health
      summary: Readiness check
      description: Readiness check including sync status
      operationId: readyz
      responses:
        '200':
          description: Service is ready
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/SuccessResponse'
              example:
                success: true
                data:
                  ready: true
                  sync_status: synced
        '503':
          description: Service not ready
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
              example:
                success: false
                error:
                  code: SERVICE_UNAVAILABLE
                  message: Agent not ready

  /status:
    get:
      tags:
        - Health
      summary: Server status
      description: Detailed server status including sync state
      operationId: status
      responses:
        '200':
          description: Server status
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/SuccessResponse'
              example:
                success: true
                data:
                  version: "1.2.0"
                  singbox_version: "1.12.21"
                  sync_status: synced
                  sync_timestamp: 1735689600
                  uptime_seconds: 3600

  /inbounds:
    get:
      tags:
        - Inbounds
      summary: List all inbounds
      description: Retrieve all configured inbound connections
      operationId: listInbounds
      responses:
        '200':
          description: List of inbounds
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/SuccessResponse'
                  - type: object
                    properties:
                      data:
                        type: array
                        items:
                          $ref: '#/components/schemas/Inbound'
    post:
      tags:
        - Inbounds
      summary: Create inbound
      description: Create a new inbound configuration
      operationId: createInbound
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/InboundCreate'
      responses:
        '201':
          description: Inbound created successfully
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/SuccessResponse'
        '400':
          description: Invalid request
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'

  /inbounds/{tag}:
    get:
      tags:
        - Inbounds
      summary: Get inbound
      description: Retrieve a specific inbound configuration by tag
      operationId: getInbound
      parameters:
        - name: tag
          in: path
          required: true
          schema:
            type: string
          description: Inbound tag identifier
      responses:
        '200':
          description: Inbound details
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/SuccessResponse'
                  - type: object
                    properties:
                      data:
                        $ref: '#/components/schemas/Inbound'
        '404':
          description: Inbound not found
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
              example:
                success: false
                error:
                  code: INBOUND_NOT_FOUND
                  message: Inbound 'vless-reality' not found
    put:
      tags:
        - Inbounds
      summary: Update inbound
      description: Update an existing inbound configuration
      operationId: updateInbound
      parameters:
        - name: tag
          in: path
          required: true
          schema:
            type: string
          description: Inbound tag identifier
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/InboundUpdate'
      responses:
        '200':
          description: Inbound updated successfully
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/SuccessResponse'
        '404':
          description: Inbound not found
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
    delete:
      tags:
        - Inbounds
      summary: Delete inbound
      description: Delete an inbound configuration
      operationId: deleteInbound
      parameters:
        - name: tag
          in: path
          required: true
          schema:
            type: string
          description: Inbound tag identifier
      responses:
        '204':
          description: Inbound deleted successfully
        '404':
          description: Inbound not found
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'

  /inbounds/{tag}/users:
    get:
      tags:
        - Users
      summary: List users
      description: Retrieve all users for a specific inbound
      operationId: listUsers
      parameters:
        - name: tag
          in: path
          required: true
          schema:
            type: string
          description: Inbound tag identifier
      responses:
        '200':
          description: List of users
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/SuccessResponse'
                  - type: object
                    properties:
                      data:
                        type: array
                        items:
                          $ref: '#/components/schemas/User'
        '404':
          description: Inbound not found
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
    post:
      tags:
        - Users
      summary: Create user
      description: Create a new user for the specified inbound
      operationId: createUser
      parameters:
        - name: tag
          in: path
          required: true
          schema:
            type: string
          description: Inbound tag identifier
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/UserCreate'
      responses:
        '201':
          description: User created successfully
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/SuccessResponse'
                  - type: object
                    properties:
                      data:
                        $ref: '#/components/schemas/UserCreateResponse'
        '400':
          description: Invalid request
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
        '409':
          description: User already exists or idempotency conflict
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'

  /inbounds/{tag}/users/{subId}:
    get:
      tags:
        - Users
      summary: Get user
      description: Retrieve a specific user by subscription ID
      operationId: getUser
      parameters:
        - name: tag
          in: path
          required: true
          schema:
            type: string
          description: Inbound tag identifier
        - name: subId
          in: path
          required: true
          schema:
            type: string
          description: Subscription ID (primary identifier)
      responses:
        '200':
          description: User details
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/SuccessResponse'
                  - type: object
                    properties:
                      data:
                        $ref: '#/components/schemas/User'
        '404':
          description: User not found
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
              example:
                success: false
                error:
                  code: USER_NOT_FOUND
                  message: User 'device-123' not found
    put:
      tags:
        - Users
      summary: Update user
      description: Update an existing user configuration
      operationId: updateUser
      parameters:
        - name: tag
          in: path
          required: true
          schema:
            type: string
          description: Inbound tag identifier
        - name: subId
          in: path
          required: true
          schema:
            type: string
          description: Subscription ID
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/UserUpdate'
      responses:
        '200':
          description: User updated successfully
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/SuccessResponse'
        '404':
          description: User not found
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
    delete:
      tags:
        - Users
      summary: Delete user
      description: Delete a user from the inbound
      operationId: deleteUser
      parameters:
        - name: tag
          in: path
          required: true
          schema:
            type: string
          description: Inbound tag identifier
        - name: subId
          in: path
          required: true
          schema:
            type: string
          description: Subscription ID
      responses:
        '204':
          description: User deleted successfully
        '404':
          description: User not found
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'

  /stats/traffic:
    get:
      tags:
        - Stats
      summary: Traffic statistics
      description: Retrieve cumulative traffic statistics for all inbounds
      operationId: getTrafficStats
      responses:
        '200':
          description: Traffic statistics
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/SuccessResponse'
                  - type: object
                    properties:
                      data:
                        $ref: '#/components/schemas/TrafficStats'

  /stats/online:
    get:
      tags:
        - Stats
      summary: Online users
      description: Retrieve list of currently online users (currently always empty — online detection is not implemented; requires the sing-box clash API)
      operationId: getOnlineUsers
      responses:
        '200':
          description: Online users
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/SuccessResponse'
                  - type: object
                    properties:
                      data:
                        type: array
                        items:
                          $ref: '#/components/schemas/OnlineUser'

  /sync/desired-state:
    post:
      tags:
        - Sync
      summary: Apply desired state
      description: Push desired state from central API to agent
      operationId: syncDesiredState
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/DesiredState'
      responses:
        '200':
          description: State applied successfully
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/SuccessResponse'
                  - type: object
                    properties:
                      data:
                        $ref: '#/components/schemas/SyncResult'
        '409':
          description: Version conflict or idempotency conflict
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'
        '500':
          description: Sync failed
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'

  /sync/status:
    get:
      tags:
        - Sync
      summary: Sync status
      description: Retrieve current synchronization status
      operationId: getSyncStatus
      responses:
        '200':
          description: Sync status
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/SuccessResponse'
                  - type: object
                    properties:
                      data:
                        $ref: '#/components/schemas/SyncStatus'

  /core/reload:
    post:
      tags:
        - Core
      summary: Reload configuration
      description: Reload sing-box configuration from file
      operationId: reloadConfig
      responses:
        '200':
          description: Configuration reloaded successfully
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/SuccessResponse'

  /core/restart:
    post:
      tags:
        - Core
      summary: Restart sing-box
      description: Restart the sing-box core
      operationId: restartCore
      responses:
        '200':
          description: Core restarted successfully
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/SuccessResponse'

  /core/config:
    get:
      tags:
        - Core
      summary: Get configuration
      description: Retrieve current sing-box configuration (sensitive fields redacted)
      operationId: getConfig
      responses:
        '200':
          description: Current configuration
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/SuccessResponse'
                  - type: object
                    properties:
                      data:
                        type: object
                        description: Sing-box configuration with sensitive fields redacted

  /subscription/generate:
    post:
      tags:
        - Subscription
      summary: Generate client config
      description: Generate client configuration for a user
      operationId: generateSubscription
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required:
                - subId
                - inboundTag
              properties:
                subId:
                  type: string
                  description: Subscription ID
                inboundTag:
                  type: string
                  description: Inbound tag
                format:
                  type: string
                  enum: [v2ray, clash, sing-box]
                  default: sing-box
                  description: Output format
      responses:
        '200':
          description: Configuration generated successfully
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/SuccessResponse'
                  - type: object
                    properties:
                      data:
                        $ref: '#/components/schemas/SubscriptionConfig'

  /subscription/{subId}:
    get:
      tags:
        - Subscription
      summary: Get subscription
      description: Retrieve subscription for a user
      operationId: getSubscription
      parameters:
        - name: subId
          in: path
          required: true
          schema:
            type: string
          description: Subscription ID
      responses:
        '200':
          description: Subscription details
          content:
            application/json:
              schema:
                allOf:
                  - $ref: '#/components/schemas/SuccessResponse'
                  - type: object
                    properties:
                      data:
                        $ref: '#/components/schemas/Subscription'
        '404':
          description: User not found
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ErrorResponse'

components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT
      description: Bearer token for authentication (minimum 32 characters)
    RequestSigning:
      type: apiKey
      in: header
      name: X-Signature
      description: |
        HMAC-SHA256 signature for request authentication

        **Required Headers:**
        - X-Signature: HMAC-SHA256(canonical_string, secret)
        - X-Timestamp: Unix seconds (reject if |server_time - timestamp| > 300s)
        - X-Nonce: UUID v4 (stored in LRU cache, 24h TTL, 10k max entries)

        **Canonicalization:**
        ```
        canonical_string = "{nonce}\n{timestamp}\n{method}\n{path}\n{body_hash}"
        body_hash = SHA256(request_body).hexdigest()  // empty string if no body
        signature = HMAC-SHA256(canonical_string, secret).hexdigest()
        ```

        Note: only the path is signed — the query string is NOT covered by the signature.

  schemas:
    SuccessResponse:
      type: object
      required:
        - success
      properties:
        success:
          type: boolean
          example: true
        data:
          type: object
          description: Response payload

    ErrorResponse:
      type: object
      required:
        - success
        - error
      properties:
        success:
          type: boolean
          example: false
        error:
          type: object
          required:
            - code
            - message
          properties:
            code:
              type: string
              enum:
                - INVALID_REQUEST
                - UNAUTHORIZED
                - REPLAY_DETECTED
                - INBOUND_NOT_FOUND
                - USER_NOT_FOUND
                - USER_EXISTS
                - IDEMPOTENCY_CONFLICT
                - VERSION_CONFLICT
                - SYNC_FAILED
                - SERVICE_UNAVAILABLE
              description: Error code
            message:
              type: string
              description: Human-readable error message

    Inbound:
      type: object
      required:
        - tag
        - type
        - listen
        - listen_port
      properties:
        tag:
          type: string
          description: Unique inbound identifier
        type:
          type: string
          enum: [vless, vmess, trojan, shadowsocks, shadowtls, hysteria2, tuic]
          description: Protocol type
        listen:
          type: string
          description: Listen address (e.g., "0.0.0.0")
        listen_port:
          type: integer
          description: Listen port
        users:
          type: array
          items:
            $ref: '#/components/schemas/User'
        tls:
          type: object
          description: TLS configuration
        transport:
          type: object
          description: Transport layer configuration

    InboundCreate:
      type: object
      required:
        - tag
        - type
        - listen
        - listen_port
      properties:
        tag:
          type: string
          description: Unique inbound identifier
        type:
          type: string
          enum: [vless, vmess, trojan, shadowsocks, shadowtls, hysteria2, tuic]
        listen:
          type: string
        listen_port:
          type: integer
        tls:
          type: object
        transport:
          type: object

    InboundUpdate:
      type: object
      properties:
        tls:
          type: object
        transport:
          type: object

    User:
      type: object
      required:
        - subId
        - uuid
      properties:
        subId:
          type: string
          description: Subscription ID (primary identifier)
        uuid:
          type: string
          format: uuid
          description: VLESS/VMess UUID (protocol-specific)
        name:
          type: string
          description: User display name
        email:
          type: string
          description: User email
        flow:
          type: string
          description: VLESS flow (e.g., "xtls-rprx-vision")
        enabled:
          type: boolean
          default: true
        traffic:
          type: object
          properties:
            upload:
              type: integer
              description: Cumulative upload bytes
            download:
              type: integer
              description: Cumulative download bytes
            reset_at:
              type: string
              format: date-time
              description: Last counter reset timestamp

    UserCreate:
      type: object
      required:
        - subId
      properties:
        subId:
          type: string
          description: Subscription ID
        uuid:
          type: string
          format: uuid
          description: VLESS/VMess UUID (auto-generated if omitted)
        name:
          type: string
        email:
          type: string
        flow:
          type: string
        enabled:
          type: boolean
          default: true
        limitIp:
          type: integer
          description: Accepted for XUI-convention compatibility; stored but NOT enforced in sing-box (no native per-user connection limits)
        uploadLimit:
          type: integer
          description: Accepted for XUI-convention compatibility; stored but NOT enforced in sing-box — quota enforcement is the caller's responsibility
        downloadLimit:
          type: integer
          description: Accepted for XUI-convention compatibility; stored but NOT enforced in sing-box — quota enforcement is the caller's responsibility

    UserUpdate:
      type: object
      properties:
        name:
          type: string
        email:
          type: string
        flow:
          type: string
        enabled:
          type: boolean
        limitIp:
          type: integer
          description: Accepted for XUI-convention compatibility; stored but NOT enforced in sing-box (no native per-user connection limits)
        uploadLimit:
          type: integer
          description: Accepted for XUI-convention compatibility; stored but NOT enforced in sing-box — quota enforcement is the caller's responsibility
        downloadLimit:
          type: integer
          description: Accepted for XUI-convention compatibility; stored but NOT enforced in sing-box — quota enforcement is the caller's responsibility

    UserCreateResponse:
      type: object
      required:
        - subId
        - link
      properties:
        subId:
          type: string
          description: Subscription ID
        link:
          type: string
          description: Generated connection link

    TrafficStats:
      type: object
      required:
        - inbounds
        - counters_reset_at
      properties:
        inbounds:
          type: array
          items:
            type: object
            required:
              - tag
              - upload
              - download
            properties:
              tag:
                type: string
                description: Inbound tag
              upload:
                type: integer
                description: Cumulative upload bytes
              download:
                type: integer
                description: Cumulative download bytes
        counters_reset_at:
          type: string
          format: date-time
          description: Last counter reset timestamp

    OnlineUser:
      type: object
      required:
        - subId
        - inboundTag
        - connected_at
      properties:
        subId:
          type: string
          description: Subscription ID
        inboundTag:
          type: string
          description: Inbound tag
        connected_at:
          type: string
          format: date-time
          description: Connection timestamp
        remote_addr:
          type: string
          description: Client IP address

    DesiredState:
      type: object
      required:
        - version
        - inbounds
      properties:
        version:
          type: integer
          description: Monotonically increasing version number
        inbounds:
          type: array
          items:
            $ref: '#/components/schemas/Inbound'

    SyncResult:
      type: object
      required:
        - success
        - version
      properties:
        success:
          type: boolean
        version:
          type: integer
          description: Applied version
        inbounds_updated:
          type: integer
          description: Number of inbounds updated
        users_added:
          type: integer
          description: Number of users added
        users_removed:
          type: integer
          description: Number of users removed

    SyncStatus:
      type: object
      required:
        - state
        - version
      properties:
        state:
          type: string
          enum: [synced, syncing, error]
          description: Current sync state
        version:
          type: integer
          description: Current version
        last_sync_at:
          type: string
          format: date-time
          description: Last successful sync timestamp
        error:
          type: string
          description: Last error message (if state is error)

    SubscriptionConfig:
      type: object
      required:
        - config
        - link
      properties:
        config:
          type: object
          description: Client configuration object
        link:
          type: string
          description: Connection link (e.g., vless://...)

    Subscription:
      type: object
      required:
        - subId
        - configs
      properties:
        subId:
          type: string
          description: Subscription ID
        configs:
          type: array
          items:
            type: object
            description: Available configurations for different protocols
        updated_at:
          type: string
          format: date-time
          description: Last update timestamp

  parameters:
    IdempotencyKey:
      name: Idempotency-Key
      in: header
      description: |
        Idempotency key for safe retries

        **Behavior:**
        - Same key + same payload: Return original response (replay)
        - Same key + different payload: Return 409 IDEMPOTENCY_CONFLICT
        - New key: Normal processing, cached for 24h

        Keys are per-server scope, stored in LRU cache (24h TTL, 10k max entries)
      required: false
      schema:
        type: string
      example: device-123-create-user
```

## 10. Environment Variables Reference
| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SINGBOX_AGENT_API_PORT` | No | `8080` | API listen port |
| `SINGBOX_AGENT_METRICS_PORT` | No | `9090` | Prometheus metrics port |
| `SINGBOX_AGENT_TOKEN` | Yes | — | Bearer auth token (min 32 chars) |
| `SINGBOX_AGENT_SECRET` | Yes | — | HMAC signing secret |
| `SINGBOX_AGENT_SINGBOX_CONFIG_PATH` | No | `/etc/sing-box/config.json` | sing-box config file path |
| `SINGBOX_AGENT_LOG_LEVEL` | No | `info` | Log level (debug, info, warn, error) |
| `SINGBOX_AGENT_TLS_CERT_PATH` | No | — | TLS certificate path for API |
| `SINGBOX_AGENT_TLS_KEY_PATH` | No | — | TLS key path for API |
| `SINGBOX_AGENT_FASTIFY_URL` | No | — | Central control-plane base URL (optional) |
| `SINGBOX_AGENT_SERVER_ID` | Conditional | — | Server ID (required when `FASTIFY_URL` set) |
| `SINGBOX_AGENT_RELOAD_STRATEGY` | No | `systemctl` | Reload strategy: `systemctl` \| `signal` \| `command` |
| `SINGBOX_AGENT_RELOAD_TARGET` | No | `sing-box` | Systemd unit name / PID file path |
| `SINGBOX_AGENT_RELOAD_COMMAND` | No | — | Shell command when strategy=`command` |
