# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial public release preparation.

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
- Source is MIT; compiled binaries and Docker images are not distributed
  from this repository because sing-box is GPLv3 (see README "License").

[Unreleased]: https://github.com/oglenyaboss/sing-box-agent/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/oglenyaboss/sing-box-agent/releases/tag/v0.1.0
