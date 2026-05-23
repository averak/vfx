// Package cli builds the vfx cobra command tree.
//
// Splitting command construction out of main lets two binaries share the same surface: the vanilla cmd/vfx ships with no plugins registered (gateway/admin/migrate still work, `vfx room` errors asking for a plugin), and each example under examples/<game>/cmd/ builds a custom binary that registers its plugin before calling NewRootCmd.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/averak/vfx/internal/domain/plugin"
)

// Version is overridden at link time via -ldflags "-X ..."
var Version = "0.0.0-dev"

func NewRootCmd(registry *plugin.Registry) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "vfx",
		Short:         "WebTransport-native, WASM-driven game server engine",
		Long:          "vfx hosts realtime multiplayer games using WebTransport for transport and WebAssembly for game logic.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(
		newGatewayCmd(),
		newRoomCmd(registry),
		newAdminCmd(),
		newMigrateCmd(),
	)
	return cmd
}
