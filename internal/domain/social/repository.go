package social

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Friendship and block methods take the pair in any order and canonicalize internally; the caller need not pre-order.
type Repository interface {
	CreateRequest(ctx context.Context, requester, addressee uuid.UUID, now time.Time) error
	RequestExists(ctx context.Context, requester, addressee uuid.UUID) (bool, error)

	// DeleteRequest returns ErrRequestNotFound when no such pending request exists.
	DeleteRequest(ctx context.Context, requester, addressee uuid.UUID) error

	ListIncoming(ctx context.Context, addressee uuid.UUID) ([]*PendingRequest, error)
	ListOutgoing(ctx context.Context, requester uuid.UUID) ([]*PendingRequest, error)

	CreateFriendship(ctx context.Context, a, b uuid.UUID, now time.Time) error
	AreFriends(ctx context.Context, a, b uuid.UUID) (bool, error)

	// DeleteFriendship returns ErrNotFriends when the two are not friends.
	DeleteFriendship(ctx context.Context, a, b uuid.UUID) error

	ListFriends(ctx context.Context, playerID uuid.UUID) ([]*Friend, error)

	// Block is idempotent: re-blocking is a no-op.
	Block(ctx context.Context, blocker, blocked uuid.UUID, now time.Time) error

	// Unblock is idempotent: unblocking a non-block is a no-op.
	Unblock(ctx context.Context, blocker, blocked uuid.UUID) error

	// IsBlocked reports whether either player has blocked the other, in either direction.
	IsBlocked(ctx context.Context, a, b uuid.UUID) (bool, error)

	ListBlocked(ctx context.Context, blocker uuid.UUID) ([]*BlockedPlayer, error)
}
