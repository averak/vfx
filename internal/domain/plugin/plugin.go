// Package plugin is the contract between the room daemon and the
// game-logic code it hosts.
//
// The interface mirrors the proto ABI (Init, OnTick, OnGameEnd) but
// stays in Go terms so two backends can satisfy it:
//
//   - Go-native plugins linked into a custom vfx-room binary. Faster to
//     author and easier to debug, with no WASM toolchain needed.
//   - WASM modules loaded by wazero. Same Plugin interface, a different
//     Factory implementation. Authors switch by recompiling their code
//     with TinyGo and pointing the room at the .wasm file for sandboxed
//     execution.
package plugin

import (
	"context"

	pluginv1 "github.com/averak/vfx/gen/go/plugin/v1"
)

// Plugin is one running instance — one match. The room daemon creates
// a fresh Plugin via Factory.Create for every Allocation and closes it
// when the match ends.
type Plugin interface {
	Init(ctx context.Context, req *pluginv1.InitRequest) (*pluginv1.InitResponse, error)
	OnTick(ctx context.Context, req *pluginv1.OnTickRequest) (*pluginv1.OnTickResponse, error)
	OnGameEnd(ctx context.Context, req *pluginv1.OnGameEndRequest) (*pluginv1.OnGameEndResponse, error)
	Close() error
}

// Factory produces fresh Plugin instances. Implementations may hold
// expensive resources (a wazero compiled module, a network handle,
// etc.) that they want to share across instances, so the room daemon
// reuses the same Factory throughout its lifetime.
type Factory interface {
	// Name is the identifier matched against the VFX_ROOM_PLUGIN_NAME
	// env var so a single binary can ship multiple plugins.
	Name() string

	// Create instantiates a fresh Plugin. The returned Plugin owns no
	// shared state with previous instances; closing it must release
	// every resource it held.
	Create(ctx context.Context) (Plugin, error)
}
