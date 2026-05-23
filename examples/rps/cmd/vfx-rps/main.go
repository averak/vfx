// Package main is the example binary that ships the RPS plugin.
//
// It is identical to cmd/vfx except for one line: rps.NewFactory is registered with the plugin registry before the cobra root command is built.
// `vfx-rps room` therefore boots a WebTransport server hosting rock-paper-scissors matches, while the other subcommands (gateway/admin/migrate) behave exactly like the vanilla binary.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	rps "github.com/averak/vfx/examples/rps/plugin"
	"github.com/averak/vfx/internal/cli"
	"github.com/averak/vfx/internal/domain/plugin"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "vfx-rps: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	registry := plugin.NewRegistry()
	if err := registry.Register(rps.NewFactory()); err != nil {
		return fmt.Errorf("register rps plugin: %w", err)
	}
	return cli.NewRootCmd(registry).ExecuteContext(ctx)
}
