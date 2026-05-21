package interceptor

import (
	"context"
	"time"

	"connectrpc.com/connect"

	"github.com/averak/vfx/internal/infra/metrics"
)

// Metrics returns an interceptor that records a request count (labelled
// by method and result code) and a latency histogram for every unary
// RPC. Streaming RPCs are passed through untimed — their lifetime is
// dominated by how long the client stays subscribed, which is not a
// useful latency signal.
func Metrics(reg *metrics.Registry) connect.Interceptor {
	return &metricsInterceptor{reg: reg}
}

type metricsInterceptor struct {
	reg *metrics.Registry
}

func (m *metricsInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		start := time.Now()
		resp, err := next(ctx, req)
		method := req.Spec().Procedure
		m.reg.RPCDuration.WithLabelValues(method).Observe(time.Since(start).Seconds())
		m.reg.RPCRequests.WithLabelValues(method, codeLabel(err)).Inc()
		return resp, err
	}
}

func (m *metricsInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (m *metricsInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		err := next(ctx, conn)
		m.reg.RPCRequests.WithLabelValues(conn.Spec().Procedure, codeLabel(err)).Inc()
		return err
	}
}

// codeLabel maps a handler error to a metric label, reporting success
// as "ok" rather than connect's zero code.
func codeLabel(err error) string {
	if err == nil {
		return "ok"
	}
	return connect.CodeOf(err).String()
}
