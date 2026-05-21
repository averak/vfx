package match

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ErrTicketNotFound is returned when a queue lookup misses.
var ErrTicketNotFound = errors.New("match: ticket not found")

// Queue persists tickets and the per-ticket Event stream that
// WatchTicket consumers read from. The contract is intentionally
// implementation-neutral so a future swap from Valkey to something
// else does not touch the usecase layer.
type Queue interface {
	// Enqueue stores a ticket and seeds its event stream with a Queued
	// event so a subscriber attached even later still sees it.
	Enqueue(ctx context.Context, t *Ticket) error

	// Cancel removes the ticket and publishes a Failed event so any
	// active WatchTicket subscribers terminate cleanly.
	Cancel(ctx context.Context, ticketID uuid.UUID) error

	// Subscribe returns a channel of Events for the given ticket. The
	// channel is closed once a terminal event (Matched or Failed) is
	// emitted, or the context is canceled.
	Subscribe(ctx context.Context, ticketID uuid.UUID) (<-chan Event, error)

	// Pending returns the tickets currently waiting for a match in the
	// given game_mode, ordered oldest first. The matchmaker reads from
	// this to find candidates.
	Pending(ctx context.Context, gameMode string) ([]*Ticket, error)

	// Claim atomically removes the given tickets from the pending pool,
	// reserving them for the caller. It returns true only if every ticket
	// was still pending; if any was already claimed by a concurrent
	// matchmaker (another gateway replica), it claims none and returns
	// false, so the caller abandons the grouping and retries. This is
	// what makes matchmaking safe to run on more than one gateway.
	Claim(ctx context.Context, gameMode string, ticketIDs []uuid.UUID) (bool, error)

	// Publish broadcasts an event to subscribers of the given ticket.
	// The matchmaker uses this to signal Matched; the handler uses it
	// to signal Failed.
	Publish(ctx context.Context, ticketID uuid.UUID, event Event) error

	// Depth reports how many tickets are currently waiting in the given
	// game_mode, used as a hint in EventQueued.
	Depth(ctx context.Context, gameMode string) (int32, error)
}
