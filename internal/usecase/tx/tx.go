// Package tx defines the transaction-boundary ports shared across usecases.
//
// The boundary belongs to the usecase, but "run work in a transaction" is a generic capability, so it lives here once instead of being redeclared in every usecase.
// Splitting it into Reader and ReadWriter keeps read-only callers from depending on RW they never use; the infra implementation places the transaction on the context the repositories read from.
package tx

import "context"

// ReadWriter runs fn inside a read-write transaction.
type ReadWriter interface {
	RW(ctx context.Context, fn func(context.Context) error) error
}

// Reader runs fn inside a read-only transaction.
type Reader interface {
	RO(ctx context.Context, fn func(context.Context) error) error
}
