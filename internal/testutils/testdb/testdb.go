// Package testdb provides a PostgreSQL pool for tests that exercise the repository and usecase layers against a real database.
//
// Tests call Pool(t); if DATABASE_URL is unset the test is skipped, so a machine without a database still passes the pure-logic suites.
// CI sets DATABASE_URL to the compose-managed PostgreSQL, so the integration tests run there.
package testdb

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// tables are truncated by Truncate, child-first so foreign keys allow it.
var tables = []string{
	"match_players",
	"matches",
	"refresh_tokens",
	"player_identities",
	"players",
}

// Pool returns a connection pool to the test database, or skips the test when DATABASE_URL is not configured.
// The pool is closed via t.Cleanup.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping database-backed test")
	}

	pool, err := pgxpool.New(t.Context(), url)
	if err != nil {
		t.Fatalf("testdb: open pool: %v", err)
	}
	if err := pool.Ping(t.Context()); err != nil {
		pool.Close()
		t.Fatalf("testdb: ping: %v", err)
	}
	t.Cleanup(pool.Close)

	Truncate(t, pool)
	return pool
}

// Truncate empties every vfx table so each test starts from a clean slate.
// It is registered to run again on cleanup.
func Truncate(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	truncate := func() {
		for _, table := range tables {
			if _, err := pool.Exec(context.Background(), "DELETE FROM "+table); err != nil {
				t.Fatalf("testdb: truncate %s: %v", table, err)
			}
		}
	}
	truncate()
	t.Cleanup(truncate)
}
