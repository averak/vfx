// Package main is the entry point for the vfx binary.
//
// vfx is dispatched via subcommands:
//   - vfx gateway: stateless API server (auth, matchmaking, storage, admin RPCs)
//   - vfx room:    stateful match host (WebTransport + WASM plugin)
//   - vfx admin:   operations API (and later UI)
//   - vfx migrate: thin wrapper around atlas for database migrations
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

// version is overridden at link time via -ldflags "-X main.version=...".
var version = "0.0.0-dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "vfx: %v\n", err)
		os.Exit(1)
	}
}

// run keeps signal-handling cleanup outside the os.Exit path: deferred
// cancellation in main itself would never fire when we exit with a non-zero
// code on error.
func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	return newRootCmd().ExecuteContext(ctx)
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "vfx",
		Short:         "WebTransport-native, WASM-driven game server engine",
		Long:          "vfx hosts realtime multiplayer games using WebTransport for transport and WebAssembly for game logic.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(
		newGatewayCmd(),
		newRoomCmd(),
		newAdminCmd(),
		newMigrateCmd(),
	)
	return cmd
}
