package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/averak/vfx/internal/infra/config"
	"github.com/averak/vfx/internal/infra/token"
	"github.com/averak/vfx/internal/presentation/room"
)

func newRoomCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "room",
		Short: "Run the match-hosting WebTransport server",
		Long: "Starts a WebTransport server that hosts matches via the configured " +
			"WASM plugin. Session tokens issued by the gateway grant connection rights.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRoom(cmd.Context())
		},
	}
	return cmd
}

func runRoom(ctx context.Context) error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.LoadRoom()
	if err != nil {
		return fmt.Errorf("room: %w", err)
	}

	signer := token.NewSigner(cfg.JWTSecret)
	srv, err := room.NewServer(cfg, signer, logger)
	if err != nil {
		return fmt.Errorf("room: %w", err)
	}
	return srv.ListenAndServe(ctx)
}
