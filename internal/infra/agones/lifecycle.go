// Package agones adapts the Agones game-server SDK to the room daemon's lifecycle.
//
// When a room runs inside an Agones-managed pod, an SDK sidecar exposes a local gRPC endpoint the daemon uses to report its state: Ready to accept allocation, periodic Health pings so Agones knows it is alive, and Shutdown when the process is winding down.
// Outside Agones (compose or local runs) there is no sidecar, so this is wired only when VFX_ROOM_AGONES_ENABLED is set.
package agones

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	agonessdk "agones.dev/agones/sdks/go"
)

// healthSender is the slice of the Agones SDK this package needs; an interface keeps Start testable without a live sidecar.
type healthSender interface {
	Ready() error
	Health() error
	Shutdown() error
}

// Start connects to the Agones SDK sidecar, marks the game server Ready, and launches a health-ping loop until ctx is cancelled or the returned stop func runs.
// stop sends Shutdown so Agones can recycle the pod.
func Start(ctx context.Context, healthInterval time.Duration, logger *slog.Logger) (func(), error) {
	s, err := agonessdk.NewSDK()
	if err != nil {
		return nil, fmt.Errorf("agones: connect sdk: %w", err)
	}
	return start(ctx, s, healthInterval, logger)
}

func start(ctx context.Context, s healthSender, healthInterval time.Duration, logger *slog.Logger) (func(), error) {
	if err := s.Ready(); err != nil {
		return nil, fmt.Errorf("agones: ready: %w", err)
	}
	logger.Info("agones: marked ready")

	healthCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(healthInterval)
		defer ticker.Stop()
		for {
			select {
			case <-healthCtx.Done():
				return
			case <-ticker.C:
				if err := s.Health(); err != nil {
					logger.Warn("agones: health ping failed", "err", err)
				}
			}
		}
	}()

	stop := func() {
		cancel()
		<-done
		if err := s.Shutdown(); err != nil {
			logger.Warn("agones: shutdown", "err", err)
		}
	}
	return stop, nil
}
