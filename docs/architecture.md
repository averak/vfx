# Architecture

This document captures the system design of vfx: components, protocols, deployment topology, and the rationale behind the major choices.

## Overview

vfx is a game server engine built around three architectural commitments:

1. **WebTransport (HTTP/3) for realtime**, mixing reliable streams and unreliable datagrams in one connection.
2. **WebAssembly for game logic**, isolating user code from the host runtime and allowing any language that targets WASM.
3. **Kubernetes-native operation** with Agones managing the lifecycle of match-hosting processes.

The engine is delivered as a single binary that runs in different roles depending on the subcommand.

## Design Principles

- **One process per match.** Each in-progress match is hosted by exactly one process (an Agones GameServer Pod). Density is managed by running many Pods, not by multiplexing matches within one process.
- **Stateless gateway, stateful room.** The gateway handles auth, matchmaking, and storage APIs and can scale horizontally behind a load balancer. The room process holds in-memory match state and is addressed individually.
- **Schema-first.** Protocol Buffers are the source of truth for both wire protocols and code generation.
- **Standard infrastructure.** vfx integrates with widely used building blocks (PostgreSQL, Valkey, Agones, Kubernetes) rather than inventing its own.
- **Single-tenant.** vfx is intended to be deployed by an operator running their own game(s). It is not a multi-tenant SaaS platform.

## Components

```mermaid
flowchart TB
    Client([Client])

    subgraph Cluster["Kubernetes cluster"]
        Gateway["vfx gateway<br/>(stateless Deployment)"]
        Admin["vfx admin"]
        subgraph Fleet["Agones Fleet"]
            Room1["vfx room<br/>Pod #1"]
            Room2["vfx room<br/>Pod #2"]
            RoomN["vfx room<br/>Pod #N"]
        end
        Agones["Agones controller"]
    end

    PG[("PostgreSQL<br/>(managed or self-hosted)")]
    VK[("Valkey<br/>(managed or self-hosted)")]

    Client -- "Connect RPC<br/>over HTTP/2" --> Gateway
    Client -- "WebTransport<br/>over HTTP/3" --> Room1

    Gateway --> PG
    Gateway --> VK
    Gateway -- "Allocate" --> Agones
    Agones -- "Ready → Allocated → Shutdown" --> Fleet
    Room1 --> PG
    Room1 --> VK
    Admin --> PG
    Admin --> VK
```

### vfx gateway

- Stateless HTTP server speaking Connect RPC over HTTP/2.
- Surfaces: authentication, matchmaking, storage API, leaderboard, social features, admin RPCs.
- Holds no per-match state; can be scaled horizontally behind any L7 load balancer.
- Runs the matchmaking worker as a goroutine: pulls tickets from Valkey, groups them, and calls the Agones Allocator.

### vfx room

