# Roadmap

vfx is built incrementally. Each phase delivers a usable subset of the final system.

## Phase 1 — Realtime MVP

Goal: prove that WebTransport + WASM + Agones works as a usable game server foundation, with a single end-to-end sample.

### Tooling & Project Skeleton

- [x] `mise.toml` with CLI versions pinned (Go, buf, sqlc, atlas, helm, kubectl, kind, golangci-lint, gofumpt). TinyGo to be re-added when plugin SDK work starts.
- [x] `compose.yml` for PostgreSQL and Valkey.
- [x] `Dockerfile` for the `vfx` binary (multi-stage, distroless, non-root, 40MB).
- [x] `deploy/local/kind-config.yaml` and `deploy/local/values.yaml`.
- [x] `deploy/helm/vfx/` with default Chart, configurable for external DB / Valkey / TLS secrets.

### Schema

- [x] `buf.yaml` / `buf.gen.yaml` for Go and TypeScript code generation (managed mode, go_package_prefix).
- [x] `schema/api/vfx/v1/auth/auth_service.proto`
- [x] `schema/api/vfx/v1/match/match_service.proto`
- [x] `schema/api/vfx/v1/realtime/frame.proto`
- [x] `schema/api/plugin/v1/plugin.proto` (host ⇔ guest ABI)
- [x] `schema/db/schema.sql` (PostgreSQL declarative schema).
- [x] `atlas.hcl` and `schema/db/migrations/` (atlas-managed).
- [x] `sqlc.yaml` and `schema/db/queries/` for type-safe SQL bindings.

### Application

- [x] `cmd/vfx`: Cobra-based CLI dispatching to subcommands.
- [x] `vfx gateway`: Connect RPC server, auth, matchmaking worker.
  - [x] AuthService (Login/Refresh/Logout/UpdateProfile) end-to-end with anonymous credential
  - [x] MatchService (CreateTicket/WatchTicket/CancelTicket; GetCurrentMatch returns nil until assignment store lands)
  - [x] Matchmaker worker (in-memory queue, pair-up policy, stub allocator)
  - [ ] Valkey-backed queue (multi-gateway)
  - [ ] Tier-based matching (rating / region relaxation)
  - [ ] Agones Allocator integration
- [x] `vfx room`: WebTransport server, plugin host (Go-native), tick loop, match orchestrator. Agones SDK and wazero deferred to Phase 2.
- [ ] `vfx admin`: minimal HTTP API (web UI deferred to later phase).
- [x] `vfx migrate`: thin wrapper around atlas (apply / status / down).

### SDK

- [ ] `sdk/plugin/go`: Plugin SDK targeting TinyGo → WASM (Phase 2, needs the wazero loader).
- [x] `sdk/client/go`: Client SDK (auth + match + WebTransport); rps-cli is built on it.
- [x] `sdk/client/ts`: TypeScript client SDK (connect-web + browser WebTransport), in-repo, not published.

### Example

- [x] `examples/rps/`: Rock-paper-scissors (best of 3) as the canonical Phase 1 sample.
  - [x] Plugin (Go-native): round resolution, best-of-3 state, end-to-end verified.
  - [x] CLI client (Go): built on the Go SDK.
  - [x] Web client (TypeScript): built on the TS SDK, Vite dev server.
  - [ ] TinyGo build of the same plugin → WASM (Phase 2 with wazero).
  - [ ] Helm values overlay (needs a vfx-rps image build).

### Observability

- [ ] OpenTelemetry traces from `gateway` and `room` (W3C Trace Context propagated end-to-end).
- [x] Prometheus-compatible `/metrics` endpoint (gateway today; room daemon HTTP/1.1 sidecar pending).
- [x] Health (`/healthz`) and readiness (`/readyz` with PostgreSQL ping) probes on the gateway.

### CI / Release

- [x] `.github/workflows/ci.yml` runs generate / lint / test / build on every PR and push to main.
- [ ] `.github/workflows/release.yml` builds and pushes the image and chart on tagged releases.

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
