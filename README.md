# vfx

A lightweight, WebTransport-native, and WASM-driven game server engine.

## Status

Early development. The architecture is settled; implementation has not started.

## Highlights

- **WebTransport (HTTP/3)** for low-latency realtime communication, with reliable streams and unreliable datagrams unified in one connection.
- **WASM-sandboxed game logic** (`wazero`), letting you write plugins in any language that targets WebAssembly.

## License

[MIT](./LICENSE)
