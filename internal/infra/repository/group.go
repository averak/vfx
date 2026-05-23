package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/averak/vfx/internal/domain/group"
	"github.com/averak/vfx/internal/infra/db"
	"github.com/averak/vfx/internal/infra/dbgen"
)

// Group is the storage implementation of [group.Repository].
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
	return &group.Group{ID: row.ID, Name: row.Name, OwnerID: row.OwnerID, CreatedAt: row.CreatedAt.Time}, nil
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
		CreatedAt: toTimestamptz(g.CreatedAt),
	})
}

func (Group) Delete(ctx context.Context, id uuid.UUID) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	return dbgen.New(tx).DeleteGroup(ctx, id)
}

func (Group) AddMember(ctx context.Context, groupID, playerID uuid.UUID, now time.Time) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	return dbgen.New(tx).AddGroupMember(ctx, dbgen.AddGroupMemberParams{
		ID:       uuid.New(),
		GroupID:  groupID,
		PlayerID: playerID,
		JoinedAt: toTimestamptz(now),
	})
}

func (Group) RemoveMember(ctx context.Context, groupID, playerID uuid.UUID) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	affected, err := dbgen.New(tx).RemoveGroupMember(ctx, dbgen.RemoveGroupMemberParams{GroupID: groupID, PlayerID: playerID})
	if err != nil {
		return err
	}
	if affected == 0 {
		return group.ErrNotMember
	}
	return nil
}

func (Group) IsMember(ctx context.Context, groupID, playerID uuid.UUID) (bool, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return false, err
	}
	return dbgen.New(tx).GroupMemberExists(ctx, dbgen.GroupMemberExistsParams{GroupID: groupID, PlayerID: playerID})
}

func (Group) ListForPlayer(ctx context.Context, playerID uuid.UUID) ([]*group.Group, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := dbgen.New(tx).ListGroupsForPlayer(ctx, playerID)
	if err != nil {
		return nil, err
	}
	out := make([]*group.Group, len(rows))
	for i, row := range rows {
		out[i] = &group.Group{ID: row.ID, Name: row.Name, OwnerID: row.OwnerID, CreatedAt: row.CreatedAt.Time}
	}
	return out, nil
}

func (Group) ListMembers(ctx context.Context, groupID uuid.UUID) ([]*group.Member, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := dbgen.New(tx).ListGroupMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}
	out := make([]*group.Member, len(rows))
	for i, row := range rows {
		out[i] = &group.Member{PlayerID: row.ID, Nickname: row.Nickname, JoinedAt: row.JoinedAt.Time}
	}
	return out, nil
}
