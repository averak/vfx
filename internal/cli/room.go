package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/averak/vfx/internal/bootstrap"
	"github.com/averak/vfx/internal/domain/plugin"
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

	container, cleanup, err := bootstrap.NewRoom(ctx, registry, logger)
	if err != nil {
		return fmt.Errorf("room bootstrap: %w", err)
	}
	defer cleanup()

	return container.Server.ListenAndServe(ctx)
}
