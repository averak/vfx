package matchqueue_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"pgregory.net/rapid"

	"github.com/averak/vfx/internal/domain/match"
	"github.com/averak/vfx/internal/infra/matchqueue"
)

// Claim is the atomic primitive matchmaking safety rests on, so it must
// be all-or-nothing for any subset of the pending pool: it succeeds only
// when every requested ticket is still claimable, and a success removes
// exactly that set from the pool and cannot be repeated.
func TestInMemClaim_AllOrNothingProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ctx := context.Background()
		q := matchqueue.NewInMem()
		now := time.Now()

		n := rapid.IntRange(1, 6).Draw(t, "ticketCount")
		ids := make([]uuid.UUID, n)
		for i := range ids {
			tk, err := match.NewTicket(uuid.New(), uuid.New(), "rps", now)
			if err != nil {
				t.Fatalf("NewTicket: %v", err)
			}
			ids[i] = tk.ID
			if err := q.Enqueue(ctx, tk); err != nil {
				t.Fatalf("Enqueue: %v", err)
			}
		}

		idxs := rapid.SliceOfDistinct(rapid.IntRange(0, n-1), func(i int) int { return i }).Draw(t, "subset")
		claimSet := make([]uuid.UUID, 0, len(idxs)+1)
		for _, i := range idxs {
			claimSet = append(claimSet, ids[i])
		}
		includeAbsent := rapid.Bool().Draw(t, "includeAbsent")
		if includeAbsent {
			claimSet = append(claimSet, uuid.New())
		}

		ok, err := q.Claim(ctx, "rps", claimSet)
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}

		// An absent ticket makes the whole claim fail.
		if includeAbsent && ok {
			t.Fatalf("claim succeeded despite an absent ticket")
		}
		if !ok {
			return
		}

		// A success removes exactly the claimed tickets from the pending pool.
		pending, err := q.Pending(ctx, "rps")
		if err != nil {
			t.Fatalf("Pending: %v", err)
		}
		claimed := make(map[uuid.UUID]bool, len(claimSet))
		for _, id := range claimSet {
			claimed[id] = true
		}
		for _, p := range pending {
			if claimed[p.ID] {
				t.Fatalf("a claimed ticket is still pending")
			}
		}

		// The same non-empty set cannot be claimed twice.
		if len(claimSet) > 0 {
			again, err := q.Claim(ctx, "rps", claimSet)
			if err != nil {
				t.Fatalf("second Claim: %v", err)
			}
			if again {
				t.Fatalf("re-claimed an already-claimed set")
			}
		}
	})
}
