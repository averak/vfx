# vfx

A lightweight, WebTransport-native, WASM-driven game server engine.

vfx hosts realtime multiplayer matches. You write the game logic as a WebAssembly (or Go-native) plugin; vfx handles transport, matchmaking, room lifecycle, and persistence. It ships as a single binary that runs a different role per subcommand (`gateway`, `room`, `admin`, `migrate`).

> **Status:** early development. The engine runs end to end today — [`examples/rps`](./examples/rps) plays a full rock-paper-scissors match between clients against a live gateway and room daemon.

## Features

- **WebTransport (HTTP/3) realtime** — reliable streams and unreliable datagrams over one connection, for native and browser clients alike.
- **Sandboxed game logic** — plugins run as WebAssembly under [`wazero`](https://wazero.io/), so you can write a game in any language that targets WASM (or link a Go plugin in directly).
- **Kubernetes-native** — one process per match, with the room fleet's lifecycle managed by [Agones](https://agones.dev/).
- **Authentication** — anonymous guest accounts and OIDC sign-in (Google, Apple), with account linking to upgrade a guest into a federated identity.
- **Matchmaking** — ticket queue with rating- and region-aware tiers that widen as a player waits, parties that are placed into one match together, and atomic claiming so replicas never double-match a ticket.
- **Leaderboards** — configurable ascending or descending boards, keep-best scoring applied atomically, and ranked / around-player queries.
- **Player & title storage** — owner-scoped save data and operator-published content. File bytes stream directly between the client and object storage over signed URLs; the gateway brokers only metadata and authorization, never the bytes.
- **Social graph** — friend requests with mutual auto-accept, friend lists, blocking, and player groups (create / join / leave / members).
- **Chat** — 1:1 direct messages and membership-gated group channels with paginated history, plus a realtime channel subscription that streams new messages as they are posted.

## Quickstart

You need [`mise`](https://mise.jdx.dev/) and Docker.

```bash
mise install          # pinned Go, buf, sqlc, atlas, helm, ...
docker compose up -d  # PostgreSQL + Valkey
mise run db-migrate   # apply migrations
```

Then start the gateway, a room hosting rock-paper-scissors, and two players:

```bash
go run ./cmd/vfx gateway
go run ./examples/rps/cmd/vfx-rps room          # needs VFX_ROOM_TLS_CERT / VFX_ROOM_TLS_KEY
go run ./examples/rps/cmd/rps-cli --device alice --auto
go run ./examples/rps/cmd/rps-cli --device bob --auto
```

The full walkthrough, including a local TLS cert for the room, is in [`examples/rps/README.md`](./examples/rps/README.md).

## Writing a game

A game implements the `plugin.v1` ABI — `Init`, `OnTick`, `OnGameEnd`. The room calls it once per tick with all batched player input and broadcasts the state it returns. The same source links into a custom `vfx` binary as a Go-native plugin or compiles to WASM with TinyGo for the wazero host; the rps example does both. Start from the guest SDK in [`sdk/plugin/go`](./sdk/plugin/go).

## How it works

A client reaches the **control plane** — auth, matchmaking, leaderboards, friends, chat, and storage — over Connect RPC on the stateless gateway, then opens a **data-plane** WebTransport session straight to the room hosting its match. [`docs/architecture.md`](./docs/architecture.md) covers the components, protocols, the match and reconnection lifecycles, deployment topology, and the internal layering.

## Development

```bash
mise run gen   # regenerate protobuf + sqlc code
mise run lint  # buf + golangci-lint + gofumpt
mise run test  # unit + component tests (component tests need PostgreSQL + Valkey)
```

CI runs the full suite under `-race` against ephemeral PostgreSQL and Valkey.

## License

[MIT](./LICENSE)
