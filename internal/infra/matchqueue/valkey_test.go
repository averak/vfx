package matchqueue_test

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	domainmatch "github.com/averak/vfx/internal/domain/match"
	"github.com/averak/vfx/internal/infra/matchqueue"
	"github.com/averak/vfx/internal/infra/valkey"
)

func newValkeyQueue(t *testing.T) *matchqueue.Valkey {
	t.Helper()
	url := os.Getenv("VALKEY_URL")
	if url == "" {
		t.Skip("VALKEY_URL not set; skipping Valkey-backed queue test")
	}
	client, err := valkey.NewClient(url)
	if err != nil {
		t.Fatalf("connect valkey: %v", err)
	}
	t.Cleanup(client.Close)
	return matchqueue.NewValkey(client)
}

// uniqueMode keeps parallel test runs from colliding on shared keys.
func uniqueMode() string { return "test-" + uuid.NewString() }

func enqueue(t *testing.T, q *matchqueue.Valkey, mode string) *domainmatch.Ticket {
	t.Helper()
	ticket, err := domainmatch.NewTicket(uuid.New(), uuid.New(), mode, time.Now())
	if err != nil {
		t.Fatalf("NewTicket: %v", err)
	}
	if err := q.Enqueue(t.Context(), ticket); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	return ticket
}

func TestValkeyQueue_EnqueuePendingDepth(t *testing.T) {
	q := newValkeyQueue(t)
	mode := uniqueMode()

	a := enqueue(t, q, mode)
	b := enqueue(t, q, mode)

	pending, err := q.Pending(t.Context(), mode)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending = %d, want 2", len(pending))
	}
	// FIFO: a (enqueued first) comes first.
	if pending[0].ID != a.ID || pending[1].ID != b.ID {
		t.Errorf("pending order = [%s %s], want [%s %s]", pending[0].ID, pending[1].ID, a.ID, b.ID)
	}

	depth, err := q.Depth(t.Context(), mode)
	if err != nil {
		t.Fatalf("Depth: %v", err)
	}
	if depth != 2 {
		t.Errorf("depth = %d, want 2", depth)
	}
}

func TestValkeyQueue_SubscribeReceivesQueuedThenMatched(t *testing.T) {
	q := newValkeyQueue(t)
	mode := uniqueMode()
	ticket := enqueue(t, q, mode)

	ch, err := q.Subscribe(t.Context(), ticket.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// First event is the seeded Queued snapshot.
	ev := recvEvent(t, ch)
	if _, ok := ev.(domainmatch.EventQueued); !ok {
		t.Fatalf("first event = %T, want Queued", ev)
	}

	// Let the SUBSCRIBE connection establish before publishing.
	time.Sleep(150 * time.Millisecond)
	assignment := &domainmatch.Assignment{
		MatchID:      uuid.New(),
		Endpoint:     "room:7777",
		SessionToken: "tok",
		ExpiresAt:    time.Now().Add(time.Minute),
	}
	if err := q.Publish(t.Context(), ticket.ID, domainmatch.EventMatched{Assignment: assignment}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	ev = recvEvent(t, ch)
	matched, ok := ev.(domainmatch.EventMatched)
	if !ok {
		t.Fatalf("second event = %T, want Matched", ev)
	}
	if matched.Assignment.Endpoint != "room:7777" {
		t.Errorf("endpoint = %q, want room:7777", matched.Assignment.Endpoint)
	}

	// A terminal event removes the ticket from the pending pool.
	pending, _ := q.Pending(t.Context(), mode)
	if len(pending) != 0 {
		t.Errorf("pending = %d after match, want 0", len(pending))
	}
}

func TestValkeyQueue_ClaimIsAtomic(t *testing.T) {
	q := newValkeyQueue(t)
	mode := uniqueMode()
	a := enqueue(t, q, mode)
	b := enqueue(t, q, mode)

	ok, err := q.Claim(t.Context(), mode, []uuid.UUID{a.ID, b.ID})
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if !ok {
		t.Fatal("first Claim returned false, want true")
	}
	// A second claim of the same tickets must fail; they are gone.
	ok, err = q.Claim(t.Context(), mode, []uuid.UUID{a.ID, b.ID})
	if err != nil {
		t.Fatalf("Claim 2: %v", err)
	}
	if ok {
		t.Fatal("second Claim returned true, want false (already claimed)")
	}
}

func TestValkeyQueue_ClaimAllOrNothing(t *testing.T) {
	q := newValkeyQueue(t)
	mode := uniqueMode()
	a := enqueue(t, q, mode)
	b := enqueue(t, q, mode)

	// Claim a alone, then try to claim a+b: must fail and leave b pending.
	if ok, _ := q.Claim(t.Context(), mode, []uuid.UUID{a.ID}); !ok {
		t.Fatal("claiming a alone failed")
	}
	ok, _ := q.Claim(t.Context(), mode, []uuid.UUID{a.ID, b.ID})
	if ok {
		t.Fatal("claim of a(taken)+b succeeded, want false")
	}
	pending, _ := q.Pending(t.Context(), mode)
	if len(pending) != 1 || pending[0].ID != b.ID {
		t.Errorf("after partial claim, pending = %v, want just b", pending)
	}
}

func TestValkeyQueue_CancelPublishesFailed(t *testing.T) {
	q := newValkeyQueue(t)
	mode := uniqueMode()
	ticket := enqueue(t, q, mode)

	if err := q.Cancel(t.Context(), ticket.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	pending, _ := q.Pending(t.Context(), mode)
	if len(pending) != 0 {
		t.Errorf("pending = %d after cancel, want 0", len(pending))
	}
	// A late subscriber replays the stream history (Queued) and then the
	// terminal Failed, after which the channel closes.
	ch, err := q.Subscribe(t.Context(), ticket.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	var sawFailed bool
	for !sawFailed {
		if _, ok := recvEvent(t, ch).(domainmatch.EventFailed); ok {
			sawFailed = true
		}
	}
}

func TestValkeyQueue_SubscribeUnknownTicket(t *testing.T) {
	q := newValkeyQueue(t)
	if _, err := q.Subscribe(t.Context(), uuid.New()); err == nil {
		t.Error("Subscribe to an unknown ticket succeeded, want ErrTicketNotFound")
	}
}

func recvEvent(t *testing.T, ch <-chan domainmatch.Event) domainmatch.Event {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("event channel closed before an event arrived")
		}
		return ev
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for an event")
		return nil
	}
}
