package group

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	// Find returns ErrNotFound when no group has that id.
	Find(ctx context.Context, id uuid.UUID) (*Group, error)
	Save(ctx context.Context, g *Group) error

	// Delete cascades to the group's memberships.
	Delete(ctx context.Context, id uuid.UUID) error

	// AddMember is idempotent: re-joining is a no-op.
	AddMember(ctx context.Context, groupID, playerID uuid.UUID, now time.Time) error

	// RemoveMember returns ErrNotMember when the player is not in the group.
	RemoveMember(ctx context.Context, groupID, playerID uuid.UUID) error

	IsMember(ctx context.Context, groupID, playerID uuid.UUID) (bool, error)

	ListForPlayer(ctx context.Context, playerID uuid.UUID) ([]*Group, error)
	ListMembers(ctx context.Context, groupID uuid.UUID) ([]*Member, error)
}
