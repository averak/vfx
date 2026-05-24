package group

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	// Find returns ErrNotFound when no group has that id.
	Find(ctx context.Context, id uuid.UUID) (*Group, error)
	Save(ctx context.Context, g *Group) error

	// Delete cascades to the group's memberships.
	Delete(ctx context.Context, id uuid.UUID) error
}
