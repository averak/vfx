// Package matchqueue holds queue implementations of the match.Queue contract.
//
// InMem is the single-process backend: an in-memory queue with per-ticket fan-out.
// It is correct for a single-gateway deployment; the Valkey-backed implementation is used when matchmaking has to span gateway replicas.
package matchqueue

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/match"
)

type InMem struct {
	mu          sync.Mutex
	tickets     map[uuid.UUID]*ticketEntry
	subscribers map[uuid.UUID][]chan match.Event
}

type ticketEntry struct {
	ticket   *match.Ticket
	latest   match.Event
	finished bool
	claimed  bool
}

var _ match.Queue = (*InMem)(nil)

func NewInMem() *InMem {
	return &InMem{
		tickets:     make(map[uuid.UUID]*ticketEntry),
		subscribers: make(map[uuid.UUID][]chan match.Event),
	}
}

func (q *InMem) Enqueue(_ context.Context, t *match.Ticket) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, exists := q.tickets[t.ID]; exists {
		return nil
	}
	depth := q.countPendingLocked(t.GameMode) + 1
	queued := match.EventQueued{
		QueuedAt:   t.CreatedAt,
		QueueDepth: depth,
	}
	q.tickets[t.ID] = &ticketEntry{
		ticket: t,
		latest: queued,
	}
	return nil
}

func (q *InMem) Cancel(_ context.Context, ticketID uuid.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	entry, ok := q.tickets[ticketID]
	if !ok {
		return match.ErrTicketNotFound
	}
	if entry.finished {
		return nil
	}
	q.publishLocked(ticketID, match.EventFailed{
		Reason:  "cancelled",
		Message: "ticket was cancelled by the client",
	})
	return nil
}

func (q *InMem) Subscribe(ctx context.Context, ticketID uuid.UUID) (<-chan match.Event, error) {
	q.mu.Lock()
	entry, ok := q.tickets[ticketID]
	if !ok {
		q.mu.Unlock()
		return nil, match.ErrTicketNotFound
	}

	ch := make(chan match.Event, 4)
	if entry.latest != nil {
		ch <- entry.latest
	}
	if entry.finished {
		close(ch)
		q.mu.Unlock()
		return ch, nil
	}
	q.subscribers[ticketID] = append(q.subscribers[ticketID], ch)
	q.mu.Unlock()

	// Watch for ctx cancellation so a disconnecting client doesn't leak a subscriber slot.
	go func() {
		<-ctx.Done()
		q.detach(ticketID, ch)
	}()
	return ch, nil
}

func (q *InMem) Pending(_ context.Context, gameMode string) ([]*match.Ticket, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	out := make([]*match.Ticket, 0)
	for _, e := range q.tickets {
		if e.finished || e.claimed {
			continue
		}
		if e.ticket.GameMode != gameMode {
			continue
		}
		out = append(out, e.ticket)
	}
	// Older tickets first so the matchmaker is FIFO-ish.
	sortByCreatedAt(out)
	return out, nil
}

// Claim marks the tickets as taken, removing them from the pending pool.
// All-or-nothing: if any ticket is already claimed or finished, none are claimed and it returns false.
func (q *InMem) Claim(_ context.Context, _ string, ticketIDs []uuid.UUID) (bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, id := range ticketIDs {
		e, ok := q.tickets[id]
		if !ok || e.finished || e.claimed {
			return false, nil
		}
	}
	for _, id := range ticketIDs {
		q.tickets[id].claimed = true
	}
	return true, nil
}

func (q *InMem) Publish(_ context.Context, ticketID uuid.UUID, event match.Event) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, ok := q.tickets[ticketID]; !ok {
		return match.ErrTicketNotFound
	}
	q.publishLocked(ticketID, event)
	return nil
}

func (q *InMem) Depth(_ context.Context, gameMode string) (int32, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.countPendingLocked(gameMode), nil
}

func (q *InMem) countPendingLocked(gameMode string) int32 {
	var n int32
	for _, e := range q.tickets {
		if e.finished || e.claimed {
			continue
		}
		if e.ticket.GameMode == gameMode {
			n++
		}
	}
	return n
}

// publishLocked writes event to all current subscribers of ticketID, updates the cached "latest" event, and marks the entry finished when event is terminal.
// Caller must hold q.mu.
func (q *InMem) publishLocked(ticketID uuid.UUID, event match.Event) {
	entry := q.tickets[ticketID]
	entry.latest = event

	terminal := isTerminal(event)
	subs := q.subscribers[ticketID]
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			// Slow consumer: drop the event.
			// A reconnecting client resyncs via GetCurrentMatch when needed.
		}
		if terminal {
			close(ch)
		}
	}
	if terminal {
		entry.finished = true
		delete(q.subscribers, ticketID)
	}
}

// detach removes a subscriber on context cancellation and closes its channel so the handler's range loop unblocks.
// If a terminal event already removed and closed the channel via publishLocked, target is no longer in the slice, so we leave it alone and avoid a double close.
// Both paths run under q.mu, so the find-then-close is race-free.
func (q *InMem) detach(ticketID uuid.UUID, target chan match.Event) {
	q.mu.Lock()
	defer q.mu.Unlock()

	subs := q.subscribers[ticketID]
	for i, ch := range subs {
		if ch == target {
			q.subscribers[ticketID] = append(subs[:i], subs[i+1:]...)
			close(ch)
			return
		}
	}
}

func isTerminal(e match.Event) bool {
	switch e.(type) {
	case match.EventMatched, match.EventFailed:
		return true
	}
	return false
}

// sortByCreatedAt orders tickets oldest-first.
// An insertion sort is fine for the small queue sizes a single gateway holds in memory.
func sortByCreatedAt(ts []*match.Ticket) {
	for i := 1; i < len(ts); i++ {
		for j := i; j > 0 && ts[j-1].CreatedAt.After(ts[j].CreatedAt); j-- {
			ts[j-1], ts[j] = ts[j], ts[j-1]
		}
	}
}
