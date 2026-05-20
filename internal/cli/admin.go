package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
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

func runAdmin(_ context.Context) error {
	fmt.Println("admin: not yet implemented")
	return nil
}
