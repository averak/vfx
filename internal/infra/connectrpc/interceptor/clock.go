// Package interceptor holds the gateway's Connect-RPC interceptors, each addressing a single concern.
package interceptor

import (
	"context"
	"time"

	"connectrpc.com/connect"

	"github.com/averak/vfx/internal/stdx/clock"
)

// Clock attaches time.Now (UTC) to the request context so downstream layers call clock.Now instead of time.Now directly.
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
