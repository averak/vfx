package match_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"pgregory.net/rapid"

	"github.com/averak/vfx/internal/domain/match"
)

// However the pending pool is shaped, a group SelectGroup returns is
// always exactly PlayersPerMatch distinct tickets drawn from that pool.
// (The helpers policy, ticket, f, and s live in matcher_test.go.)
func TestMatcher_SelectGroupStructuralInvariants(t *testing.T) {
	regions := rapid.SampledFrom([]string{"us", "eu", "ap"})

	rapid.Check(t, func(t *rapid.T) {
		k := rapid.IntRange(2, 4).Draw(t, "playersPerMatch")
		m := match.NewMatcher(k, policy())
		now := time.Now()

		n := rapid.IntRange(0, 10).Draw(t, "pendingCount")
		pending := make([]*match.Ticket, n)
		for i := range pending {
			waited := time.Duration(rapid.IntRange(0, 120).Draw(t, "waitSeconds")) * time.Second
			var rating *float64
			if rapid.Bool().Draw(t, "hasRating") {
				rating = f(float64(rapid.IntRange(0, 3000).Draw(t, "rating")))
			}
			var region *string
			if rapid.Bool().Draw(t, "hasRegion") {
				region = s(regions.Draw(t, "region"))
			}
			pending[i] = ticket(now.Add(-waited), rating, region)
		}

		group := m.SelectGroup(now, pending)
		if group == nil {
			return
		}

		if len(group) != k {
			t.Fatalf("group size %d, want %d", len(group), k)
		}
		inPending := make(map[uuid.UUID]bool, n)
		for _, p := range pending {
			inPending[p.ID] = true
		}
		seen := make(map[uuid.UUID]bool, k)
		for _, g := range group {
			if !inPending[g.ID] {
				t.Fatalf("group contains a ticket that was not pending")
			}
			if seen[g.ID] {
				t.Fatalf("group contains a duplicate ticket")
			}
			seen[g.ID] = true
		}
	})
}
