package player

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrRefreshTokenInvalid is returned when a refresh token is unknown,
// expired, or revoked. Callers must not surface the distinction to
// users: any of these means "log in again".
var ErrRefreshTokenInvalid = errors.New("player: refresh token invalid")

// RefreshToken is the persistent half of a refresh token. Hash is
// SHA-256 of the raw token; the raw value is never stored.
type RefreshToken struct {
	ID        uuid.UUID
	PlayerID  uuid.UUID
	Hash      []byte
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

// IsActive reports whether the token can be used to mint a new access
// token at the given moment.
func (rt *RefreshToken) IsActive(now time.Time) bool {
	if rt.RevokedAt != nil {
		return false
	}
	return rt.ExpiresAt.After(now)
}

// RefreshTokenRepository persists RefreshToken rows. Like Repository, it
// takes only a context; the transaction is carried there by the usecase.
type RefreshTokenRepository interface {
	Create(ctx context.Context, rt *RefreshToken) error
	FindByHash(ctx context.Context, hash []byte, now time.Time) (*RefreshToken, error)
	Revoke(ctx context.Context, id uuid.UUID, now time.Time) error
	RevokeAllForPlayer(ctx context.Context, playerID uuid.UUID, now time.Time) error
}
