# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Idempotency middleware wired into all mutating protected endpoints
  (no-op for requests without an `Idempotency-Key` header).
- `stats_api_address` configuration option — v2ray_api address used for
  traffic statistics.
- Initial public release preparation.
- Optional HTTP Basic auth for the `/metrics` endpoint via `metrics_username`
  and `metrics_password` (active only when both are set).
- `cmd/statsprobe` — debug utility that queries v2ray_api `QueryStats` and
  prints raw traffic counters.

### Security
- Constant-time bearer-token comparison.
- Nonce is cached only after signature verification, so a request with an
  invalid signature no longer burns a replay-window entry.
- Request body size limited to 10 MB.
- sing-box config is written atomically with `0600` permissions, guarded by
  a process-local lock between concurrent writers.
- Eliminated a rollback race in desired-state reconciliation.

### Fixed
- Documentation no longer implies that `limitIp`/`uploadLimit`/
  `downloadLimit` are enforced: they are accepted for XUI-convention
  compatibility and stored, but sing-box has no native per-user limits;
  quota enforcement stays with the calling service.

### Changed
- Dropped the sing-box Go-library dependency: the agent binary no longer
  links sing-box and is MIT-only. sing-box is managed as a separate process
  (systemd / signal / command).
- `/core/reload` respects the configured `reload_strategy`.
- Metrics are additionally served on a dedicated listener (`metrics_port`,
  default `9090`); `/metrics` remains available on the API port.
- Logs are structured JSON.
- Consistent handling of user `email` and `enabled` fields.
- Subscription cache is bounded.

### Removed
- Dead code: `internal/db` (SQLite/Postgres persistence and migrations),
  validators, shared testutil helpers, and legacy mocks.

### Notes
- The Docker image bundles the GPLv3 sing-box executable next to the
  MIT-licensed agent (aggregation, not linking); see the README "License"
  section.

## [0.1.0] — TBD

### Added
- REST API for managing sing-box inbounds and users at runtime.
- HMAC-SHA256 request signing with replay protection (timestamp + nonce cache).
- Bearer-token authentication.
- Desired-state reconciliation engine with monotonic sync versions.
- Prometheus metrics (`/metrics`) and health endpoints (`/healthz`, `/readyz`, `/status`).
- Subscription-link generation (`v2ray`, `clash`, `sing-box` formats).
- Pluggable reload strategies: `systemctl`, `signal`, `command`.
- Example `docker-compose.yml` for local development.
- Example systemd deployment scripts under `deploy/example/`.
- Example VLESS + Reality inbound config under `deploy/example/sing-box-vless-reality.json`.
- Russian-language README (`README.ru.md`).
- GitHub issue and pull request templates.

### Notes
- Source and agent binaries are MIT — the agent does not link sing-box.
  The Docker image bundles the GPLv3 sing-box executable (aggregation);
  see README "License".

[Unreleased]: https://github.com/oglenyaboss/sing-box-agent/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/oglenyaboss/sing-box-agent/releases/tag/v0.1.0
