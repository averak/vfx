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
  - [x] MatchService (CreateTicket/WatchTicket/CancelTicket; GetCurrentMatch served from the Valkey assignment store for reconnect / multi-replica recovery)
  - [x] Matchmaker worker (in-memory queue, pair-up policy; stub allocator for local, Agones allocator for clusters; assignments persisted to Valkey)
  - [x] Agones Allocator integration (GameServerAllocation via the in-cluster API, selected with VFX_ALLOCATOR=agones). Verified on kind: matchmaking allocates a Ready GameServer and hands its address:port to clients.
  - [x] Valkey-backed queue (multi-gateway): tickets in a per-mode sorted set, events over pub/sub, atomic Lua Claim so concurrent matchmakers never double-match. Selected with VFX_MATCH_QUEUE=valkey; in-memory remains the default.
  - [x] Tier-based matching: rating-window pairing that widens with wait time, region enforced until a relaxation deadline. Tickets without rating/region skip the respective check (so the rps sample still pairs instantly).
- [x] `vfx room`: WebTransport server, tick loop, match orchestrator, and both plugin hosts — Go-native (registry) and WASM (wazero). Agones game-server SDK (Ready/Health/Shutdown) wired in, gated by VFX_ROOM_AGONES_ENABLED.
- [x] `vfx admin`: minimal read-only HTTP/JSON ops API (player lookup, queue depth, health probes), deployable via the chart's optional admin tier. Web UI deferred to a later phase.
- [x] `vfx migrate`: thin wrapper around atlas (apply / status / down).

### SDK

- [x] `sdk/plugin/go`: guest SDK targeting TinyGo → WASM, loaded by the room's wazero host. Uses vtprotobuf for a reflection-free codec.
- [x] `sdk/client/go`: Client SDK (auth + match + WebTransport); rps-cli is built on it.
- [x] `sdk/client/ts`: TypeScript client SDK (connect-web + browser WebTransport), in-repo, not published.

### Example

- [x] `examples/rps/`: Rock-paper-scissors (best of 3) as the canonical Phase 1 sample.
  - [x] Plugin (Go-native): round resolution, best-of-3 state, end-to-end verified.
  - [x] CLI client (Go): built on the Go SDK.
  - [x] Web client (TypeScript): built on the TS SDK, Vite dev server.
  - [x] TinyGo build of the same plugin → WASM, run in the room's wazero sandbox.
  - [x] Helm values overlay + image: examples/rps/Dockerfile bakes rps.wasm onto the vfx image; deploy/local/values-agones.yaml runs it as an Agones Fleet on kind.

### Observability

- [x] OpenTelemetry traces from `gateway` (Connect interceptor) and `room` (session and match-create spans). Opt-in via `OTEL_EXPORTER_OTLP_ENDPOINT`; off by default so single-node deployments pay nothing.
- [x] Prometheus-compatible `/metrics` endpoint on both the gateway and the room daemon (the room exposes a plain-HTTP listener alongside its UDP WebTransport tier; active-match and tick-duration metrics wired at their call sites).
- [x] Health (`/healthz`) and readiness (`/readyz` with PostgreSQL ping) probes on the gateway.

### CI / Release

- [x] `.github/workflows/ci.yml` runs generate / lint / test / build on every PR and push to main.
- [x] `.github/workflows/release.yml` builds and pushes a multi-arch image and the Helm chart (OCI) to GHCR on `v*` tags, then cuts a GitHub release.

## Phase 2 — General-purpose backend features

Goal: feature breadth comparable to mature game backends.

- [x] OIDC providers: Google, Apple (ID-token verification via JWKS), with account linking to upgrade an anonymous player; anonymous device already shipped.
- [x] Storage API — player data (owner-scoped) and title storage (operator-published, tag-gated); bytes in object storage via signed URLs, metadata in PostgreSQL.
- [x] Leaderboard — config-defined boards (asc/desc), keep-best scores, ranked/around-player queries.
- [x] Friends — request lifecycle (send/accept/decline/cancel, mutual auto-accept), friend list, incoming/outgoing, remove. (Groups, parties, and blocking still pending.)
- [x] Chat — direct messages (send + paginated, newest-first history). (Group channels and realtime subscribe still pending.)
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
