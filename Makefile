PHONY := help lint test build test-race test-coverage security release version tools-install
.PHONY: $(PHONY)
GOLANGCI_LINT_VERSION := v2.13.2
STATICCHECK_VERSION := 2025.1.1
GOFUMPT_VERSION := v0.7.0

TOOLS_DIR := .tools


DEFAULT_GOFLAGS=-trimpath
VERSION ?= dev
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GIT_COMMIT ?= $(shell git rev-parse --short=12 HEAD)
BUILD_LDFLAGS=-s -w -buildid= -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)

help:
	@echo "Available targets:"
	@echo "  help      - Show this help"
	@echo "  lint      - Run full linting pipeline (gofumpt, go vet, staticcheck, golangci-lint)"
	@echo "  test      - Run go test ./..."
	@echo "  test-race - Run go test with race detector"
	@echo "  test-coverage - Run tests with coverage threshold (>=70%)"
	@echo "  build     - Build all packages (deterministic flags)"
	@echo "  version   - Print build version metadata"
	@echo "  security  - Run security checks (govulncheck)"
	@echo "  release   - Create local multi-platform release artifacts"

lint: ## Run full linting pipeline (gofumpt, go vet, staticcheck, golangci-lint)
	@echo "Running linter pipeline..."
	@# 1) gofumpt (format check)
	@if command -v gofumpt >/dev/null 2>&1; then \
		files=$$(gofumpt -l .); \
		if [ -n "$$files" ]; then \
			echo "Files need formatting (run 'gofumpt -w .' to fix):"; \
			echo "$$files"; \
			exit 1; \
		fi; \
	else \
		echo "gofumpt not found — install via 'go install mvdan.cc/gofumpt@${GOFUMPT_VERSION}'"; exit 2; \
	fi

	@# 2) go vet
	@go vet ./...

	@# 3) staticcheck
	@if command -v staticcheck >/dev/null 2>&1; then \
		staticcheck ./... ; \
	else \
		echo "staticcheck not found — install via 'go install honnef.co/go/tools/cmd/staticcheck@${STATICCHECK_VERSION}'"; exit 2; \
	fi

	@# 4) golangci-lint
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --config .golangci.yml ; \
	else \
		echo "golangci-lint not found — install via 'curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/${GOLANGCI_LINT_VERSION}/install.sh | sh -s ${GOLANGCI_LINT_VERSION}'"; exit 2; \
	fi

tools-install: ## Install pinned tooling locally
	@echo "Installing lint tools (pinned versions) into ".tools/bin"
	@mkdir -p ${TOOLS_DIR}/bin
	@GOBIN=$(PWD)/${TOOLS_DIR}/bin go install mvdan.cc/gofumpt@${GOFUMPT_VERSION}
	@GOBIN=$(PWD)/${TOOLS_DIR}/bin go install honnef.co/go/tools/cmd/staticcheck@${STATICCHECK_VERSION}
	@# Install golangci-lint via official install script pinned to version
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/${GOLANGCI_LINT_VERSION}/install.sh | sh -s ${GOLANGCI_LINT_VERSION}


test:
	@echo "Running tests..."
	go test ./...

test-race: ## Run tests with race detector
	@echo "Running tests with race detector..."
	go test -race ./...

test-coverage: ## Run tests with coverage threshold (>=70%)
	@echo "Running tests with coverage..."
	go test -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | grep total | awk '{if ($$3+0 < 70) {print "Coverage below 70%: " $$3; exit 1}}'

build:
	@echo "Building sing-box-agent..."
	GOFLAGS="$(DEFAULT_GOFLAGS)" go build -ldflags "$(BUILD_LDFLAGS)" -o sing-box-agent ./cmd/agent

version:
	@echo "Version: $(VERSION)"
	@echo "BuildTime: $(BUILD_TIME)"
	@echo "GitCommit: $(GIT_COMMIT)"

security: ## Run security checks (govulncheck)
	@echo "Running security checks..."
	govulncheck ./...

release:
	@echo "Building local release artifacts..."
	@mkdir -p dist
	@set -euo pipefail; \
	for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do \
		goos=$${target%/*}; \
		goarch=$${target#*/}; \
		artifact="sing-box-agent-$${goos}-$${goarch}"; \
		echo "Building $${artifact}"; \
		CGO_ENABLED=0 GOOS=$${goos} GOARCH=$${goarch} GOFLAGS="$(DEFAULT_GOFLAGS)" \
			go build -buildvcs=false -ldflags "$(BUILD_LDFLAGS)" -o "dist/$${artifact}" ./cmd/agent; \
		tar -C dist -czf "dist/$${artifact}.tar.gz" "$${artifact}"; \
		rm "dist/$${artifact}"; \
		sha256sum "dist/$${artifact}.tar.gz" > "dist/$${artifact}.sha256"; \
	done; \
	cat dist/*.sha256 | sort > dist/checksums.txt; \
	echo "Release artifacts generated in dist/"
