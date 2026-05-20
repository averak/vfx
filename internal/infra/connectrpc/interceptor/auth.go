package interceptor

import (
	"context"
	"strings"

	"connectrpc.com/connect"

	"github.com/averak/vfx/internal/infra/connectrpc/authctx"
	"github.com/averak/vfx/internal/infra/token"
)

const (
	authorizationHeader = "Authorization"
	bearerPrefix        = "Bearer "
)

// Auth returns an interceptor that verifies a bearer access token when
// one is supplied and attaches the player id to the context. Missing
// or invalid tokens are not rejected here; the handler decides whether
// it requires authentication and reads from authctx accordingly.
//
// This deliberate softness means Login/Refresh keep working without an
// Authorization header, while Logout/UpdateProfile can opt into a hard
// check via authctx.From.
func Auth(signer *token.Signer) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			header := req.Header().Get(authorizationHeader)
			if header == "" {
				return next(ctx, req)
			}
			rawToken, ok := strings.CutPrefix(header, bearerPrefix)
			if !ok {
				return next(ctx, req)
			}
			claims, err := signer.Verify(rawToken)
			if err != nil {
				return next(ctx, req)
			}
			ctx = authctx.With(ctx, claims.PlayerID)
			return next(ctx, req)
		}
	}
}