- Hosts exactly one match for its lifetime.
- Listens on UDP for WebTransport (HTTP/3) connections.
- Embeds [`wazero`](https://wazero.io/) and runs the game-specific WASM plugin.
- Integrates the Agones SDK to report `Ready`, `Health`, and `Shutdown`.
- Writes the final match result to PostgreSQL on completion, then exits; Agones replenishes the Fleet.

### vfx admin

- Operational API (and later a web UI) for inspecting rooms, players, and plugin deployments.
- Deployed separately from the gateway, on a different port and behind separate auth.

### vfx migrate

- Wraps `atlas` to apply database migrations.
- Designed to run as a Kubernetes Job during deploy.

## Code Architecture

Within each binary the Go packages follow a Clean Architecture layering. The dependency rule — code may depend only on layers inward of it — is enforced mechanically by `depguard` in `golangci-lint`, not left to convention: the domain may not import infrastructure, and the usecase layer may not import infra, presentation, or any persistence technology.

- **domain** — entities and the rules intrinsic to them (enterprise business rules). `domain/match` owns matchmaking eligibility and tier relaxation (`Ticket.CompatibleWith`, the `Matcher` service); `domain/player` owns the `Player` and its invariants, such as the nickname rule. The domain imports no infrastructure and no persistence type — repository interfaces here take only a `context.Context`.
- **usecase** — application business rules: orchestration that coordinates domain objects but makes no enterprise decisions of its own. Each usecase declares the narrow ports it needs (`Transactor`, `TokenIssuer`, `SessionSigner`), so it depends on capabilities rather than concrete infrastructure.
- **infra** — adapters that implement those ports: PostgreSQL repositories, the Valkey-backed queue and stores, the JWT signer, the Agones allocator, the wazero plugin host.
- **presentation** — the Connect handlers and the WebTransport server, translating between the wire protocol and the usecases.
- **bootstrap** — manual dependency wiring per subcommand; no DI framework.
- **stdx** — small, dependency-free utilities (such as the context clock) usable from any layer.

### Enterprise vs application rules

Each rule is placed by asking whether it is intrinsic to an entity or merely how the application coordinates a flow. Pairing eligibility is intrinsic to a ticket, so it lives in `domain/match`; "claim a group, allocate a room, then notify" is coordination, so it lives in the matchmaker usecase. A valid nickname is part of what a `Player` is, so `player.New` and `SetNickname` enforce it; deciding *when* to rename a player is the auth usecase's concern.

### Transactions

The transaction boundary belongs to the usecase, not the repositories. A usecase calls its `Transactor` port (`RW` or `RO`); the infra implementation opens a pgx transaction and places it on the `context.Context` the work receives, and repositories retrieve it with `db.Tx(ctx)`. Because the transaction rides on the context rather than a method parameter, a single usecase method can open more than one transaction — for example splitting a slim read-write transaction from a separate read-only one — without changing any repository signature.

## Protocols

### Control plane: Connect RPC over HTTP/2

Connect was chosen for the control plane because:

- It is callable directly from browsers without a translation proxy.
- The same Protocol Buffers definitions generate idiomatic clients in Go, TypeScript, and other languages.
- HTTP semantics make it easy to debug with curl.
- Server-side streaming covers matchmaking notifications cleanly.

### Realtime: WebTransport over HTTP/3

WebTransport provides:

- **Reliable bidirectional streams** for important events (match start, score updates, errors).
- **Unreliable datagrams** for high-frequency state updates that tolerate loss.
- **Connection migration** via QUIC, which improves mobile experience.
- **TLS by default**, removing an entire class of misconfiguration.

A WebSocket fallback path for environments without HTTP/3 support is not currently implemented.

### Message envelope

Every realtime message is wrapped in a single envelope so reliable streams and datagrams share one codec:

```protobuf
message Frame {
  uint64 seq = 1;

  oneof body {
    PlayerInput input = 10;
    StateSnapshot snapshot = 20;
    StateDelta delta = 21;
    SystemEvent event = 22;
    ErrorMessage error = 23;
  }
}
```

The game-specific payload inside `PlayerInput` and `StateDelta` is opaque `bytes`, interpreted by the WASM plugin.

## Schema Management

| Layer | Tool |
| --- | --- |
| API (`.proto`) | [`buf`](https://buf.build/) for lint, breaking-change detection, and code generation. |
| Database schema | [`atlas`](https://atlasgo.io/) with a declarative `schema.sql` driving versioned migrations. |
| SQL queries | [`sqlc`](https://sqlc.dev/) for type-safe Go bindings. |

The repository keeps generated code under `gen/` (gitignored) and `sdk/.../gen/` (gitignored, regenerated by `buf generate`).

Breaking-change detection runs in CI: `buf breaking --against '.git#branch=main'`.

## Match Lifecycle

```mermaid
sequenceDiagram
    autonumber
    participant C as Client
    participant G as vfx gateway
    participant V as Valkey
    participant A as Agones
    participant R as vfx room

    C->>G: CreateTicket(game_mode, attributes)
    G->>V: enqueue ticket
    G-->>C: ticket_id

    C->>G: WatchTicket(ticket_id)
    Note over G: matchmaking worker<br/>scans queue, groups tickets
    G->>A: Allocate(game_mode)
    A->>R: state = Allocated
    A-->>G: GameServer (endpoint, ports)
    G-->>C: TicketMatched(endpoint, session_token)

    C->>R: WebTransport connect (+ token)
    R->>R: verify token, instantiate WASM
    Note over C,R: realtime gameplay

    R->>R: detect game end
    R->>+G: commit final result (via shared DB)
    R->>A: Shutdown
    A->>R: terminate Pod
    Note over A: Fleet replenishes with new Ready Pod
```

## Reconnection

Players will lose their connection to the room — Wi-Fi to cellular handoff, app backgrounding, brief network blips, full restarts. The protocol handles each of these without forcing a rematch.

```mermaid
sequenceDiagram
    autonumber
    participant C as Client
    participant G as vfx gateway
    participant R as vfx room
    participant P as Plugin (WASM)

    Note over C,R: realtime gameplay in progress
    C--xR: connection lost (network blip)
    R->>P: NetworkEvent{PlayerDisconnected}
    Note over R: keep slot reserved<br/>(grace period, default 60s)

    Note over C: app resumes / network restored
    C->>G: GetCurrentMatch()
    G-->>C: CurrentMatch{endpoint, session_token, expires_at}
    C->>R: WebTransport reconnect (+ token)
    R->>R: match player_id to reserved slot
    R->>P: NetworkEvent{PlayerReconnected}
    R->>C: StateSnapshot (catch up to current tick)
    Note over C,R: gameplay resumes
```

Key design points:

- The room keeps the player's slot reserved for a configurable grace period (default 60s) after a disconnect.
- `MatchService.GetCurrentMatch` returns a fresh short-lived session token if the player is still in a match — the client does not need to remember its old token, only its login.
- The plugin sees three distinct events: `PlayerDisconnected` (transient), `PlayerReconnected` (came back), and `PlayerLeft` (permanently gone). The plugin decides how to handle each — pause the player's actions during disconnect, replay missed events on reconnect, or surrender the slot.
- On reconnect, the room sends a full `StateSnapshot` so the client catches up immediately without replaying every delta.

## Storage

Durable data lives in two substrates chosen by shape: structured records in PostgreSQL, opaque file bytes in object storage. The gateway is the control plane for both and never proxies file bytes.

### PostgreSQL

- Holds accounts, friends, match history, leaderboards, and file *metadata* (filename, size, content hash, tags).
- Managed services (Cloud SQL, RDS, Neon, Supabase) and self-hosted instances are both first-class.
- Distributed SQL backends are not a supported target.

### Object storage (GCS)

- Holds the file bytes for two buckets: **player data** (owner-scoped save data) and **title storage** (operator-published, read-only, tag-gated content and remote config).
- Transfers go directly between the client and the store over V4 signed URLs, so large blobs never traverse the gateway. The gateway authorizes the request, enforces per-player quotas, issues the URL, and — for writes — verifies the uploaded object before recording its metadata.
- A metadata row exists only after a committed upload; an interrupted upload leaves an orphan object with no row, reclaimed by the bucket's lifecycle/sweep. Deletes remove the metadata row first (the source of truth) and best-effort the object second.
- Signing uses IAM SignBlob (Workload Identity) in production, so no service-account key is placed on disk; local development and CI run [fake-gcs-server](https://github.com/fsouza/fake-gcs-server).

### Valkey

- Ephemeral state: matchmaking queue, ticket subscriptions, room allocation index, leaderboard caches.
- Treated as recoverable: an outage stops new matchmaking but does not corrupt anything persistent.

## Plugin Model

vfx plugins are WebAssembly modules executed by `wazero` inside the room process. The host and the guest communicate through a Protocol Buffers ABI defined in `schema/api/plugin/v1/plugin.proto`.

```mermaid
flowchart LR
    subgraph Room["vfx room process"]
        Host["Host runtime<br/>(WebTransport, tick loop, IO)"]
        subgraph WASM["wazero instance"]
            Plugin["Game plugin<br/>(Go / Rust / C# ... → WASM)"]
        end
    end

    Host -- "TickContext (proto)" --> Plugin
    Plugin -- "TickResult (proto)" --> Host
    Host -- "host functions<br/>(log, time, rng)" --> Plugin
```

Design constraints:

- The host calls the plugin once per tick with all queued player actions batched into a single `TickContext`. The plugin returns a single `TickResult` containing state diffs and outbound messages.
- This batching limits FFI cost to two crossings per tick, regardless of input volume.
- Plugins must be deterministic when given the same `TickContext`, including seeded RNG. This enables replays and dispute resolution.

Plugin manifests (capability declarations) are returned from a one-time `Init` call when the room loads a plugin. They are not duplicated in sidecar files.

## Deployment Topology

The same Helm chart is used for every environment. Differences are expressed through values files.

| Environment | Cluster | Database |
| --- | --- | --- |
| Local development | kind | PostgreSQL via `compose.yml` |
| Small production (VPS) | k3s, single node | PostgreSQL in `compose.yml` or managed |
| Cloud production | Managed Kubernetes | Managed PostgreSQL service |

```mermaid
flowchart TB
    subgraph Local["Local development"]
        Compose["compose.yml<br/>postgres + valkey"]
        Kind["kind cluster<br/>vfx + Agones"]
        Compose <-->|"host.docker.internal"| Kind
    end

    subgraph Cloud["Cloud production"]
        K8s["Managed Kubernetes<br/>vfx + Agones"]
        ManagedDB[("Managed PostgreSQL")]
        ManagedCache[("Managed Valkey")]
        K8s --> ManagedDB
        K8s --> ManagedCache
    end
```

### Why DB outside Kubernetes locally

Running PostgreSQL via `compose.yml` rather than inside the kind cluster:

- Mirrors production where the database is a managed service outside the workload cluster.
- Survives cluster recreation, so iteration on application code does not wipe data.
- Makes direct inspection (`psql`) and debugging straightforward.

### Room data-plane reachability

The control plane (gateway, Connect RPC over HTTP/2) sits behind an L7 load balancer like any stateless service. The data plane is different: a client must reach the *specific* room hosting its match, over UDP (QUIC), so it cannot go through an L7 HTTP load balancer.

How a client reaches its room:

1. The matchmaker allocates a GameServer (`Allocator.Allocate`), and Agones reports that GameServer's externally reachable `address` and dynamically assigned `port`.
2. The gateway returns exactly that `address:port` to the client as the match endpoint.
3. The client opens a WebTransport (HTTP/3 over QUIC, UDP) session straight to it — bypassing the L7 load balancer.

In a cloud cluster the GameServer's address is the node's external IP and the port is the Agones-assigned host port (default range 7000–8000). Operators expose that range: a node port range through the cloud firewall, or a UDP-capable network load balancer that preserves the port. The host port is dynamic precisely so many rooms can share a node without colliding.

**kind limitation.** kind runs the cluster inside Docker, so a GameServer binds an in-cluster IP (e.g. `172.x`) that is not reachable from the host. The control plane is fully exercisable on kind — allocation flips a GameServer to `Allocated` and its `address:port` is handed back — but the actual WebTransport data plane is not reachable from a host-side client. The data plane is instead verified on the compose setup (a single local room), and the design above is what production clusters use. Forcing a static host port (`portPolicy: Static`) with a matching kind `extraPortMappings` entry can expose one room for manual testing, but that diverges from the dynamic-port model real deployments rely on.

## Matchmaking

Matchmaking is implemented inside the gateway as a worker goroutine. It is intentionally a small piece of code rather than a separate dependency.

- Tickets are stored in Valkey, scored by game mode, region, and rating.
- The worker scans the queue at a fixed interval (200ms by default).
- Compatible tickets are grouped according to a `MatchingPolicy` that defines tier-based relaxation: after N seconds of waiting, the rating spread and region constraints loosen.
- On success the worker calls the Agones Allocator, signs a short-lived session token, and notifies the players through their `WatchTicket` server stream.

This design keeps matchmaking visible and tunable in regular Go code. A pluggable interface is kept open so that a heavier matchmaking system can be substituted in the future without changing the public API.

## Testing Strategy

Tests are written in standard Go style — table-driven cases on the standard `testing` package, with no assertion or BDD framework. Three techniques are applied deliberately:

- **Boundary-value and equivalence-partition tests** for rules with edges: the matcher's rating window and region-relax thresholds, the nickname length limit, refresh-token expiry at exactly its deadline.
- **Property-based tests** (`pgregory.net/rapid`) for invariants that must hold across all inputs: the rock-paper-scissors rule's algebra, token sign/verify round trips, the matcher's group shape, and the all-or-nothing contract of the queue's atomic claim.
- **Race detection** — the suite runs under `go test -race`.

| Layer | Scope | Infrastructure |
| --- | --- | --- |
| Unit | Pure functions, domain rules, in-memory adapters | none (`go test`) |
| Component | Connect RPC handlers, PostgreSQL repositories | a database via `DATABASE_URL`; skipped when unset |
| Integration | Valkey queue and leader lock, Agones SDK lifecycle | Valkey via `VALKEY_URL`, or a fake Agones SDK; skipped when unset |
| End-to-end | gateway + room over the real protocols | compose (single room) and a kind cluster with Agones |

Shared test utilities live under `internal/testutils/`:

- `testconnect` — boots an in-process Connect RPC server with the production handler wiring.
- `testdb` — a PostgreSQL pool that truncates between tests and skips when `DATABASE_URL` is unset.
- `faker` — deterministic IDs (`UUIDv5("name")`) for stable assertions.

## Observability

- All processes export OpenTelemetry traces and metrics.
- W3C Trace Context propagates from the client through the gateway and into the room.
- Per-room metrics include tick duration, FFI call count, player count, and WASM memory usage.
- Health and readiness probes are implemented for every workload.

## Non-Goals

- **MMO-style persistent worlds.** vfx hosts discrete matches, not zones with thousands of co-located players.
- **Multi-tenant SaaS.** vfx is meant to be deployed by an operator running their own games.
- **Distributed SQL.** Supporting multiple SQL dialects is a constant tax that conflicts with the goal of staying small. PostgreSQL is the only target.
- **Built-in match-function plugin system (separate from game logic).** Matchmaking lives in vfx code, configurable but not pluggable in the way game logic is.
