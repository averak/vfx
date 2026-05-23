// Package main is the example binary that registers the RPS plugin Factory before calling cli.NewRootCmd, so its room command can host rock-paper-scissors.
// It is otherwise the vanilla vfx binary (cmd/vfx).
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
