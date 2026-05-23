package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/averak/vfx/internal/domain/player"
	"github.com/averak/vfx/internal/infra/db"
	"github.com/averak/vfx/internal/infra/dbgen"
)

// Player is the storage implementation of [player.Repository].
//
// Methods are stateless: each pulls the active transaction from the context (placed there by db.Session) and wraps it in a fresh dbgen.Queries, so one instance is safe across goroutines and transactions.
type Player struct{}

var _ player.Repository = (*Player)(nil)

func NewPlayer() *Player {
	return &Player{}
}

func (Player) GetByID(ctx context.Context, id uuid.UUID) (*player.Player, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	row, err := dbgen.New(tx).GetPlayerByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, player.ErrPlayerNotFound
		}
		return nil, err
	}
	return playerFromRow(row), nil
}

func (Player) Save(ctx context.Context, p *player.Player) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	_, err = dbgen.New(tx).CreatePlayer(ctx, dbgen.CreatePlayerParams{
		ID:        p.ID,
		Nickname:  p.Nickname,
		CreatedAt: toTimestamptz(p.CreatedAt),
		UpdatedAt: toTimestamptz(p.UpdatedAt),
	})
	return err
}

func (Player) UpdateNickname(ctx context.Context, p *player.Player) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	_, err = dbgen.New(tx).UpdatePlayerNickname(ctx, dbgen.UpdatePlayerNicknameParams{
		ID:        p.ID,
		Nickname:  p.Nickname,
		UpdatedAt: toTimestamptz(p.UpdatedAt),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return player.ErrPlayerNotFound
		}
		return err
	}
	return nil
}

func (Player) FindPlayerByIdentity(ctx context.Context, provider player.Provider, providerUID string) (*player.Player, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	row, err := dbgen.New(tx).FindPlayerByIdentity(ctx, dbgen.FindPlayerByIdentityParams{
		Provider:    string(provider),
		ProviderUid: providerUID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, player.ErrIdentityNotFound
		}
		return nil, err
	}
	return playerFromRow(row), nil
}

func (Player) SaveIdentity(ctx context.Context, i *player.Identity) error {
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
	return err
}

func playerFromRow(row dbgen.Player) *player.Player {
	return &player.Player{
		ID:        row.ID,
		Nickname:  row.Nickname,
		CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
	}
}
