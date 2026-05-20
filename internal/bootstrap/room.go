package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/averak/vfx/internal/domain/plugin"
	"github.com/averak/vfx/internal/infra/config"
	"github.com/averak/vfx/internal/infra/token"
	roomhandler "github.com/averak/vfx/internal/presentation/room"
	usecaseroom "github.com/averak/vfx/internal/usecase/room"
)

// Room bundles the dependencies the room daemon needs at runtime.
type Room struct {
	Config  *config.Room
	Signer  *token.Signer
	Manager *usecaseroom.Manager
	Server  *roomhandler.Server
}

// NewRoom constructs and validates the room container. The registry
// must already carry the plugin selected by VFX_ROOM_PLUGIN_NAME, or
// startup fails with a helpful error.
func NewRoom(_ context.Context, registry *plugin.Registry, logger *slog.Logger) (*Room, func(), error) {
	cfg, err := config.LoadRoom()
	if err != nil {
		return nil, nil, err
	}

	if cfg.PluginPath == "" {
		// In Phase 1 PluginPath doubles as the plugin name (no wazero
		// loader yet). Treat empty as "use the first registered
		// plugin" so a quickstart binary can ship a single example.
		names := registry.Names()
		if len(names) == 0 {
			return nil, nil, fmt.Errorf("room: no plugin available; build a binary that registers one or set VFX_ROOM_PLUGIN_PATH")
		}
		cfg.PluginPath = names[0]
	}

	factory, err := registry.Lookup(cfg.PluginPath)
	if err != nil {
		return nil, nil, fmt.Errorf("room: %w (available: %v)", err, registry.Names())
	}

	signer := token.NewSigner(cfg.JWTSecret)
	manager := usecaseroom.NewManager(factory, logger)
	server, err := roomhandler.NewServer(cfg, signer, manager, logger)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {}

	return &Room{
		Config:  cfg,
		Signer:  signer,
		Manager: manager,
		Server:  server,
	}, cleanup, nil
}
