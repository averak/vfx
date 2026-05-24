package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/group"
	"github.com/averak/vfx/internal/infra/db"
	"github.com/averak/vfx/internal/infra/dbgen"
)

type GroupMembership struct{}

var _ group.MembershipRepository = (*GroupMembership)(nil)

func NewGroupMembership() *GroupMembership {
	return &GroupMembership{}
}

func (GroupMembership) Save(ctx context.Context, m *group.Membership) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	// ON CONFLICT (group_id, player_id) DO NOTHING makes the join idempotent; the surrogate id is an infra detail outside the aggregate's identity.
	return dbgen.New(tx).AddGroupMember(ctx, dbgen.AddGroupMemberParams{
		ID:       uuid.New(),
		GroupID:  m.GroupID,
		PlayerID: m.PlayerID,
		JoinedAt: toTimestamptz(m.JoinedAt),
	})
}

func (GroupMembership) Delete(ctx context.Context, groupID, playerID uuid.UUID) error {
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

func (GroupMembership) IsMember(ctx context.Context, groupID, playerID uuid.UUID) (bool, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return false, err
	}
	return dbgen.New(tx).GroupMemberExists(ctx, dbgen.GroupMemberExistsParams{GroupID: groupID, PlayerID: playerID})
}

func (GroupMembership) ListMembers(ctx context.Context, groupID uuid.UUID) ([]*group.Member, error) {
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

func (GroupMembership) ListGroupsForPlayer(ctx context.Context, playerID uuid.UUID) ([]*group.Group, error) {
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
		out[i] = &group.Group{ID: row.ID, Name: row.Name, OwnerID: row.OwnerID, FoundedAt: row.FoundedAt.Time}
	}
	return out, nil
}
