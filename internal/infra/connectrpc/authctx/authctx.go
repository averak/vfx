// Package authctx propagates the authenticated player id through a
// request context. Interceptors attach the id once at the request
// boundary; handlers read it back when they need to know "who is this".
package authctx

import (
	"context"

	"github.com/google/uuid"
)

type ctxKey struct{}

// With returns a context carrying the given authenticated player id.
func With(ctx context.Context, playerID uuid.UUID) context.Context {
	return context.WithValue(ctx, ctxKey{}, playerID)
}

// From returns the authenticated player id and ok=true when one is
// present, or uuid.Nil and ok=false otherwise.
func From(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(ctxKey{}).(uuid.UUID)
	return id, ok
}
