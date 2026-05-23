// Package token deals with access tokens and refresh tokens.
//
// Access tokens are JWTs signed with HMAC-SHA256 using a shared secret.
// Both the gateway (issuer) and the room (verifier) read the same secret from VFX_JWT_SECRET.
// A future iteration can switch to an asymmetric algorithm (EdDSA) without changing call sites.
//
// Refresh tokens are 32 random bytes encoded as hex.
// Only the SHA-256 of the raw token is stored, so a database leak does not directly enable impersonation; the raw value is returned to the client and never persisted.
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type AccessClaims struct {
	PlayerID uuid.UUID `json:"sub"`
	jwt.RegisteredClaims
}

type SessionClaims struct {
	PlayerID uuid.UUID `json:"sub"`
	MatchID  string    `json:"mid"`

	// MatchPlayers is the full roster the gateway paired together for this match.
	// The room daemon reads it from the first joining player to call plugin.Init, so plugins that require an exact player count work even when joins arrive one by one.
	MatchPlayers []string `json:"mps,omitempty"`

	jwt.RegisteredClaims
}

type Signer struct {
	secret []byte
}

func NewSigner(secret string) *Signer {
	return &Signer{secret: []byte(secret)}
}

func (s *Signer) SignAccess(playerID uuid.UUID, now time.Time, ttl time.Duration) (string, error) {
	claims := AccessClaims{
		PlayerID: playerID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("token: sign: %w", err)
	}
	return signed, nil
}

// SignSession issues a token the room daemon trusts because it shares the signing secret with the gateway.
func (s *Signer) SignSession(playerID uuid.UUID, matchID string, matchPlayers []string, now time.Time, ttl time.Duration) (string, error) {
	claims := SessionClaims{
		PlayerID:     playerID,
		MatchID:      matchID,
		MatchPlayers: matchPlayers,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("token: sign session: %w", err)
	}
	return signed, nil
}

func (s *Signer) VerifySession(tokenStr string) (*SessionClaims, error) {
	claims := &SessionClaims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %q", t.Method.Alg())
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("token: verify session: %w", err)
	}
	return claims, nil
}

func (s *Signer) Verify(tokenStr string) (*AccessClaims, error) {
	claims := &AccessClaims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %q", t.Method.Alg())
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("token: verify: %w", err)
	}
	return claims, nil
}

type Refresh struct {
	Raw  string // sent to the client, never persisted
	Hash []byte // stored in the refresh_tokens table
}

func NewRefresh() (*Refresh, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("token: random: %w", err)
	}
	raw := hex.EncodeToString(buf)
	return &Refresh{Raw: raw, Hash: HashRefresh(raw)}, nil
}

func HashRefresh(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

// NewRefresh is the method form that lets *Signer satisfy the TokenIssuer port, which works in primitives rather than the Refresh type.
func (s *Signer) NewRefresh() (raw string, hash []byte, err error) {
	r, err := NewRefresh()
	if err != nil {
		return "", nil, err
	}
	return r.Raw, r.Hash, nil
}

// HashRefresh is the method form of the package function, so *Signer satisfies the TokenIssuer port.
func (s *Signer) HashRefresh(raw string) []byte {
	return HashRefresh(raw)
}
