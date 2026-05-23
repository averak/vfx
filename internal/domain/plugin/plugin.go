// Package plugin is the contract between the room daemon and the game-logic code it hosts.
//
// The interface mirrors the proto ABI (Init, OnTick, OnGameEnd) but stays in Go terms so two backends can satisfy it:
//   - Go-native plugins linked into a custom vfx-room binary: faster to author and debug, no WASM toolchain needed.
//   - WASM modules loaded by wazero for sandboxed execution: authors switch by recompiling with TinyGo and pointing the room at the .wasm file.
package plugin

import (
	"context"

	pluginv1 "github.com/averak/vfx/gen/go/plugin/v1"
)

// Plugin is one running instance — one match.
// The room daemon creates a fresh Plugin via Factory.Create for every allocation and closes it when the match ends.
type Plugin interface {
	Init(ctx context.Context, req *pluginv1.InitRequest) (*pluginv1.InitResponse, error)
	OnTick(ctx context.Context, req *pluginv1.OnTickRequest) (*pluginv1.OnTickResponse, error)
	OnGameEnd(ctx context.Context, req *pluginv1.OnGameEndRequest) (*pluginv1.OnGameEndResponse, error)
	Close() error
}

// Factory produces fresh Plugin instances.
// It may hold expensive shared resources (a wazero compiled module, say), so the room daemon reuses one Factory for its whole lifetime.
type Factory interface {
	Name() string
	// Create returns a Plugin sharing no state with previous instances; closing it must release everything it held.
	Create(ctx context.Context) (Plugin, error)
}
