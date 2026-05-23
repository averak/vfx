//go:build wasip1

// Command wasm is the WebAssembly entry point for the RPS plugin.
//
// It registers the RPS Game factory with the guest SDK and is compiled to WASM by TinyGo:
//
//	tinygo build -o rps.wasm -buildmode=c-shared -target=wasip1 \
//	  ./examples/rps/plugin/cmd/wasm
//
// The resulting module is loaded by the room daemon's wazero host (point VFX_ROOM_PLUGIN_PATH at the .wasm file).
// The exact same Game type drives the in-process vfx-rps binary, so the rules are verified once and run in both worlds.
package main

import (
	rps "github.com/averak/vfx/examples/rps/plugin"
	vfxplugin "github.com/averak/vfx/sdk/plugin/go"
)

func init() {
	vfxplugin.Register(func() vfxplugin.Game { return rps.NewGame() })
}

func main() {}
