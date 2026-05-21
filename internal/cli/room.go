package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/averak/vfx/internal/bootstrap"
	"github.com/averak/vfx/internal/domain/plugin"
	"github.com/averak/vfx/internal/infra/agones"
	"github.com/averak/vfx/internal/infra/tracing"
)

func newRoomCmd(registry *plugin.Registry) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "room",
		Short: "Run the match-hosting WebTransport server",
		Long: "Starts a WebTransport server that hosts matches via the configured plugin. " +
			"Session tokens issued by the gateway grant connection rights.\n\n" +
			"Requires a registered plugin; build a custom binary that calls cli.NewRootCmd " +
			"with the desired Factory registered, or invoke an example binary like vfx-rps.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRoom(cmd.Context(), registry)
		},
	}
	return cmd
}

func runRoom(ctx context.Context, registry *plugin.Registry) error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	shutdownTracing, err := tracing.Setup(ctx, "vfx-room")
	if err != nil {
		return fmt.Errorf("room tracing: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if shutdownErr := shutdownTracing(shutdownCtx); shutdownErr != nil {
			logger.Warn("room tracing shutdown", "err", shutdownErr)
		}
	}()

	container, cleanup, err := bootstrap.NewRoom(ctx, registry, logger)
	if err != nil {
		return fmt.Errorf("room bootstrap: %w", err)
	}
	defer cleanup()

	if container.Config.AgonesEnabled {
		stopAgones, agonesErr := agones.Start(ctx, container.Config.AgonesHealthInterval, logger)
		if agonesErr != nil {
			return fmt.Errorf("room agones: %w", agonesErr)
		}
		defer stopAgones()
	}

	if addr := container.Config.MetricsAddr; addr != "" {
		stopMetrics := startRoomMetricsServer(addr, container, logger)
		defer stopMetrics()
	}

	return container.Server.ListenAndServe(ctx)
}

// startRoomMetricsServer runs the room's plain-HTTP listener for
// Prometheus scraping and orchestrator probes, alongside the
// WebTransport (HTTP/3) tier. It returns a shutdown function.
func startRoomMetricsServer(addr string, container *bootstrap.Room, logger *slog.Logger) func() {
	mux := http.NewServeMux()
	mux.Handle("/metrics", container.Metrics.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		logger.Info("room metrics listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("room metrics server failed", "err", err)
		}
	}()

	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Warn("room metrics shutdown", "err", err)
		}
	}
}
