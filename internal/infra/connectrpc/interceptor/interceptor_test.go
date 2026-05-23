package interceptor_test

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	authv1 "github.com/averak/vfx/gen/go/vfx/v1/auth"
	"github.com/averak/vfx/internal/infra/connectrpc/authctx"
	"github.com/averak/vfx/internal/infra/connectrpc/interceptor"
	"github.com/averak/vfx/internal/infra/token"
	"github.com/averak/vfx/internal/stdx/clock"
)

// recorder returns a terminal UnaryFunc and a getter for the context it
// was called with, so a test can inspect what an interceptor attached
// without passing a pointer around.
func recorder() (next connect.UnaryFunc, seenCtx func() context.Context) {
	var seen context.Context
	next = func(ctx context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		seen = ctx
		return connect.NewResponse(&authv1.LoginResponse{}), nil
	}
	return next, func() context.Context { return seen }
}

func TestClockInterceptor_AttachesUTCNow(t *testing.T) {
	next, seen := recorder()
	wrapped := interceptor.Clock().WrapUnary(next)
	if _, err := wrapped(t.Context(), connect.NewRequest(&authv1.LoginRequest{})); err != nil {
		t.Fatalf("wrapped: %v", err)
	}

	// A bare context's clock.Now falls back to local time.Now; the
	// interceptor attaches a fixed UTC instant, so the location proves it
	// was set rather than defaulted.
	now := clock.Now(seen())
	if now.Location() != time.UTC {
		t.Errorf("clock.Now location = %v, want UTC", now.Location())
	}
	if time.Since(now) > time.Minute {
		t.Errorf("attached now is implausible: %v", now)
	}
}

func TestAuthInterceptor_PopulatesFromValidBearer(t *testing.T) {
	signer := token.NewSigner("interceptor-secret")
	playerID := uuid.New()
	tok, err := signer.SignAccess(playerID, time.Now(), time.Hour)
	if err != nil {
		t.Fatalf("SignAccess: %v", err)
	}

	next, seen := recorder()
	wrapped := interceptor.Auth(signer).WrapUnary(next)
	req := connect.NewRequest(&authv1.LoginRequest{})
	req.Header().Set("Authorization", "Bearer "+tok)
	if _, err := wrapped(t.Context(), req); err != nil {
		t.Fatalf("wrapped: %v", err)
	}

	id, ok := authctx.From(seen())
	if !ok {
		t.Fatal("auth interceptor did not attach a player id for a valid token")
	}
	if id != playerID {
		t.Errorf("attached id = %v, want %v", id, playerID)
	}
}

// Missing or invalid credentials are not rejected by the interceptor; the
// request proceeds with no player id, and the handler decides what to do.
func TestAuthInterceptor_SoftFailsWithoutCredentials(t *testing.T) {
	signer := token.NewSigner("interceptor-secret")

	tests := []struct {
		name   string
		header string
	}{
		{"no header", ""},
		{"not a bearer", "Basic abc"},
		{"garbage token", "Bearer not-a-jwt"},
		{"foreign token", "Bearer " + foreignToken(t)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next, seen := recorder()
			wrapped := interceptor.Auth(signer).WrapUnary(next)
			req := connect.NewRequest(&authv1.LoginRequest{})
			if tt.header != "" {
				req.Header().Set("Authorization", tt.header)
			}
			if _, err := wrapped(t.Context(), req); err != nil {
				t.Fatalf("interceptor rejected the request instead of passing it through: %v", err)
			}
			if _, ok := authctx.From(seen()); ok {
				t.Error("a player id was attached despite missing/invalid credentials")
			}
		})
	}
}

func foreignToken(t *testing.T) string {
	t.Helper()
	tok, err := token.NewSigner("a-different-secret").SignAccess(uuid.New(), time.Now(), time.Hour)
	if err != nil {
		t.Fatalf("SignAccess: %v", err)
	}
	return tok
}
