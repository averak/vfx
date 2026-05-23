package token_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"pgregory.net/rapid"

	"github.com/averak/vfx/internal/infra/token"
)

func drawUUID(t *rapid.T, label string) uuid.UUID {
	raw := rapid.SliceOfN(rapid.Byte(), 16, 16).Draw(t, label)
	var id uuid.UUID
	copy(id[:], raw)
	return id
}

// Any player id and any positive TTL must survive a sign/verify round
// trip unchanged. The token is opaque, but its subject claim is not.
func TestSigner_AccessRoundTripProperty(t *testing.T) {
	signer := token.NewSigner("property-secret")
	rapid.Check(t, func(t *rapid.T) {
		id := drawUUID(t, "player")
		ttl := time.Duration(rapid.IntRange(1, 86_400).Draw(t, "ttlSeconds")) * time.Second

		tok, err := signer.SignAccess(id, time.Now(), ttl)
		if err != nil {
			t.Fatalf("SignAccess: %v", err)
		}
		claims, err := signer.Verify(tok)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if claims.PlayerID != id {
			t.Fatalf("round trip changed player id: got %v, want %v", claims.PlayerID, id)
		}
	})
}

// A session token must carry the player, the match id, and the full
// roster across a round trip for any roster size.
func TestSigner_SessionRoundTripProperty(t *testing.T) {
	signer := token.NewSigner("property-secret")
	rapid.Check(t, func(t *rapid.T) {
		player := drawUUID(t, "player")
		matchID := drawUUID(t, "match").String()
		rosterSize := rapid.IntRange(0, 8).Draw(t, "rosterSize")
		roster := make([]string, rosterSize)
		for i := range roster {
			roster[i] = drawUUID(t, "rosterMember").String()
		}

		tok, err := signer.SignSession(player, matchID, roster, time.Now(), time.Minute)
		if err != nil {
			t.Fatalf("SignSession: %v", err)
		}
		claims, err := signer.VerifySession(tok)
		if err != nil {
			t.Fatalf("VerifySession: %v", err)
		}
		if claims.PlayerID != player {
			t.Fatalf("player id changed: got %v, want %v", claims.PlayerID, player)
		}
		if claims.MatchID != matchID {
			t.Fatalf("match id changed: got %q, want %q", claims.MatchID, matchID)
		}
		if len(claims.MatchPlayers) != len(roster) {
			t.Fatalf("roster size changed: got %d, want %d", len(claims.MatchPlayers), len(roster))
		}
		for i := range roster {
			if claims.MatchPlayers[i] != roster[i] {
				t.Fatalf("roster member %d changed: got %q, want %q", i, claims.MatchPlayers[i], roster[i])
			}
		}
	})
}
