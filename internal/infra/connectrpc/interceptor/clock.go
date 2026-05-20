// Package interceptor holds Connect-RPC interceptors used by the
// gateway.
//
// Each interceptor here addresses a single concern. The Clock
// interceptor stamps the request boundary with a fixed "now"; the Auth
// interceptor verifies the Authorization header and attaches the
// player id to ctx.
package interceptor

import (
	"context"
	"time"

	"connectrpc.com/connect"

	"github.com/averak/vfx/internal/stdx/clock"
)

// Clock returns an interceptor that attaches time.Now (UTC) to the
// request context so downstream layers can call clock.Now without
// reaching for time.Now directly. The interceptor applies to both
// unary and streaming RPCs.
func Clock() connect.Interceptor {
	return &clockInterceptor{}
}

type clockInterceptor struct{}

func (c *clockInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		return next(c.attach(ctx), req)
	}
}

func (c *clockInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (c *clockInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return next(c.attach(ctx), conn)
	}
}

func (*clockInterceptor) attach(ctx context.Context) context.Context {
	return clock.With(ctx, time.Now().UTC())
}
