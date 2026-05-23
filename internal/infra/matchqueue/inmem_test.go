package matchqueue_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/match"
	"github.com/averak/vfx/internal/infra/matchqueue"
)

func newTicket(gameMode string) *match.Ticket {
	return newTicketAt(gameMode, time.Now())
}

func newTicketAt(gameMode string, created time.Time) *match.Ticket {
	t, err := match.NewTicket(uuid.New(), uuid.New(), gameMode, created)
	if err != nil {
		panic(err) // test inputs are always valid
	}
	return t
}

func TestInMem_SubscribeReplaysQueuedState(t *testing.T) {
	q := matchqueue.NewInMem()
	ticket := newTicket("rps")
	if err := q.Enqueue(t.Context(), ticket); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// A subscriber attaching after Enqueue still sees the current Queued
	// state thanks to the cached "latest" event.
	ch, err := q.Subscribe(t.Context(), ticket.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	ev := <-ch
	if _, ok := ev.(match.EventQueued); !ok {
		t.Fatalf("first event = %T, want EventQueued", ev)
	}
}

func TestInMem_PublishMatchedClosesChannel(t *testing.T) {
	q := matchqueue.NewInMem()
	ticket := newTicket("rps")
	_ = q.Enqueue(t.Context(), ticket)

	ch, err := q.Subscribe(t.Context(), ticket.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	<-ch // drain the queued event

	assignment := &match.Assignment{MatchID: uuid.New(), Endpoint: "host:1", SessionToken: "tok"}
	if err := q.Publish(t.Context(), ticket.ID, match.EventMatched{Assignment: assignment}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	matched := <-ch
	if _, ok := matched.(match.EventMatched); !ok {
		t.Fatalf("event = %T, want EventMatched", matched)
	}
	// Terminal event closes the channel.
	if _, open := <-ch; open {
		t.Error("channel still open after a terminal Matched event")
	}
}

func TestInMem_CancelPublishesFailed(t *testing.T) {
	q := matchqueue.NewInMem()
	ticket := newTicket("rps")
	_ = q.Enqueue(t.Context(), ticket)

	ch, _ := q.Subscribe(t.Context(), ticket.ID)
	<-ch // queued

	if err := q.Cancel(t.Context(), ticket.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	ev := <-ch
	failed, ok := ev.(match.EventFailed)
	if !ok {
		t.Fatalf("event = %T, want EventFailed", ev)
	}
	if failed.Reason != "cancelled" {
		t.Errorf("reason = %q, want cancelled", failed.Reason)
	}
}

func TestInMem_PendingIsFIFOAndModeScoped(t *testing.T) {
	q := matchqueue.NewInMem()

	older := newTicketAt("rps", time.Now().Add(-time.Second))
	newer := newTicketAt("rps", time.Now())
	other := newTicket("chess")
	for _, ticket := range []*match.Ticket{newer, older, other} {
		_ = q.Enqueue(t.Context(), ticket)
	}

	pending, err := q.Pending(t.Context(), "rps")
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("Pending(rps) returned %d tickets, want 2", len(pending))
	}
	if pending[0].ID != older.ID {
		t.Error("Pending is not oldest-first")
	}
}

func TestInMem_DepthCountsPendingPerMode(t *testing.T) {
	q := matchqueue.NewInMem()
	_ = q.Enqueue(t.Context(), newTicket("rps"))
	_ = q.Enqueue(t.Context(), newTicket("rps"))
	_ = q.Enqueue(t.Context(), newTicket("chess"))

	depth, err := q.Depth(t.Context(), "rps")
	if err != nil {
		t.Fatalf("Depth: %v", err)
	}
	if depth != 2 {
		t.Errorf("Depth(rps) = %d, want 2", depth)
	}
}

func TestInMem_CancelUnknownTicket(t *testing.T) {
	q := matchqueue.NewInMem()
	if err := q.Cancel(t.Context(), uuid.New()); err == nil {
		t.Error("Cancel of unknown ticket returned nil, want ErrTicketNotFound")
	}
}
