package match

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Assignment is the matchmaker's output: which match the player ended
// up in and how to connect to it.
type Assignment struct {
	MatchID      uuid.UUID
	Endpoint     string
	SessionToken string
	ExpiresAt    time.Time
}

// AssignmentStore persists a player's current match assignment so it can
// be re-read after a reconnect or from a different gateway replica. It
// is distinct from the Queue because an assignment outlives the ticket
// that produced it: the ticket's event stream closes once Matched is
// delivered, but the player may reconnect to GetCurrentMatch later.
type AssignmentStore interface {
	// Put records the player's assignment, replacing any previous one,
	// with a TTL after which it is forgotten. The TTL should track the
	// session token lifetime, since a stale assignment is useless once
	// its token can no longer authenticate to the room.
	Put(ctx context.Context, playerID uuid.UUID, a *Assignment, ttl time.Duration) error

	// Get returns the player's current assignment, or (nil, nil) when
	// there is none recorded (or it has expired).
	Get(ctx context.Context, playerID uuid.UUID) (*Assignment, error)
}
