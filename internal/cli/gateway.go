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
	"github.com/averak/vfx/internal/infra/leaderlock"
	"github.com/averak/vfx/internal/infra/tracing"
	"github.com/averak/vfx/internal/presentation/gateway"
)

func newGatewayCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gateway",
		Short: "Run the stateless API gateway",
		Long: "Starts the Connect RPC server that handles authentication, matchmaking, " +
			"storage APIs, and admin RPCs. The gateway is stateless and can be scaled " +
			"horizontally behind any L7 load balancer.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runGateway(cmd.Context())
		},
	}
	return cmd
}

func runGateway(ctx context.Context) error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	shutdownTracing, err := tracing.Setup(ctx, "vfx-gateway")
	if err != nil {
		return fmt.Errorf("gateway tracing: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if shutdownErr := shutdownTracing(shutdownCtx); shutdownErr != nil {
			logger.Warn("gateway tracing shutdown", "err", shutdownErr)
		}
	}()

	container, cleanup, err := bootstrap.NewGateway(ctx)
	if err != nil {
		return fmt.Errorf("gateway bootstrap: %w", err)
	}
	defer cleanup()

	handler, err := gateway.NewHandler(container)
	if err != nil {
		return fmt.Errorf("gateway handler: %w", err)
	}
	srv := &http.Server{
		Addr:              container.Config.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	gateway.EnableHTTP2(srv)

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("gateway listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	matchmakerCtx, stopMatchmaker := context.WithCancel(ctx)
	defer stopMatchmaker()
	go func() {
		// Only the leader replica runs the matchmaker loop; the rest stand
		// by. Correctness across a brief leadership overlap is guaranteed
		// by the queue's atomic Claim, so this is a work-dedup optimisation.
		err := leaderlock.Run(matchmakerCtx, container.Valkey, leaderlock.Config{
			Key:    "vfx:matchmaker:leader",
			TTL:    container.Config.MatchmakerLeaderTTL,
			Logger: logger,
		}, container.Matchmaker.Run)
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("matchmaker exited", "err", err)
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("gateway shutting down")
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("gateway listen: %w", err)
		}
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("gateway shutdown: %w", err)
	}
	return nil
}
