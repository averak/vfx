// Package token deals with access tokens and refresh tokens.
//
// Access tokens are JWTs signed with HMAC-SHA256 using a shared secret.
// Both the gateway (issuer) and the room (verifier) read the same secret
// from VFX_JWT_SECRET. A future iteration can switch to an asymmetric
// algorithm (EdDSA) without changing call sites.
//
// Refresh tokens are 32 random bytes encoded as hex. Only the SHA-256
// of the raw token is stored, so a database leak does not directly
// enable impersonation; the raw value is returned to the client and
// never persisted.
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

// AccessClaims are the claims embedded in every issued access token.
type AccessClaims struct {
	PlayerID uuid.UUID `json:"sub"`
	jwt.RegisteredClaims
}

// Signer issues and verifies access tokens.
type Signer struct {
	secret []byte
}

// NewSigner returns a Signer using the given HMAC secret.
func NewSigner(secret string) *Signer {
	return &Signer{secret: []byte(secret)}
}

// SignAccess issues a fresh access token for the given player.
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

// Verify parses and validates an access token string. It returns the
// embedded claims when the signature, algorithm, and expiry all check
// out.
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

// Refresh holds both halves of a freshly minted refresh token.
type Refresh struct {
	// Raw is the value sent to the client. It is never persisted.
	Raw string
	// Hash is what gets stored in the refresh_tokens table.
	Hash []byte
}

// NewRefresh generates a cryptographically random refresh token.
func NewRefresh() (*Refresh, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("token: random: %w", err)
	}
	raw := hex.EncodeToString(buf)
	return &Refresh{Raw: raw, Hash: HashRefresh(raw)}, nil
}

// HashRefresh returns the SHA-256 of the raw token. It is the form
// stored in the database and looked up on refresh.
func HashRefresh(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}
