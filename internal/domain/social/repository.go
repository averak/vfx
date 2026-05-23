package social

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository persists friend requests and friendships.
// Friendship methods take the pair in any order and canonicalize internally; the usecase need not pre-order.
type Repository interface {
	// CreateRequest records a pending request requester -> addressee.
	CreateRequest(ctx context.Context, requester, addressee uuid.UUID, now time.Time) error

	// RequestExists reports whether a pending request requester -> addressee exists.
	RequestExists(ctx context.Context, requester, addressee uuid.UUID) (bool, error)

	// DeleteRequest removes the pending request requester -> addressee, returning ErrRequestNotFound when there is none.
	DeleteRequest(ctx context.Context, requester, addressee uuid.UUID) error

	ListIncoming(ctx context.Context, addressee uuid.UUID) ([]*PendingRequest, error)
	ListOutgoing(ctx context.Context, requester uuid.UUID) ([]*PendingRequest, error)

	CreateFriendship(ctx context.Context, a, b uuid.UUID, now time.Time) error
	AreFriends(ctx context.Context, a, b uuid.UUID) (bool, error)

	// DeleteFriendship removes the friendship, returning ErrNotFriends when the two are not friends.
	DeleteFriendship(ctx context.Context, a, b uuid.UUID) error

	ListFriends(ctx context.Context, playerID uuid.UUID) ([]*Friend, error)

	// Block records blocker -> blocked; it is idempotent (re-blocking is a no-op).
	Block(ctx context.Context, blocker, blocked uuid.UUID, now time.Time) error

	// Unblock removes blocker -> blocked; it is idempotent (unblocking a non-block is a no-op).
	Unblock(ctx context.Context, blocker, blocked uuid.UUID) error

	// IsBlocked reports whether either player has blocked the other.
	IsBlocked(ctx context.Context, a, b uuid.UUID) (bool, error)

	ListBlocked(ctx context.Context, blocker uuid.UUID) ([]*BlockedPlayer, error)
}
