// Package oidc verifies provider ID tokens for the auth usecase.
//
// It implements [usecaseauth.OIDCVerifier] over go-oidc, validating each token's signature, issuer, audience, and expiry against the provider's JWKS.
// A remote key set fetches and caches the JWKS lazily, so no network call or OIDC discovery happens at startup.
package oidc

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/averak/vfx/internal/domain/player"
	usecaseauth "github.com/averak/vfx/internal/usecase/auth"
)

type endpoint struct {
	issuer  string
	jwksURL string
}

// providerEndpoints pins each provider's issuer and JWKS URL, so verification needs no discovery round-trip.
var providerEndpoints = map[player.Provider]endpoint{
	player.ProviderGoogle: {issuer: "https://accounts.google.com", jwksURL: "https://www.googleapis.com/oauth2/v3/certs"},
	player.ProviderApple:  {issuer: "https://appleid.apple.com", jwksURL: "https://appleid.apple.com/auth/keys"},
}

// Config maps a provider to the audience (client id) its tokens must carry; a provider absent here is unconfigured and rejected.
type Config map[player.Provider]string

type Verifier struct {
	verifiers map[player.Provider]*oidc.IDTokenVerifier
}

var _ usecaseauth.OIDCVerifier = (*Verifier)(nil)

// New builds a verifier for the providers in cfg with a non-empty audience; ctx scopes the lazy JWKS key sets' background refresh.
func New(ctx context.Context, cfg Config) *Verifier {
	verifiers := make(map[player.Provider]*oidc.IDTokenVerifier, len(cfg))
	for provider, audience := range cfg {
		ep, ok := providerEndpoints[provider]
		if !ok || audience == "" {
			continue
		}
		keySet := oidc.NewRemoteKeySet(ctx, ep.jwksURL)
		verifiers[provider] = oidc.NewVerifier(ep.issuer, keySet, &oidc.Config{ClientID: audience})
	}
	return &Verifier{verifiers: verifiers}
}

func (v *Verifier) Verify(ctx context.Context, provider player.Provider, idToken string) (*usecaseauth.VerifiedIdentity, error) {
	verifier, ok := v.verifiers[provider]
	if !ok {
		return nil, fmt.Errorf("oidc: provider %q is not configured", provider)
	}
	tok, err := verifier.Verify(ctx, idToken)
	if err != nil {
		return nil, fmt.Errorf("oidc: verify %s token: %w", provider, err)
	}
	var claims struct {
		Name string `json:"name"`
	}
	if err := tok.Claims(&claims); err != nil {
		// The display name is optional (Apple often omits it); an extraction failure just leaves the player unnamed.
		claims.Name = ""
	}
	return &usecaseauth.VerifiedIdentity{Subject: tok.Subject, Name: claims.Name}, nil
}
