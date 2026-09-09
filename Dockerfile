# syntax=docker/dockerfile:1.7
#
# Multi-stage build: compiles the agent in the golang image, then copies the
# static binary into a minimal Alpine runtime that also ships sing-box
# (downloaded from the official GitHub release).
#
# The container runs sing-box and the agent side-by-side; the agent reloads
# sing-box via SIGHUP (strategy=signal) so no systemd is required.
#
# LICENSING NOTE (IMPORTANT)
# --------------------------
# The sing-box-agent source in this repository is MIT licensed, but the
# *compiled* agent binary produced by this Dockerfile links sing-box as a
# Go library. sing-box is GPLv3. The resulting image (binary + bundled
# sing-box executable) is therefore a GPLv3 derived work.
#
# This Dockerfile is provided so operators can build and run the image for
# their own use ("mere use" is unrestricted under GPLv3). Do NOT publish
# the resulting image to a public registry under the MIT license — if you
# redistribute the image you must comply with GPLv3 (provide corresponding
# source, etc.) or link only against a non-GPL core.
#
# For these reasons this project intentionally does NOT ship a pre-built
# image on Docker Hub / ghcr.io.

ARG GO_VERSION=1.25
ARG SINGBOX_VERSION=1.12.0

# ----------------------------------------------------------------------------
# Build stage (Go agent)
# ----------------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build

WORKDIR /src

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOFLAGS=-trimpath \
    go build \
        -ldflags "-s -w -buildid= \
            -X main.Version=${VERSION} \
            -X main.GitCommit=${GIT_COMMIT} \
            -X main.BuildTime=${BUILD_TIME}" \
        -o /out/sing-box-agent ./cmd/agent

# ----------------------------------------------------------------------------
# Download sing-box release binary for the target platform
# ----------------------------------------------------------------------------
FROM alpine:3.20 AS singbox-download

ARG TARGETARCH
ARG SINGBOX_VERSION

RUN apk add --no-cache curl tar

RUN set -eux; \
    case "${TARGETARCH}" in \
        amd64)  SB_ARCH=amd64 ;; \
        arm64)  SB_ARCH=arm64 ;; \
        arm)    SB_ARCH=armv7 ;; \
        *)      echo "unsupported arch: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    url="https://github.com/SagerNet/sing-box/releases/download/v${SINGBOX_VERSION}/sing-box-${SINGBOX_VERSION}-linux-${SB_ARCH}.tar.gz"; \
    curl -fsSL "$url" -o /tmp/sing-box.tgz; \
    mkdir -p /out; \
    tar -xzf /tmp/sing-box.tgz -C /tmp; \
    install -m 0755 /tmp/sing-box-*/sing-box /out/sing-box

# ----------------------------------------------------------------------------
# Runtime stage
# ----------------------------------------------------------------------------
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tini wget

ENV SINGBOX_AGENT_SINGBOX_CONFIG_PATH=/etc/sing-box/config.json \
    SINGBOX_AGENT_RELOAD_STRATEGY=signal \
    SINGBOX_AGENT_RELOAD_TARGET=/run/sing-box.pid \
    SINGBOX_AGENT_API_PORT=8080 \
    SINGBOX_AGENT_METRICS_PORT=9090

COPY --from=singbox-download /out/sing-box /usr/local/bin/sing-box
COPY --from=build /out/sing-box-agent /usr/local/bin/sing-box-agent
COPY deploy/docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh /usr/local/bin/sing-box /usr/local/bin/sing-box-agent \
    && mkdir -p /etc/sing-box /etc/sing-box-agent /var/lib/sing-box-agent /run

EXPOSE 8080 9090

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://127.0.0.1:${SINGBOX_AGENT_API_PORT:-8080}/healthz || exit 1

ENTRYPOINT ["/sbin/tini", "--", "/entrypoint.sh"]
