package social

import (
	"context"

	"github.com/google/uuid"
)

type FriendRequestRepository interface {
	Save(ctx context.Context, r *FriendRequest) error

	// Delete returns ErrRequestNotFound when no such pending request exists.
	Delete(ctx context.Context, requester, addressee uuid.UUID) error

	Exists(ctx context.Context, requester, addressee uuid.UUID) (bool, error)

	ListIncoming(ctx context.Context, addressee uuid.UUID) ([]*PendingRequest, error)
	ListOutgoing(ctx context.Context, requester uuid.UUID) ([]*PendingRequest, error)
}

// FriendshipRepository methods take the pair in any order and canonicalize internally.
type FriendshipRepository interface {
	Save(ctx context.Context, f *Friendship) error

	// Delete returns ErrNotFriends when the two are not friends.
	Delete(ctx context.Context, a, b uuid.UUID) error

	Exists(ctx context.Context, a, b uuid.UUID) (bool, error)
	ListFriends(ctx context.Context, playerID uuid.UUID) ([]*Friend, error)
}

type BlockRepository interface {
	// Save is idempotent: re-blocking is a no-op.
	Save(ctx context.Context, b *Block) error

	// Delete is idempotent: unblocking a non-block is a no-op.
	Delete(ctx context.Context, blocker, blocked uuid.UUID) error

	// IsBlocked reports whether either player has blocked the other, in either direction.
	IsBlocked(ctx context.Context, a, b uuid.UUID) (bool, error)

	ListBlocked(ctx context.Context, blocker uuid.UUID) ([]*BlockedPlayer, error)
}
