package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newRoomCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "room",
		Short: "Run a single match-hosting room",
		Long: "Starts a WebTransport server that hosts exactly one match, " +
			"running the configured WASM plugin via wazero. The room registers " +
			"with Agones (Ready → Allocated → Shutdown) for lifecycle management.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRoom(cmd.Context())
		},
	}
	return cmd
}

func runRoom(_ context.Context) error {
	// TODO: implement in task #5
	//   - load config (env-driven)
	//   - load WASM plugin via wazero
	//   - integrate Agones SDK (Ready/Health/Shutdown)
	//   - start WebTransport server
	//   - run tick loop
	fmt.Println("room: not yet implemented")
	return nil
}
