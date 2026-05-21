# Rock-paper-scissors

The canonical Phase 1 sample game.

## What this demonstrates

- Anonymous login via the gateway's `AuthService`.
- Ticket-based matchmaking through `MatchService.CreateTicket` and the `WatchTicket` server stream.
- Allocation of a room by the matchmaker worker (stub allocator).
- WebTransport handshake with a short-lived signed session token.
- A `plugin.Plugin` implementation hosted by the room daemon (Go-native today; a TinyGo → WASM build will replace it once the wazero loader lands).
- State deltas streamed back to clients as JSON-encoded game state.

## Layout

```
examples/rps/
├── plugin/                  # Game logic (satisfies plugin.Plugin)
├── cmd/
│   ├── vfx-rps/             # Custom vfx binary: registers the plugin
│   └── rps-cli/             # CLI client (Go SDK) for testing
├── client-web/              # Browser client (TypeScript SDK + Vite)
└── README.md
```

## Run it locally

Prerequisites:

- `mise install` (installs Go, buf, sqlc, atlas, helm, kubectl, kind, golangci-lint, gofumpt at the pinned versions).
- `docker compose up -d` (starts PostgreSQL and Valkey).
- `mise run db-migrate` (applies migrations).
- A TLS cert + key for the room daemon (any self-signed pair works for localhost). Generate one with:
  ```bash
  mkdir -p deploy/local/certs
  openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
    -keyout deploy/local/certs/dev-key.pem \
    -out   deploy/local/certs/dev-cert.pem \
    -subj "/CN=localhost" \
    -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"
  ```

Then in four terminals (or use a runner of your choice):

```bash
# Terminal 1 — gateway
export DATABASE_URL="postgres://vfx:dev@localhost:5432/vfx?sslmode=disable"
export VFX_JWT_SECRET="dev-only-do-not-use-in-production"
go run ./cmd/vfx gateway

# Terminal 2 — room hosting RPS
export VFX_JWT_SECRET="dev-only-do-not-use-in-production"
export VFX_ROOM_TLS_CERT="$PWD/deploy/local/certs/dev-cert.pem"
export VFX_ROOM_TLS_KEY="$PWD/deploy/local/certs/dev-key.pem"
go run ./examples/rps/cmd/vfx-rps room

# Terminal 3 — player Alice
go run ./examples/rps/cmd/rps-cli --device alice --nickname Alice --auto

# Terminal 4 — player Bob
go run ./examples/rps/cmd/rps-cli --device bob   --nickname Bob   --auto
```

`--auto` picks R/P/S at random every ~800 ms. Drop the flag and type
choices interactively into the CLI's stdin.

## Run the web client

The browser client under `client-web/` uses the TypeScript SDK
(`sdk/client/ts`) and the browser-native WebTransport API.

```bash
cd examples/rps/client-web
npm install
npm run dev   # serves on http://localhost:5173
```

Open two browser tabs to play a match against yourself.

**Certificate caveat.** Browser WebTransport requires a certificate the
browser trusts. The self-signed RSA pair used by the CLI quickstart is
*not* eligible for the WebTransport hash-pinning API (that needs an
ECDSA cert with short validity). The simplest path is
[`mkcert`](https://github.com/FiloSottile/mkcert), which installs a
local CA your browser trusts:

```bash
mkcert -install
mkcert -cert-file deploy/local/certs/dev-cert.pem \
       -key-file  deploy/local/certs/dev-key.pem \
       localhost 127.0.0.1
```

Restart `vfx-rps room` after regenerating the certificate.

## How the plugin works

Two players, best of three rounds. Each round both players send a
single byte (`R`, `P`, or `S`) as the `payload` of a `PlayerInput`
frame. As soon as both choices for a round are recorded the plugin
resolves the round, appends it to the history, and emits a state
delta — JSON-encoded `gameState` — which the room broadcasts to every
attached session.

The match ends when one player reaches two round wins, or when three
rounds have been played. `OnGameEnd` runs once and persists the final
roster + per-player rank.

## Why Go-native and not WASM (yet)

The same `plugin.Plugin` interface will be satisfied by a wazero-backed
loader in Phase 2 that reads a `.wasm` file produced by TinyGo. The
RPS rules in `plugin/plugin.go` are deliberately pure Go with no host
function calls so the same source can be compiled to WASM unchanged
once that path is in place.
