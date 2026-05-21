package token_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/infra/token"
)

func TestSigner_AccessRoundTrip(t *testing.T) {
	signer := token.NewSigner("test-secret")
	playerID := uuid.New()
	now := time.Now()

	tok, err := signer.SignAccess(playerID, now, time.Hour)
	if err != nil {
		t.Fatalf("SignAccess: %v", err)
	}
	claims, err := signer.Verify(tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.PlayerID != playerID {
		t.Errorf("PlayerID = %v, want %v", claims.PlayerID, playerID)
	}
}

func TestSigner_RejectsExpiredAccessToken(t *testing.T) {
	signer := token.NewSigner("test-secret")
	past := time.Now().Add(-2 * time.Hour)

	tok, err := signer.SignAccess(uuid.New(), past, time.Hour) // expired an hour ago
	if err != nil {
		t.Fatalf("SignAccess: %v", err)
	}
	if _, err := signer.Verify(tok); err == nil {
		t.Error("Verify accepted an expired token")
	}
}

func TestSigner_RejectsForeignSecret(t *testing.T) {
	issuer := token.NewSigner("issuer-secret")
	attacker := token.NewSigner("different-secret")

	tok, err := issuer.SignAccess(uuid.New(), time.Now(), time.Hour)
	if err != nil {
		t.Fatalf("SignAccess: %v", err)
	}
	if _, err := attacker.Verify(tok); err == nil {
		t.Error("Verify accepted a token signed with a different secret")
	}
}

func TestSigner_SessionRoundTripCarriesRoster(t *testing.T) {
	signer := token.NewSigner("test-secret")
	playerID := uuid.New()
	matchID := uuid.NewString()
	roster := []string{playerID.String(), uuid.NewString()}

	tok, err := signer.SignSession(playerID, matchID, roster, time.Now(), time.Minute)
	if err != nil {
		t.Fatalf("SignSession: %v", err)
	}
	claims, err := signer.VerifySession(tok)
	if err != nil {
		t.Fatalf("VerifySession: %v", err)
	}
	if claims.PlayerID != playerID {
		t.Errorf("PlayerID = %v, want %v", claims.PlayerID, playerID)
	}
	if claims.MatchID != matchID {
		t.Errorf("MatchID = %q, want %q", claims.MatchID, matchID)
	}
	if len(claims.MatchPlayers) != len(roster) {
		t.Errorf("MatchPlayers = %v, want %v", claims.MatchPlayers, roster)
	}
}

func TestRefresh_HashIsStableAndOpaque(t *testing.T) {
	r, err := token.NewRefresh()
	if err != nil {
		t.Fatalf("NewRefresh: %v", err)
	}
	if r.Raw == "" {
		t.Fatal("raw refresh token is empty")
	}
	// The stored hash must match a fresh hash of the raw value...
	rehash := token.HashRefresh(r.Raw)
	if !bytes.Equal(rehash, r.Hash) {
		t.Error("HashRefresh(raw) does not match the stored hash")
	}
	// ...and must not be the raw value itself.
	if string(r.Hash) == r.Raw {
		t.Error("stored hash equals the raw token")
	}
}

func TestRefresh_DistinctTokensEachCall(t *testing.T) {
	a, err := token.NewRefresh()
	if err != nil {
		t.Fatalf("NewRefresh: %v", err)
	}
	b, err := token.NewRefresh()
	if err != nil {
		t.Fatalf("NewRefresh: %v", err)
	}
	if a.Raw == b.Raw {
		t.Error("two refresh tokens collided")
	}
}
