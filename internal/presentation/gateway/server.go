// Package gateway assembles the gateway HTTP handler.
//
// The server exposes every Connect-RPC service mounted under
// /vfx.v1.<service>/<method>, plus a /healthz endpoint for orchestrators.
// Unencrypted HTTP/2 is enabled at the [http.Server] level so local
// development and in-cluster communication work without TLS, while TLS
// is added by the deployment layer when needed.
package gateway

import (
	"fmt"
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"

	"github.com/averak/vfx/gen/go/vfx/v1/auth/authconnect"
	"github.com/averak/vfx/gen/go/vfx/v1/match/matchconnect"
	"github.com/averak/vfx/internal/bootstrap"
	"github.com/averak/vfx/internal/infra/connectrpc/interceptor"
)

// NewHandler returns the root HTTP handler for the gateway process.
func NewHandler(c *bootstrap.Gateway) (http.Handler, error) {
	mux := http.NewServeMux()

	// Tracing comes from the global tracer provider installed by
	// tracing.Setup; with no OTLP endpoint configured that provider is a
	// no-op, so this interceptor is effectively free. Metrics stay on the
	// vfx-owned Prometheus path, so otelconnect is asked for spans only.
	otelInterceptor, err := otelconnect.NewInterceptor(otelconnect.WithoutMetrics())
	if err != nil {
		return nil, fmt.Errorf("gateway: otel interceptor: %w", err)
	}

	// The tracing interceptor is outermost so its span wraps the auth,
	// clock, and metrics work done by the inner interceptors.
	interceptors := connect.WithInterceptors(
		otelInterceptor,
		interceptor.Metrics(c.Metrics),
		interceptor.Clock(),
		interceptor.Auth(c.Signer),
	)

	authPath, authHandler := authconnect.NewAuthServiceHandler(c.AuthHandler, interceptors)
	mux.Handle(authPath, authHandler)

	matchPath, matchHandler := matchconnect.NewMatchServiceHandler(c.MatchHandler, interceptors)
	mux.Handle(matchPath, matchHandler)

	// Liveness probe: the process can answer HTTP, that's all this
	// check guarantees. Used by Kubernetes to decide when to restart.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Readiness probe: the gateway's dependencies are reachable. A
	// failing readyz takes the pod out of the Service backend without
	// restarting it.
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := c.Pool.Ping(r.Context()); err != nil {
			http.Error(w, "postgres unreachable: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// Prometheus exposition. The registry is vfx-owned so we never
	// leak metrics from transitive dependencies we did not vet.
	mux.Handle("/metrics", c.Metrics.Handler())

	return mux, nil
}

// EnableHTTP2 turns on HTTP/2 (both encrypted and unencrypted) on the
// given server. Clients negotiate HTTP/2 over cleartext when the
// connection is plain HTTP, which is what Connect-RPC and gRPC clients
// expect from an internal gateway.
func EnableHTTP2(srv *http.Server) {
	if srv.Protocols == nil {
		srv.Protocols = &http.Protocols{}
	}
	srv.Protocols.SetHTTP1(true)
	srv.Protocols.SetHTTP2(true)
	srv.Protocols.SetUnencryptedHTTP2(true)
}
