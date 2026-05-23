// Package main is the vanilla vfx binary entry point.
//
// It registers no plugins, so `vfx gateway`, `vfx admin`, and `vfx migrate` work out of the box, but `vfx room` reports that no plugin is available.
// To host a game, build a custom binary that calls cli.NewRootCmd with the plugin Factory registered (see the example under examples/<game>/cmd/).
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
