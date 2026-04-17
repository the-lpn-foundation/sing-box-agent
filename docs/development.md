# Development Guide

A short tour of the codebase and the commands you need to move around it
quickly. For contribution policy see [CONTRIBUTING.md](../CONTRIBUTING.md);
for the HTTP contract see [integration-guide.md](./integration-guide.md)
and [openapi.yaml](./openapi.yaml).

## Repository layout

```
cmd/agent/               Main binary (main.go wires config → server)
internal/
  auth/                  Bearer-token and HMAC-SHA256 signing primitives
  client/                HTTP client for an upstream control plane
  config/                YAML + env loader, validation
  db/                    Optional local persistence (SQLite/Postgres)
  handlers/              HTTP handlers for each API group
  middleware/            Auth + idempotency middleware
  models/                Domain types (Inbound, User, DesiredState, …)
  server/                HTTP server bootstrap, lifecycle manager
  singbox/               Wrapper around sing-box (Config, CoreServiceAdapter)
  sync/                  Desired-state diff + reconciler + reload strategies
  store/                 Idempotency cache, request deduplication
  validators/            JSON-schema validation helpers
  metrics/               Prometheus collectors
  testutil/              Shared test helpers

deploy/
  example/               Systemd deploy script + example configs
  docker/                Dockerfile support (entrypoint, compose)

docs/                    Specs, integration guide, OpenAPI, dev docs
migrations/              SQL migrations (local persistence)
test/                    Cross-package integration / e2e tests
scripts/                 Release notes, hook check, misc tooling
```

## Toolchain

Install the pinned linting / formatting tools into `.tools/bin`:

```bash
make tools-install
```

This installs:

- `gofumpt` (stricter gofmt) — pre-commit + CI both run it.
- `staticcheck` — honnef.co static analysis.
- `golangci-lint` — aggregate linter (config in `.golangci.yml`).

`go vet` and `govulncheck` are invoked directly from `make` targets.

## Common tasks

```bash
make build                           # reproducible local binary
make lint                            # gofumpt + vet + staticcheck + golangci-lint
make test                            # all packages, no race
make test-race                       # with -race
make test-coverage                   # with 70% threshold
make security                        # govulncheck
make release                         # multi-platform tarballs in dist/
```

Run a subset:

```bash
go test ./internal/sync/...          # one tree
go test -run TestConfigManager -v ./internal/sync/
go test -race ./internal/handlers/...
```

## Running locally

```bash
go run ./cmd/agent -config deploy/example/agent-config.yaml
# On another terminal:
curl http://127.0.0.1:8080/healthz
```

Make sure the example config has real token/secret (`openssl rand -hex 32`)
and that `singbox_config_path` points at a valid sing-box config.

## Running inside Docker

```bash
docker build -t sing-box-agent:dev .
docker run --rm -p 8080:8080 \
  -v "$PWD/deploy/example/sing-box-config.json:/etc/sing-box/config.json:ro" \
  -e SINGBOX_AGENT_TOKEN="$(openssl rand -hex 32)" \
  -e SINGBOX_AGENT_SECRET="$(openssl rand -hex 32)" \
  sing-box-agent:dev
```

The image bundles sing-box; the entrypoint starts it in the background
and wires the agent to reload it via `SIGHUP` using the PID file at
`/run/sing-box.pid`.

## Debugging the sync engine

The reconciler compares the current sing-box config to the desired state
pushed via `POST /sync/desired-state` and applies the diff through
`ConfigManager` → `Reloader`. Interesting files:

- `internal/sync/engine.go` — main reconciler, version checks.
- `internal/sync/diff.go` — add/update/delete plan computation.
- `internal/sync/config_manager.go` — read/mutate sing-box config on disk.
- `internal/sync/reloader.go` — systemctl / signal / command strategies.

To trace a failed apply, bump `SINGBOX_AGENT_LOG_LEVEL=debug` and look
for `msg="applying plan"` / `msg="reload failed"` entries.

## Adding a new API endpoint

1. Define request/response types in `internal/handlers/<area>.go`.
2. Implement the handler — return `handlers.Response` envelopes, never
   write directly to `http.ResponseWriter` without the helper.
3. Register the route in `internal/server/server.go` under the correct
   mux block (public vs auth-required).
4. Update `docs/openapi.yaml` and `docs/integration-guide.md`.
5. Add unit tests in `internal/handlers/<area>_test.go` plus an
   integration test under `test/integration/` or `test/e2e/`.

## Adding a new reload strategy

1. Implement the `Reloader` interface in `internal/sync/reloader.go`.
2. Wire a constructor branch in `buildReloader()` inside
   `internal/server/server.go`.
3. Extend `internal/config/config.go` with any new fields (keep the env
   tag style consistent).
4. Document the strategy in [deployment.md](./deployment.md).

## Writing tests

- Prefer table-driven tests.
- Use `httptest.NewServer` for HTTP-level tests instead of spinning up
  the real `server.Server`.
- Mock the `singbox.SingBox` / `syncpkg.Reloader` interfaces — never call
  real `systemctl` in unit tests.
- Load deterministic fixtures from `test/fixtures/…`.
- Parallelise with `t.Parallel()` unless a test touches shared state.

## Release checklist (maintainers)

1. Update `CHANGELOG.md` and move entries from `Unreleased` to the new
   version heading.
2. Ensure `make lint test-coverage security build` is green.
3. Tag: `git tag -a vX.Y.Z -m "vX.Y.Z" && git push origin vX.Y.Z`.
4. Create a GitHub Release attached to the tag — the `release.yml`
   workflow builds multi-arch artefacts, attaches SBOM + checksums,
   signs with cosign, and uploads SLSA provenance.
