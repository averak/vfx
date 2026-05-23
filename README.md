# vfx

A lightweight, WebTransport-native, WASM-driven game server engine.

vfx hosts realtime multiplayer matches. Game logic ships as a WebAssembly (or Go-native) plugin; the engine handles transport, matchmaking, room lifecycle, and persistence. It is a single binary that runs as a different role per subcommand (`gateway`, `room`, `admin`, `migrate`).

> **Status:** early development. The engine runs end to end locally — see [`examples/rps`](./examples/rps) for a complete rock-paper-scissors match played between clients against a live gateway and room daemon.

## Highlights

- **WebTransport (HTTP/3)** for realtime: reliable streams and unreliable datagrams unified in one connection. Native and browser clients both connect.
- **WASM-sandboxed game logic** via [`wazero`](https://wazero.io/) — write plugins in any language that targets WebAssembly (or link a Go plugin in directly).
- **Kubernetes-native** room hosting: one process per match, lifecycle managed by [Agones](https://agones.dev/).
- **Schema-first**: Protocol Buffers for the wire protocols and the plugin ABI; declarative SQL with [atlas](https://atlasgo.io/) and [sqlc](https://sqlc.dev/).
- **Strict layering**: a Clean Architecture dependency rule enforced mechanically by `depguard`, with the transaction boundary owned by the usecase layer.

## Quickstart

Prerequisites: [`mise`](https://mise.jdx.dev/) and Docker.

```bash
mise install                 # pinned Go + buf, sqlc, atlas, helm, kind, ...
docker compose up -d         # PostgreSQL + Valkey
mise run db-migrate          # apply migrations
```

Then run the gateway, an RPS-hosting room, and two players. The full
walkthrough (including generating a local TLS cert for the room) is in
[`examples/rps/README.md`](./examples/rps/README.md); in short:

```bash
# gateway
go run ./cmd/vfx gateway

# room hosting rock-paper-scissors (needs VFX_ROOM_TLS_CERT/KEY)
go run ./examples/rps/cmd/vfx-rps room

# two auto-playing clients
go run ./examples/rps/cmd/rps-cli --device alice --auto
go run ./examples/rps/cmd/rps-cli --device bob --auto
```

## Architecture

See [`docs/architecture.md`](./docs/architecture.md) for components, protocols, the match and reconnection lifecycles, deployment topology, and the internal code layering. [`docs/roadmap.md`](./docs/roadmap.md) tracks what is built and what is planned.

A client reaches the **control plane** (auth, matchmaking, storage) over Connect RPC on the stateless gateway, then opens a **data-plane** WebTransport session straight to the specific room hosting its match.

## Repository layout

```
cmd/vfx/            # vanilla entry point (no plugins registered)
internal/
  domain/           # entities and enterprise rules (no infra imports)
  usecase/          # application orchestration; declares its ports
  infra/            # adapters: postgres, valkey, jwt, agones, wazero
  presentation/     # Connect handlers + the WebTransport room server
  bootstrap/        # per-subcommand dependency wiring
sdk/
  client/{go,ts}    # client SDKs
  plugin/go         # guest SDK for writing WASM plugins
examples/rps/       # the reference game: plugin, binaries, web + CLI clients
schema/{api,db}     # protobuf and SQL sources of truth
deploy/{helm,local} # Helm chart and local kind/compose config
```

## Plugins

A game implements the `plugin.v1` ABI (`Init` / `OnTick` / `OnGameEnd`). The room calls it once per tick with all batched inputs and broadcasts the resulting state. The same Go source can be linked into a custom `vfx`-room binary or compiled to WASM with TinyGo and loaded by the wazero host — the rps example builds both. See the guest SDK in [`sdk/plugin/go`](./sdk/plugin/go).

## Development

```bash
mise run gen        # regenerate protobuf + sqlc code
mise run lint       # buf + golangci-lint + gofumpt
mise run test       # unit + component tests (component tests need a DB/Valkey)
```

CI runs the full suite under `-race` against ephemeral PostgreSQL and Valkey services.

## License

[MIT](./LICENSE)
