package plugin

import (
	"context"

	pluginv1 "github.com/averak/vfx/gen/go/plugin/v1"
	"github.com/averak/vfx/internal/domain/plugin"
)

// Factory satisfies plugin.Factory for the Go-native (in-process) RPS
// plugin used by the vfx-rps binary. The WASM build of the same Game
// is loaded by the room's wazero host instead and does not go through
// this Factory.
type Factory struct{}

// NewFactory returns a Factory ready to register with a vfx plugin
// Registry.
func NewFactory() *Factory { return &Factory{} }

// Name is the identifier matched against VFX_ROOM_PLUGIN_PATH.
func (*Factory) Name() string { return pluginName }

// Create instantiates a fresh RPS plugin for one match.
func (*Factory) Create(_ context.Context) (plugin.Plugin, error) {
	return &hostAdapter{game: NewGame()}, nil
}

// hostAdapter wraps the transport-free Game in the host Plugin
// interface, supplying the context parameters and Close that the
// in-process contract requires but the pure game logic does not.
type hostAdapter struct {
	game *Game
}

func (a *hostAdapter) Init(_ context.Context, req *pluginv1.InitRequest) (*pluginv1.InitResponse, error) {
	return a.game.Init(req)
}

func (a *hostAdapter) OnTick(_ context.Context, req *pluginv1.OnTickRequest) (*pluginv1.OnTickResponse, error) {
	return a.game.OnTick(req)
}

func (a *hostAdapter) OnGameEnd(_ context.Context, req *pluginv1.OnGameEndRequest) (*pluginv1.OnGameEndResponse, error) {
	return a.game.OnGameEnd(req)
}

func (*hostAdapter) Close() error { return nil }
