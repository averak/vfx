package interceptor

import (
	"context"
	"net/http"
	"strings"

	"connectrpc.com/connect"

	"github.com/averak/vfx/internal/infra/connectrpc/authctx"
	"github.com/averak/vfx/internal/infra/token"
)

const (
	authorizationHeader = "Authorization"
	bearerPrefix        = "Bearer "
)

// Auth returns an interceptor that verifies a bearer access token when one is supplied and attaches the player id to the context.
// Missing or invalid tokens are not rejected here; the handler decides whether it requires authentication and reads from authctx accordingly.
//
// This deliberate softness lets Login/Refresh work without an Authorization header, while Logout/UpdateProfile/Match.* opt into a hard check via authctx.From.
//
// The interceptor implements the full [connect.Interceptor], so it applies to both unary and streaming RPCs (WatchTicket and similar).
func Auth(signer *token.Signer) connect.Interceptor {
	return &authInterceptor{signer: signer}
}

type authInterceptor struct {
	signer *token.Signer
}

func (a *authInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		ctx = a.populate(ctx, req.Header())
		return next(ctx, req)
	}
}

func (a *authInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (a *authInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		ctx = a.populate(ctx, conn.RequestHeader())
		return next(ctx, conn)
	}
}

func (a *authInterceptor) populate(ctx context.Context, header http.Header) context.Context {
	raw := header.Get(authorizationHeader)
	if raw == "" {
		return ctx
	}
	rawToken, ok := strings.CutPrefix(raw, bearerPrefix)
	if !ok {
		return ctx
	}
	claims, err := a.signer.Verify(rawToken)
	if err != nil {
		return ctx
	}
	return authctx.With(ctx, claims.PlayerID)
}
