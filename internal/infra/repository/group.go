package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/averak/vfx/internal/domain/group"
	"github.com/averak/vfx/internal/infra/db"
	"github.com/averak/vfx/internal/infra/dbgen"
)

type Group struct{}

var _ group.Repository = (*Group)(nil)

func NewGroup() *Group {
	return &Group{}
}

func (Group) Find(ctx context.Context, id uuid.UUID) (*group.Group, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	row, err := dbgen.New(tx).GetGroup(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, group.ErrNotFound
		}
		return nil, err
	}
	return &group.Group{ID: row.ID, Name: row.Name, OwnerID: row.OwnerID, FoundedAt: row.FoundedAt.Time}, nil
}

func (Group) Save(ctx context.Context, g *group.Group) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	return dbgen.New(tx).UpsertGroup(ctx, dbgen.UpsertGroupParams{
		ID:        g.ID,
		Name:      g.Name,
		OwnerID:   g.OwnerID,
		FoundedAt: toTimestamptz(g.FoundedAt),
	})
}

func (Group) Delete(ctx context.Context, id uuid.UUID) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	return dbgen.New(tx).DeleteGroup(ctx, id)
}
