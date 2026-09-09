# Contributing to sing-box-agent

Thanks for your interest in improving the project! This document covers the
workflow for contributing code, tests, documentation, and bug reports.

## Getting Started

### Prerequisites

- **Go** matching the version in [`go.mod`](./go.mod) (currently 1.24.x).
- **Make**.
- **sing-box** ≥ 1.12 (only required for live integration testing).
- **Docker** (optional, used for containerised development and E2E runs).
- **pre-commit** (`pip install pre-commit` or `brew install pre-commit`).

### Clone and bootstrap

```bash
git clone https://github.com/<your-fork>/sing-box-agent.git
cd sing-box-agent

# Install pinned tools (golangci-lint, staticcheck, gofumpt) under .tools/bin
make tools-install

# Install pre-commit hooks (gofumpt, go vet, golangci-lint, gitleaks)
pre-commit install
scripts/hook-check.sh
```

### Running locally

```bash
# Build the agent
make build

# Copy an example config and edit token/secret
cp deploy/example/agent-config.yaml /tmp/agent-config.yaml
$EDITOR /tmp/agent-config.yaml

# Run it
./sing-box-agent -config /tmp/agent-config.yaml
```

The agent will listen on `:8080` (API) and `:9090` (metrics) by default.

## Development Workflow

1. **Fork and branch.** Create a feature branch from `main`:
   ```bash
   git checkout -b feat/short-description
   ```
2. **Make changes.** Keep commits focused; the repository follows the
   [Conventional Commits](https://www.conventionalcommits.org/) style, e.g.:
   ```
   feat: add Hysteria2 subscription format
   fix: resolve race in sync engine reload
   docs: clarify HMAC signing in integration guide
   ```
3. **Run the full local pipeline before pushing:**
   ```bash
   make lint          # gofumpt + vet + staticcheck + golangci-lint
   make test          # unit tests
   make test-race     # race detector
   make test-coverage # ensures >= 70% coverage
   make security      # govulncheck
   make build         # reproducible build
   ```
4. **Open a Pull Request.** Include:
   - What changed and why.
   - How it was tested (commands, screenshots, logs).
   - Any follow-up work or known limitations.

CI (GitHub Actions) runs the same pipeline on every PR. Merges require a
green CI run.

## Code Style

- **Formatting:** [`gofumpt`](https://github.com/mvdan/gofumpt) (stricter
  `gofmt`). The pre-commit hook blocks unformatted files.
- **Imports:** grouped `std → third-party → internal` (`goimports` with
  local prefix `github.com/oglenyaboss/sing-box-agent`).
- **Linters:** `golangci-lint` (config in [`.golangci.yml`](./.golangci.yml)).
- **Error handling:** always handle or deliberately ignore errors; do not
  swallow them silently. Panics are forbidden in request handlers.
- **Comments:** document exported types and functions. Skip obvious
  inline comments — prefer clear names and short functions.

## Testing

- New logic needs unit tests alongside the package (`*_test.go`).
- End-to-end behaviour lives in `test/e2e/` and uses `httptest` servers;
  they should skip automatically when the `:8080` agent is reachable.
- Each test package defines its own local mocks implementing the `sync.SingBoxClient` interface (see `internal/sync/engine.go`).

Run a single package:

```bash
go test ./internal/sync/... -run TestConfigManager -v
```

Run the full suite with coverage:

```bash
make test-coverage
go tool cover -html=coverage.out
```

## Releasing (maintainers)

1. Update [`CHANGELOG.md`](./CHANGELOG.md) with the next version's entries.
2. Tag: `git tag -a vX.Y.Z -m "vX.Y.Z" && git push origin vX.Y.Z`.
3. Create a GitHub Release pointing at the tag — the `release.yml` workflow
   will build multi-arch artefacts, generate SBOM, sign with cosign, and
   attach assets + SLSA provenance automatically.

## Reporting Issues

For bugs, please include:

- Version (`sing-box-agent -version`) and Go version.
- Minimal reproduction (config snippet, curl command, expected vs actual).
- Relevant log lines (redact tokens/secrets!).

For security issues, see [`SECURITY.md`](./SECURITY.md) and do **not** file
a public issue.

## Code of Conduct

Be respectful, assume good intent, and focus on the technical substance.
Harassment or discrimination of any kind is not tolerated.

## License

By contributing, you agree that your contributions will be licensed under
the [MIT License](./LICENSE).
