// Package match orchestrates the MatchService.
//
// Tickets live in the match.Queue (an in-process implementation today,
// a Valkey-backed one later). The matchmaker worker — also in this
// package — drains the queue and asks the Allocator for a room. The
// usecase exposes the surface that the Connect handler talks to:
// CreateTicket, WatchTicket, CancelTicket, GetCurrentMatch.
package match

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/match"
	"github.com/averak/vfx/internal/stdx/clock"
)

// Usecase is the application-side entrypoint for matchmaking.
type Usecase struct {
	queue match.Queue
}

// New wires the usecase. The matchmaker worker is a separate
// long-running component constructed by NewMatchmaker.
func New(queue match.Queue) *Usecase {
	return &Usecase{queue: queue}
}

// TicketInput carries the request parameters from the handler down to
// the usecase, without dragging proto types this deep.
type TicketInput struct {
	PlayerID     uuid.UUID
	GameMode     string
	Rating       *float64
	Region       *string
	PartyMembers []uuid.UUID
	Attributes   map[string]string
}

// CreateTicket enqueues a fresh ticket for the player.
func (u *Usecase) CreateTicket(ctx context.Context, in *TicketInput) (uuid.UUID, error) {
	now := clock.Now(ctx)
	t := match.NewTicket(uuid.New(), in.PlayerID, in.GameMode, now)
	t.Rating = in.Rating
	t.Region = in.Region
	t.PartyMembers = in.PartyMembers
	t.Attributes = in.Attributes
	if err := u.queue.Enqueue(ctx, t); err != nil {
		return uuid.Nil, err
	}
	return t.ID, nil
}

// WatchTicket subscribes to events for the given ticket. The returned
// channel closes after a terminal event or context cancellation.
func (u *Usecase) WatchTicket(ctx context.Context, ticketID uuid.UUID) (<-chan match.Event, error) {
	return u.queue.Subscribe(ctx, ticketID)
}

// CancelTicket marks the ticket as cancelled, publishing a Failed
// event so any active WatchTicket subscriber exits cleanly.
func (u *Usecase) CancelTicket(ctx context.Context, ticketID uuid.UUID) error {
	if err := u.queue.Cancel(ctx, ticketID); err != nil {
		if errors.Is(err, match.ErrTicketNotFound) {
			return err
		}
		return err
	}
	return nil
}

// GetCurrentMatch returns the player's active match assignment, if any.
//
// Phase 1 has no persistence layer for assignments; the matchmaker
// publishes them to the in-memory queue and clients are expected to
// reconnect via the same WatchTicket stream after a transient drop.
// A future iteration backs this with Valkey so a fresh process or a
// different gateway replica can still re-hand the assignment.
func (u *Usecase) GetCurrentMatch(_ context.Context, _ uuid.UUID) (*match.Assignment, error) {
	return nil, nil //nolint:nilnil // intentional: "no active match" returns (nil, nil) until the assignment store lands.
}
