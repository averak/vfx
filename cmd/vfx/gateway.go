package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
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

func runGateway(_ context.Context) error {
	// TODO: implement in task #4
	//   - load config (env-driven)
	//   - connect to PostgreSQL and Valkey
	//   - wire DI container
	//   - register Connect RPC handlers (auth, match)
	//   - start matchmaker worker
	//   - serve HTTP/2
	fmt.Println("gateway: not yet implemented")
	return nil
}
