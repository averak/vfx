package cli_test

import (
	"testing"

	"github.com/averak/vfx/internal/cli"
	"github.com/averak/vfx/internal/domain/plugin"
)

func TestNewRootCmd_MountsEverySubcommand(t *testing.T) {
	root := cli.NewRootCmd(plugin.NewRegistry())
	if root.Use != "vfx" {
		t.Errorf("Use = %q, want vfx", root.Use)
	}

	want := map[string]bool{"gateway": false, "room": false, "admin": false, "migrate": false}
	for _, c := range root.Commands() {
		if _, ok := want[c.Name()]; ok {
			want[c.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("subcommand %q missing from the root command", name)
		}
	}
}
