// Package db provides a thin transaction-runner over a pgx pool.
//
// The Session type is the single entry point repository implementations
// see: they always receive a [pgx.Tx] whether the caller asked for a
// read-write transaction or a read-only one. That removes the temptation
// to use the pool directly from a usecase and bypass transactional
// consistency.
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Session wraps a pgx pool and exposes transaction lifecycle helpers.
type Session struct {
	pool *pgxpool.Pool
}

// NewSession returns a Session backed by the given pool.
func NewSession(pool *pgxpool.Pool) *Session {
	return &Session{pool: pool}
}

// RW runs fn inside a read-write transaction. The transaction is
// committed if fn returns nil, otherwise rolled back. Errors from fn
// are returned as-is so domain errors stay distinguishable.
func (s *Session) RW(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: begin tx: %w", err)
	}
	// Rollback after a successful Commit is a no-op that returns
	// pgx.ErrTxClosed; either way the error carries no signal.
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // Rollback after Commit returns ErrTxClosed; nothing actionable.

	if err := fn(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("db: commit: %w", err)
	}
	return nil
}

// RO runs fn inside a read-only transaction. Useful for read paths so
// that a connection is acquired predictably and any future replica
// routing can hook in at this single seam.
func (s *Session) RO(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("db: begin ro tx: %w", err)
	}
	// Rollback after a successful Commit is a no-op that returns
	// pgx.ErrTxClosed; either way the error carries no signal.
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // Rollback after Commit returns ErrTxClosed; nothing actionable.

	if err := fn(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("db: commit: %w", err)
	}
	return nil
}
