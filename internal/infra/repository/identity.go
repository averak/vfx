package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/averak/vfx/internal/domain/player"
	"github.com/averak/vfx/internal/infra/db"
	"github.com/averak/vfx/internal/infra/dbgen"
)

type Identity struct{}

var _ player.IdentityRepository = (*Identity)(nil)

func NewIdentity() *Identity {
	return &Identity{}
}

func (Identity) Find(ctx context.Context, provider player.Provider, providerUID string) (*player.Identity, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	row, err := dbgen.New(tx).FindIdentity(ctx, dbgen.FindIdentityParams{
		Provider:    string(provider),
		ProviderUid: providerUID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, player.ErrIdentityNotFound
		}
		return nil, err
	}
	return &player.Identity{
		ID:          row.ID,
		PlayerID:    row.PlayerID,
		Provider:    player.Provider(row.Provider),
		ProviderUID: row.ProviderUid,
		CreatedAt:   row.CreatedAt.Time,
	}, nil
}

func (Identity) Save(ctx context.Context, i *player.Identity) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	_, err = dbgen.New(tx).CreatePlayerIdentity(ctx, dbgen.CreatePlayerIdentityParams{
		ID:          i.ID,
		PlayerID:    i.PlayerID,
		Provider:    string(i.Provider),
		ProviderUid: i.ProviderUID,
		CreatedAt:   toTimestamptz(i.CreatedAt),
	})
	// The unique (provider, provider_uid) index turns a concurrent link into this violation; surface the domain error so the caller can resolve the race (retry login and find the winner, or reject a relink).
	if isUniqueViolation(err) {
		return player.ErrIdentityAlreadyLinked
	}
	return err
}
