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
	"player_files",
	"title_files",
	"match_players",
	"matches",
	"refresh_tokens",
	"player_identities",
	"players",
}

// testDBLockKey is an arbitrary fixed key for the session advisory lock that serializes DB-backed tests; see Pool.
const testDBLockKey = 0x7666_7864_6200 // "vfxdb"

// Pool returns a connection pool to the test database, or skips the test when DATABASE_URL is not configured.
//
// `go test ./...` runs each package's test binary in parallel, and every DB-backed test wipes the shared tables with Truncate.
// To stop one package's truncate from deleting another's rows mid-test, Pool takes a session-scoped Postgres advisory lock held for the whole test (released on cleanup), so DB-backed tests serialize across packages and processes while pure-logic suites still run fully in parallel.
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
	if err = pool.Ping(t.Context()); err != nil {
		pool.Close()
		t.Fatalf("testdb: ping: %v", err)
	}
	t.Cleanup(pool.Close)

	// The lock lives on a dedicated connection held for the test; the session-scoped lock is auto-released by Postgres if the process dies, so a crash cannot wedge the suite.
	// Background (not t.Context) on purpose: the unlock runs from a cleanup, after t.Context is already cancelled.
	lockConn, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("testdb: acquire lock conn: %v", err)
	}
	if _, err := lockConn.Exec(context.Background(), "SELECT pg_advisory_lock($1)", testDBLockKey); err != nil {
		lockConn.Release()
		t.Fatalf("testdb: advisory lock: %v", err)
	}
	t.Cleanup(func() {
		//nolint:errcheck // Best-effort unlock; the session-scoped lock is released anyway when the connection closes.
		_, _ = lockConn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", testDBLockKey)
		lockConn.Release()
	})

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
