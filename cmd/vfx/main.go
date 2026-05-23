// Package main is the vanilla vfx entry point.
// It registers no plugins, so to host a game you build a binary that registers a plugin Factory before calling cli.NewRootCmd (see examples/<game>/cmd/).
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/averak/vfx/internal/cli"
	"github.com/averak/vfx/internal/domain/plugin"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "vfx: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	return cli.NewRootCmd(plugin.NewRegistry()).ExecuteContext(ctx)
}
