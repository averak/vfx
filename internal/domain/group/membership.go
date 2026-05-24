package group

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Membership is its own aggregate root, keyed by (GroupID, PlayerID), so joining or leaving never loads the whole group — which matters once a group has many members.
type Membership struct {
	GroupID  uuid.UUID
	PlayerID uuid.UUID
	JoinedAt time.Time
}

func NewMembership(groupID, playerID uuid.UUID, now time.Time) *Membership {
	return &Membership{GroupID: groupID, PlayerID: playerID, JoinedAt: now}
}

type MembershipRepository interface {
	// Save is idempotent: re-joining is a no-op.
	Save(ctx context.Context, m *Membership) error

	// Delete returns ErrNotMember when the player is not in the group.
	Delete(ctx context.Context, groupID, playerID uuid.UUID) error

	IsMember(ctx context.Context, groupID, playerID uuid.UUID) (bool, error)

	ListMembers(ctx context.Context, groupID uuid.UUID) ([]*Member, error)
	ListGroupsForPlayer(ctx context.Context, playerID uuid.UUID) ([]*Group, error)
}
