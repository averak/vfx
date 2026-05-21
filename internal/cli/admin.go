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
)

func newAdminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Run the operations API",
		Long: "Starts the admin API that exposes operational endpoints for " +
			"inspecting rooms, players, and plugin deployments. Intended to run " +
			"behind a separate auth boundary from the player-facing gateway.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAdmin(cmd.Context())
		},
	}
	return cmd
}

func runAdmin(ctx context.Context) error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	container, cleanup, err := bootstrap.NewAdmin(ctx)
	if err != nil {
		return fmt.Errorf("admin bootstrap: %w", err)
	}
	defer cleanup()

	srv := &http.Server{
		Addr:              container.Config.ListenAddr,
		Handler:           container.Handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("admin listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case <-ctx.Done():
		logger.Info("admin shutting down")
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("admin listen: %w", err)
		}
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("admin shutdown: %w", err)
	}
	return nil
}
