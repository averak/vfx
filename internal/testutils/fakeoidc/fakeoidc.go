// Package fakeoidc is an in-memory [usecaseauth.OIDCVerifier] for tests.
//
// It accepts any non-empty token except the literal "invalid", treating the token as the identity so the same token always maps to the same player; "invalid" (or empty) is rejected like a bad signature.
package fakeoidc

import (
	"context"
	"errors"

	"github.com/averak/vfx/internal/domain/player"
	usecaseauth "github.com/averak/vfx/internal/usecase/auth"
)

type Verifier struct{}

var _ usecaseauth.OIDCVerifier = (*Verifier)(nil)

func New() *Verifier {
	return &Verifier{}
}

func (Verifier) Verify(_ context.Context, provider player.Provider, idToken string) (*usecaseauth.VerifiedIdentity, error) {
	if idToken == "" || idToken == "invalid" {
		return nil, errors.New("fakeoidc: rejected token")
	}
	// Subject is derived from provider+token so the same token reproduces the same identity, and the two providers never collide.
	return &usecaseauth.VerifiedIdentity{Subject: string(provider) + ":" + idToken}, nil
}
