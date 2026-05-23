package match

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrTicketNotFound = errors.New("match: ticket not found")

type Queue interface {
	Enqueue(ctx context.Context, t *Ticket) error
	Cancel(ctx context.Context, ticketID uuid.UUID) error
	// Subscribe's channel closes on a terminal event (Matched or Failed) or when ctx is cancelled.
	Subscribe(ctx context.Context, ticketID uuid.UUID) (<-chan Event, error)
	// Pending returns waiting tickets oldest first.
	Pending(ctx context.Context, gameMode string) ([]*Ticket, error)
	// Claim atomically removes the tickets from the pending pool, returning true only if every one was still pending.
	// If a matchmaker on another replica already took one, it claims none and returns false.
	// This is what makes matchmaking safe to run on more than one gateway.
	Claim(ctx context.Context, gameMode string, ticketIDs []uuid.UUID) (bool, error)
	Publish(ctx context.Context, ticketID uuid.UUID, event Event) error
	Depth(ctx context.Context, gameMode string) (int32, error)
}
