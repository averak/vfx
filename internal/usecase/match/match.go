// Package match orchestrates the MatchService.
//
// Tickets live in a match.Queue, and the matchmaker worker in this package drains the queue and asks the Allocator for a room.
package match

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/match"
	"github.com/averak/vfx/internal/stdx/clock"
)

type Usecase struct {
	queue       match.Queue
	assignments match.AssignmentStore
}

// New treats a nil assignments store as one that reports "no active match", convenient for tests that exercise only the ticket flow.
func New(queue match.Queue, assignments match.AssignmentStore) *Usecase {
	if assignments == nil {
		assignments = noopAssignmentStore{}
	}
	return &Usecase{queue: queue, assignments: assignments}
}

type noopAssignmentStore struct{}

func (noopAssignmentStore) Put(context.Context, uuid.UUID, *match.Assignment, time.Duration) error {
	return nil
}

func (noopAssignmentStore) Get(context.Context, uuid.UUID) (*match.Assignment, error) {
	return nil, nil //nolint:nilnil // no store configured means no current match.
}

// TicketInput carries the request parameters from the handler down to the usecase, without dragging proto types this deep.
type TicketInput struct {
	PlayerID     uuid.UUID
	GameMode     string
	Rating       *float64
	Region       *string
	PartyMembers []uuid.UUID
	Attributes   map[string]string
}

func (u *Usecase) CreateTicket(ctx context.Context, in *TicketInput) (uuid.UUID, error) {
	now := clock.Now(ctx)
	t, err := match.NewTicket(uuid.New(), in.PlayerID, in.GameMode, now)
	if err != nil {
		return uuid.Nil, err
	}
	t.Rating = in.Rating
	t.Region = in.Region
	t.PartyMembers = in.PartyMembers
	t.Attributes = in.Attributes
	if err := u.queue.Enqueue(ctx, t); err != nil {
		return uuid.Nil, err
	}
	return t.ID, nil
}

func (u *Usecase) WatchTicket(ctx context.Context, ticketID uuid.UUID) (<-chan match.Event, error) {
	return u.queue.Subscribe(ctx, ticketID)
}

// CancelTicket publishes a Failed event so any active WatchTicket subscriber exits cleanly, and returns match.ErrTicketNotFound for an unknown ticket.
func (u *Usecase) CancelTicket(ctx context.Context, ticketID uuid.UUID) error {
	return u.queue.Cancel(ctx, ticketID)
}

// GetCurrentMatch returns the player's active match assignment, or (nil, nil) if there is none.
//
// The matchmaker writes assignments to the AssignmentStore as it pairs tickets, so a client that dropped before reading EventMatched, or one that lands on a different gateway replica, can recover its room here without re-queuing.
func (u *Usecase) GetCurrentMatch(ctx context.Context, playerID uuid.UUID) (*match.Assignment, error) {
	return u.assignments.Get(ctx, playerID)
}
