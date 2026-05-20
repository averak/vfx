// Package gateway assembles the gateway HTTP handler.
//
// The server exposes every Connect-RPC service mounted under
// /vfx.v1.<service>/<method>, plus a /healthz endpoint for orchestrators.
// Unencrypted HTTP/2 is enabled at the [http.Server] level so local
// development and in-cluster communication work without TLS, while TLS
// is added by the deployment layer when needed.
package gateway

import (
	"net/http"

	"connectrpc.com/connect"

	"github.com/averak/vfx/gen/go/vfx/v1/auth/authconnect"
	"github.com/averak/vfx/gen/go/vfx/v1/match/matchconnect"
	"github.com/averak/vfx/internal/bootstrap"
	"github.com/averak/vfx/internal/infra/connectrpc/interceptor"
)

// NewHandler returns the root HTTP handler for the gateway process.
func NewHandler(c *bootstrap.Gateway) http.Handler {
	mux := http.NewServeMux()

	interceptors := connect.WithInterceptors(
		interceptor.Clock(),
		interceptor.Auth(c.Signer),
	)

	authPath, authHandler := authconnect.NewAuthServiceHandler(c.AuthHandler, interceptors)
	mux.Handle(authPath, authHandler)

	matchPath, matchHandler := matchconnect.NewMatchServiceHandler(c.MatchHandler, interceptors)
	mux.Handle(matchPath, matchHandler)

	// Liveness/readiness for orchestrators. A more thorough readiness
	// check (DB ping, Valkey ping) belongs in a later observability pass.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return mux
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
