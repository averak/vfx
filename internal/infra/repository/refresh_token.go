package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/averak/vfx/internal/domain/player"
	"github.com/averak/vfx/internal/infra/db"
	"github.com/averak/vfx/internal/infra/dbgen"
)

// RefreshToken is the storage implementation of [player.RefreshTokenRepository].
type RefreshToken struct{}

var _ player.RefreshTokenRepository = (*RefreshToken)(nil)

func NewRefreshToken() *RefreshToken {
	return &RefreshToken{}
}

func (RefreshToken) Create(ctx context.Context, rt *player.RefreshToken) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	_, err = dbgen.New(tx).CreateRefreshToken(ctx, dbgen.CreateRefreshTokenParams{
		ID:        rt.ID,
		PlayerID:  rt.PlayerID,
		TokenHash: rt.Hash,
		ExpiresAt: toTimestamptz(rt.ExpiresAt),
		CreatedAt: toTimestamptz(rt.CreatedAt),
	})
	return err
}

func (RefreshToken) FindByHash(ctx context.Context, hash []byte, now time.Time) (*player.RefreshToken, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return nil, err
	}
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

func (RefreshToken) Revoke(ctx context.Context, id uuid.UUID, now time.Time) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	affected, err := dbgen.New(tx).RevokeRefreshToken(ctx, dbgen.RevokeRefreshTokenParams{
		ID:        id,
		RevokedAt: toTimestamptz(now),
	})
	if err != nil {
		return err
	}
	// No row revoked means it was already revoked, which under concurrent refresh of the same token is the loser of the race.
	if affected == 0 {
		return player.ErrRefreshTokenInvalid
	}
	return nil
}

func (RefreshToken) RevokeAllForPlayer(ctx context.Context, playerID uuid.UUID, now time.Time) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	return dbgen.New(tx).RevokeAllRefreshTokensForPlayer(ctx, dbgen.RevokeAllRefreshTokensForPlayerParams{
		PlayerID:  playerID,
		RevokedAt: toTimestamptz(now),
	})
}
