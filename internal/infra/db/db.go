// Package db provides a thin transaction-runner over a pgx pool.
//
// A Session runs work inside a transaction and places that transaction on the context the work receives; repository implementations pull it back out with Tx(ctx).
// This keeps the transaction off the domain's repository interfaces entirely (they take only context), so the domain never imports pgx, and a usecase cannot reach for the pool directly and bypass transactional consistency.
package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNoTx is returned by Tx when called outside a Session transaction.
var ErrNoTx = errors.New("db: no transaction in context")

type txKey struct{}

type Session struct {
	pool *pgxpool.Pool
}

func NewSession(pool *pgxpool.Pool) *Session {
	return &Session{pool: pool}
}

// RW commits if fn returns nil, otherwise rolls back; fn's error is returned as-is so domain errors stay distinguishable.
func (s *Session) RW(ctx context.Context, fn func(context.Context) error) error {
	return s.run(ctx, pgx.TxOptions{}, fn)
}

// RO runs fn inside a read-only transaction, the single seam where future replica routing can hook in.
func (s *Session) RO(ctx context.Context, fn func(context.Context) error) error {
	return s.run(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}, fn)
}

func (s *Session) run(ctx context.Context, opts pgx.TxOptions, fn func(context.Context) error) error {
	tx, err := s.pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("db: begin tx: %w", err)
	}
	// Rollback after a successful Commit is a no-op that returns pgx.ErrTxClosed; either way the error carries no signal.
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // Rollback after Commit returns ErrTxClosed; nothing actionable.

	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("db: commit: %w", err)
	}
	return nil
}

// Tx returns the transaction a Session placed on ctx, for repository implementations to use.
// It returns ErrNoTx when used outside RW/RO, which signals a programming error: a repository called without a surrounding transaction.
func Tx(ctx context.Context) (pgx.Tx, error) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	if !ok {
		return nil, ErrNoTx
	}
	return tx, nil
}
