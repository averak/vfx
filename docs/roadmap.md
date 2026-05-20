# Roadmap

vfx is built incrementally. Each phase delivers a usable subset of the final system.

## Phase 1 — Realtime MVP

Goal: prove that WebTransport + WASM + Agones works as a usable game server foundation, with a single end-to-end sample.

### Tooling & Project Skeleton

- [ ] `mise.toml` with all CLI versions pinned (Go, buf, sqlc, atlas, TinyGo, helm, kubectl, kind, golangci-lint).
- [ ] `compose.yml` for PostgreSQL and Valkey.
- [ ] `Dockerfile` for the `vfx` binary.
- [ ] `deploy/local/kind-config.yaml` and `deploy/local/values.yaml`.
- [ ] `deploy/helm/vfx/` with default Chart, configurable for in-cluster or external DB.

### Schema

- [ ] `buf.yaml` / `buf.gen.yaml` for Go and TypeScript code generation.
- [ ] `schema/api/vfx/v1/auth/auth_service.proto`
- [ ] `schema/api/vfx/v1/match/match_service.proto`
- [ ] `schema/api/vfx/v1/realtime/frame.proto`
- [ ] `schema/api/plugin/v1/plugin.proto` (host ⇔ guest ABI)
- [ ] `schema/db/schema.sql` (PostgreSQL declarative schema).
- [ ] `atlas.hcl` and `schema/db/migrations/` (atlas-managed).

### Application

- [ ] `cmd/vfx`: Cobra-based CLI dispatching to subcommands.
- [ ] `vfx gateway`: Connect RPC server, auth, matchmaking worker, Agones Allocator client.
- [ ] `vfx room`: Agones SDK integration, WebTransport server, wazero plugin host, tick loop.
- [ ] `vfx admin`: minimal HTTP API (web UI deferred to later phase).
- [ ] `vfx migrate`: thin wrapper around atlas.

### SDK

- [ ] `sdk/plugin/go`: Plugin SDK targeting TinyGo → WASM.
- [ ] `sdk/client/go`: Client SDK used by integration tests and CLI tools.
- [ ] `sdk/client/ts`: TypeScript client, in-repo, not yet published to any registry.

### Example

- [ ] `examples/1_turn_based/`: A complete sample game (plugin + web client + Helm values) demonstrating the full stack.

### Observability

- [ ] OpenTelemetry traces and metrics from `gateway` and `room`.
- [ ] Prometheus-compatible `/metrics` endpoint.
- [ ] Health/readiness probes.

## Phase 2 — General-purpose backend features

Goal: feature breadth comparable to mature game backends.

- [ ] OAuth providers: Google, Apple, GitHub, anonymous device.
- [ ] Storage API (KV with per-record permissions).
- [ ] Leaderboard.
- [ ] Friends / Groups / Parties.
- [ ] Chat (channels, DMs).
- [ ] Admin web UI, embedded via `go:embed`.
- [ ] C# (Unity) client SDK.
- [ ] C# / Rust plugin SDKs.
- [ ] Hot-reload of WASM plugins in development.
- [ ] Migration guide for users coming from other backends.

## Phase 3 — Differentiation

Goal: capabilities that set vfx apart, enabled by the architecture choices in Phase 1.

- [ ] Replay and spectator support.
- [ ] A/B testing of plugin versions.
- [ ] Multi-region deployment guide.
- [ ] Built-in autoscaler for non-Kubernetes environments (optional).
- [ ] Plugin profiling and tracing integration.

## Out of Scope

The following are intentionally excluded from vfx core. Forks or extensions are welcome but unsupported.

- **MMO-style persistent worlds** — vfx's room model is match-based, not zone-based.
- **Multi-tenant SaaS hosting** — vfx is single-tenant by design.
- **Real-money gambling features** — regulatory complexity outweighs the engine's scope.
- **Distributed SQL backends** — PostgreSQL is the only supported storage.
