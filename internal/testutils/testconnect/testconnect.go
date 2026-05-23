// Package testconnect spins up an in-process Connect-RPC server wired with the real gateway handlers and interceptors, for component tests that exercise the HTTP/RPC boundary.
//
// It deliberately wires the handlers by hand rather than calling bootstrap.NewGateway, because bootstrap also dials Valkey and reads process config, which a handler test should not depend on.
// The connect protocol streams over HTTP/1.1, so a plain httptest server is enough for both unary and server-streaming RPCs.
package testconnect

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/averak/vfx/gen/go/vfx/v1/auth/authconnect"
	"github.com/averak/vfx/gen/go/vfx/v1/match/matchconnect"
	"github.com/averak/vfx/internal/infra/assignmentstore"
	"github.com/averak/vfx/internal/infra/connectrpc/interceptor"
	"github.com/averak/vfx/internal/infra/db"
	"github.com/averak/vfx/internal/infra/matchqueue"
	"github.com/averak/vfx/internal/infra/repository"
	"github.com/averak/vfx/internal/infra/token"
	gatewayauthhandler "github.com/averak/vfx/internal/presentation/gateway/auth"
	gatewaymatchhandler "github.com/averak/vfx/internal/presentation/gateway/match"
	"github.com/averak/vfx/internal/testutils/testdb"
	usecaseauth "github.com/averak/vfx/internal/usecase/auth"
	usecasematch "github.com/averak/vfx/internal/usecase/match"
)

const jwtSecret = "test-secret"

type Server struct {
	Auth  authconnect.AuthServiceClient
	Match matchconnect.MatchServiceClient

	httpServer *httptest.Server
}

// New wires the gateway handlers against a clean test database and starts an httptest server, torn down via t.Cleanup.
// Tests that do not set DATABASE_URL are skipped (see testdb.Pool).
func New(t *testing.T) *Server {
	t.Helper()

	pool := testdb.Pool(t)
	signer := token.NewSigner(jwtSecret)

	authUC := usecaseauth.New(
		db.NewSession(pool),
		repository.NewPlayer(),
		repository.NewRefreshToken(),
		signer,
		15*time.Minute,
		720*time.Hour,
	)
	matchUC := usecasematch.New(matchqueue.NewInMem(), assignmentstore.NewInMem())

	interceptors := connect.WithInterceptors(
		interceptor.Clock(),
		interceptor.Auth(signer),
	)

	mux := http.NewServeMux()
	authPath, authHandler := authconnect.NewAuthServiceHandler(gatewayauthhandler.New(authUC), interceptors)
	mux.Handle(authPath, authHandler)
	matchPath, matchHandler := matchconnect.NewMatchServiceHandler(gatewaymatchhandler.New(matchUC), interceptors)
	mux.Handle(matchPath, matchHandler)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &Server{
		Auth:       authconnect.NewAuthServiceClient(srv.Client(), srv.URL),
		Match:      matchconnect.NewMatchServiceClient(srv.Client(), srv.URL),
		httpServer: srv,
	}
}

func Authorize[T any](req *connect.Request[T], accessToken string) *connect.Request[T] {
	req.Header().Set("Authorization", "Bearer "+accessToken)
	return req
}
