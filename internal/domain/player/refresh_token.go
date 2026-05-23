package player

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrRefreshTokenInvalid covers unknown, expired, and revoked tokens alike.
// Callers must not surface the distinction: any of these means "log in again".
var ErrRefreshTokenInvalid = errors.New("player: refresh token invalid")

type RefreshToken struct {
	ID        uuid.UUID
	PlayerID  uuid.UUID
	Hash      []byte // SHA-256 of the raw token; the raw value is never stored
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

func (rt *RefreshToken) IsActive(now time.Time) bool {
	if rt.RevokedAt != nil {
		return false
	}
	return rt.ExpiresAt.After(now)
}

type RefreshTokenRepository interface {
	Create(ctx context.Context, rt *RefreshToken) error
	FindByHash(ctx context.Context, hash []byte, now time.Time) (*RefreshToken, error)
	Revoke(ctx context.Context, id uuid.UUID, now time.Time) error
	RevokeAllForPlayer(ctx context.Context, playerID uuid.UUID, now time.Time) error
}
