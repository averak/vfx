package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/averak/vfx/internal/domain/player"
	"github.com/averak/vfx/internal/infra/dbgen"
)

// RefreshToken is the storage implementation of [player.RefreshTokenRepository].
type RefreshToken struct{}

var _ player.RefreshTokenRepository = (*RefreshToken)(nil)

// NewRefreshToken returns a refresh-token repository ready to use.
func NewRefreshToken() *RefreshToken {
	return &RefreshToken{}
}

func (RefreshToken) Create(ctx context.Context, tx pgx.Tx, rt *player.RefreshToken) error {
	_, err := dbgen.New(tx).CreateRefreshToken(ctx, dbgen.CreateRefreshTokenParams{
		ID:        rt.ID,
		PlayerID:  rt.PlayerID,
		TokenHash: rt.Hash,
		ExpiresAt: toTimestamptz(rt.ExpiresAt),
		CreatedAt: toTimestamptz(rt.CreatedAt),
	})
	return err
}

func (RefreshToken) FindByHash(ctx context.Context, tx pgx.Tx, hash []byte, now time.Time) (*player.RefreshToken, error) {
	row, err := dbgen.New(tx).FindRefreshTokenByHash(ctx, dbgen.FindRefreshTokenByHashParams{
		TokenHash: hash,
		ExpiresAt: toTimestamptz(now),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, player.ErrRefreshTokenInvalid
		}
		return nil, err
	}
	return &player.RefreshToken{
		ID:        row.ID,
		PlayerID:  row.PlayerID,
		Hash:      row.TokenHash,
		ExpiresAt: row.ExpiresAt.Time,
		RevokedAt: fromNullableTimestamptz(row.RevokedAt),
		CreatedAt: row.CreatedAt.Time,
	}, nil
}

func (RefreshToken) Revoke(ctx context.Context, tx pgx.Tx, id uuid.UUID, now time.Time) error {
	return dbgen.New(tx).RevokeRefreshToken(ctx, dbgen.RevokeRefreshTokenParams{
		ID:        id,
		RevokedAt: toTimestamptz(now),
	})
}

func (RefreshToken) RevokeAllForPlayer(ctx context.Context, tx pgx.Tx, playerID uuid.UUID, now time.Time) error {
	return dbgen.New(tx).RevokeAllRefreshTokensForPlayer(ctx, dbgen.RevokeAllRefreshTokensForPlayerParams{
		PlayerID:  playerID,
		RevokedAt: toTimestamptz(now),
	})
}
