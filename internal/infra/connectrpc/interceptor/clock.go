// Package interceptor holds Connect-RPC interceptors used by the
// gateway.
//
// Each interceptor here addresses a single concern. ClockInterceptor
// stamps the request boundary with a fixed "now"; AuthInterceptor
// verifies the Authorization header and attaches the player id to ctx.
package interceptor

import (
	"context"
	"time"

	"connectrpc.com/connect"

	"github.com/averak/vfx/internal/stdx/clock"
)

// Clock returns an interceptor that attaches time.Now (UTC) to the
// request context so downstream layers can call clock.Now without
// reaching for time.Now directly.
func Clock() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			ctx = clock.With(ctx, time.Now().UTC())
			return next(ctx, req)
		}
	}
}
