# vfx

A lightweight, WebTransport-native, and WASM-driven game server engine.

## Status

Early development. The Phase 1 MVP runs end-to-end locally — see
[`examples/rps`](./examples/rps) for a complete rock-paper-scissors
match played between two CLI clients against a live gateway and room
daemon.

## Highlights

- **WebTransport (HTTP/3)** for low-latency realtime communication, with reliable streams and unreliable datagrams unified in one connection.
- **WASM-sandboxed game logic** (`wazero`), letting you write plugins in any language that targets WebAssembly.

## License

[MIT](./LICENSE)
