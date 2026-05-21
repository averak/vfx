package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

const (
	// defaultMigrationsDir points at where the Docker image stores the
	// migrations (see Dockerfile), so `vfx migrate` works out of the box
	// in a container. Running it from a source checkout needs
	// --dir file://schema/db/migrations (or just use `mise run db-migrate`).
	defaultMigrationsDir = "file:///etc/vfx/migrations"
	atlasBinary          = "atlas"
)

func newMigrateCmd() *cobra.Command {
	var (
		migrationsDir string
		databaseURL   string
	)

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Apply database migrations",
		Long: "Thin wrapper around `atlas migrate`. Subcommands map directly to " +
			"the corresponding atlas commands and forward stdout/stderr.",
	}

	cmd.PersistentFlags().StringVar(&migrationsDir, "dir", defaultMigrationsDir, "Migrations directory URL (atlas format)")
	cmd.PersistentFlags().StringVar(&databaseURL, "url", os.Getenv("DATABASE_URL"), "Database URL (overrides DATABASE_URL env)")

	cmd.AddCommand(
		newMigrateApplyCmd(&migrationsDir, &databaseURL),
		newMigrateStatusCmd(&migrationsDir, &databaseURL),
		newMigrateDownCmd(&migrationsDir, &databaseURL),
	)

	return cmd
}

func newMigrateApplyCmd(dir, url *string) *cobra.Command {
	return &cobra.Command{
		Use:   "apply",
		Short: "Apply pending migrations",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAtlas(cmd.Context(), "migrate", "apply", "--dir", *dir, "--url", *url)
		},
	}
}

func newMigrateStatusCmd(dir, url *string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show migration status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAtlas(cmd.Context(), "migrate", "status", "--dir", *dir, "--url", *url)
		},
	}
}

func newMigrateDownCmd(dir, url *string) *cobra.Command {
	var amount int
	cmd := &cobra.Command{
		Use:   "down",
		Short: "Revert the most recent migrations",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAtlas(cmd.Context(), "migrate", "down", "--dir", *dir, "--url", *url,
				"--amount", fmt.Sprintf("%d", amount))
		},
	}
	cmd.Flags().IntVar(&amount, "amount", 1, "Number of migrations to revert")
	return cmd
}

func runAtlas(ctx context.Context, args ...string) error {
	// atlasBinary is a fixed constant and args originate from cobra's parsed
	// flag values, not raw user input, so the variable invocation is safe.
	//nolint:gosec // G204: trusted subprocess invocation.
	c := exec.CommandContext(ctx, atlasBinary, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Stdin = os.Stdin
	if err := c.Run(); err != nil {
		return fmt.Errorf("atlas %s: %w", args[0], err)
	}
	return nil
}
