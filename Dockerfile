# syntax=docker/dockerfile:1.7

# ============================================================================
# atlas: copy the migration CLI that `vfx migrate` shells out to.
# Pin matches mise.toml so local + container versions agree.
# ============================================================================
FROM arigaio/atlas:1.2.0-community-distroless AS atlas

# ============================================================================
# builder: compile vfx as a static binary.
# ============================================================================
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=0.0.0-dev

WORKDIR /src

# Cache module downloads independently of source changes.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Bring in the rest of the tree and build.
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build \
      -trimpath \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/vfx \
      ./cmd/vfx

# ============================================================================
# runtime: distroless static, non-root, no shell.
# ============================================================================
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=atlas /atlas /usr/local/bin/atlas
COPY --from=builder /out/vfx /usr/local/bin/vfx
COPY --from=builder /src/schema/db/migrations /etc/vfx/migrations

# Default ports (override with --port-style flags when needed):
#   8080/tcp  Connect RPC (gateway, admin)
#   8443/udp  WebTransport / HTTP3 (room)
EXPOSE 8080/tcp
EXPOSE 8443/udp

USER nonroot:nonroot

# OCI labels for downstream tooling.
LABEL org.opencontainers.image.title="vfx" \
      org.opencontainers.image.description="WebTransport-native, WASM-driven game server engine" \
      org.opencontainers.image.source="https://github.com/averak/vfx" \
      org.opencontainers.image.licenses="MIT"

ENTRYPOINT ["/usr/local/bin/vfx"]
