// Package clock provides a context-scoped "current time" so the same instant can be threaded through every layer of a single request.
//
// At the request boundary (an HTTP middleware, a stream handler, a job runner, etc.) the operator calls With to attach a [time.Time] to the context.
// Downstream code that needs the operation's "now" then calls Now, instead of [time.Now], to obtain it.
//
// This keeps two invariants:
//
//   - Multiple statements in one request all see the same "now", so a row's created_at matches the started_at of the surrounding match even if the request takes hundreds of milliseconds.
//   - Tests can freeze the clock by attaching a fixed time, making assertions about timestamps fully deterministic.
//
// Code that never had a time attached (a stray goroutine, a test that forgot to set one) falls back to [time.Now] rather than panicking, so the absence of middleware never crashes a process; it just degrades the determinism guarantee.
package clock

import (
	"context"
	"time"
)

type ctxKey struct{}

// With returns a new context carrying t as the operation's "now".
// It is intended to be called exactly once per request, at the boundary where the operation begins.
func With(ctx context.Context, t time.Time) context.Context {
	return context.WithValue(ctx, ctxKey{}, t)
}

// Now returns the time previously attached with With.
// If no time was attached, it falls back to [time.Now] so callers never panic on a missing middleware.
func Now(ctx context.Context) time.Time {
	if t, ok := ctx.Value(ctxKey{}).(time.Time); ok {
		return t
	}
	return time.Now()
}
