package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/averak/vfx/internal/domain/plugin"
	"github.com/averak/vfx/internal/infra/config"
	"github.com/averak/vfx/internal/infra/metrics"
	"github.com/averak/vfx/internal/infra/plugin/wazerohost"
	"github.com/averak/vfx/internal/infra/token"
	roomhandler "github.com/averak/vfx/internal/presentation/room"
	usecaseroom "github.com/averak/vfx/internal/usecase/room"
)

type Room struct {
	Config  *config.Room
	Signer  *token.Signer
	Manager *usecaseroom.Manager
	Server  *roomhandler.Server
	Metrics *metrics.Registry
}

// roomMetrics adapts the Prometheus registry to usecaseroom.Metrics so the usecase layer carries no concrete metrics dependency.
type roomMetrics struct {
	reg *metrics.Registry
}

func (m roomMetrics) IncActiveMatches()           { m.reg.RoomMatchesActive.Inc() }
func (m roomMetrics) DecActiveMatches()           { m.reg.RoomMatchesActive.Dec() }
func (m roomMetrics) ObserveTick(d time.Duration) { m.reg.RoomTickDuration.Observe(d.Seconds()) }

// NewRoom constructs and validates the room container.
//
// Plugin selection follows VFX_ROOM_PLUGIN_PATH:
//   - a path ending in .wasm is compiled and run by the wazero host, the production sandboxed path;
//   - any other value names a plugin registered into the supplied registry (the in-process Go path used by the example vfx-rps binary);
//   - empty falls back to the first registered plugin, so a single-plugin quickstart binary needs no configuration.
func NewRoom(ctx context.Context, registry *plugin.Registry, logger *slog.Logger) (*Room, func(), error) {
	cfg, err := config.LoadRoom()
	if err != nil {
		return nil, nil, err
	}

	factory, factoryCleanup, err := selectFactory(ctx, cfg.PluginPath, registry)
	if err != nil {
		return nil, nil, err
	}

	signer := token.NewSigner(cfg.JWTSecret)
	metricsReg := metrics.NewRegistry()
	manager := usecaseroom.NewManager(ctx, factory, logger, roomMetrics{reg: metricsReg})
	server, err := roomhandler.NewServer(cfg, signer, manager, logger)
	if err != nil {
		factoryCleanup()
		return nil, nil, err
	}

	cleanup := func() {
		manager.Close()
		factoryCleanup()
	}

	return &Room{
		Config:  cfg,
		Signer:  signer,
		Manager: manager,
		Server:  server,
		Metrics: metricsReg,
	}, cleanup, nil
}

func selectFactory(ctx context.Context, pluginPath string, registry *plugin.Registry) (plugin.Factory, func(), error) {
	noopCleanup := func() {}

	if strings.HasSuffix(pluginPath, ".wasm") {
		wasm, err := os.ReadFile(pluginPath) //nolint:gosec // operator-supplied plugin path.
		if err != nil {
			return nil, nil, fmt.Errorf("room: read plugin %q: %w", pluginPath, err)
		}
		factory, err := wazerohost.NewFactory(ctx, pluginPath, wasm)
		if err != nil {
			return nil, nil, err
		}
		cleanup := func() { _ = factory.Close(ctx) } //nolint:errcheck // shutdown cleanup.
		return factory, cleanup, nil
	}

	name := pluginPath
	if name == "" {
		names := registry.Names()
		if len(names) == 0 {
			return nil, nil, fmt.Errorf("room: no plugin available; set VFX_ROOM_PLUGIN_PATH to a .wasm file or build a binary that registers one")
		}
		name = names[0]
	}
	factory, err := registry.Lookup(name)
	if err != nil {
		return nil, nil, fmt.Errorf("room: %w (available: %v)", err, registry.Names())
	}
	return factory, noopCleanup, nil
}
