package match

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/match"
)

// newMM builds a Matchmaker with only the tier knobs set; selectBatch
// touches nothing else, so the queue/allocator/signer can stay nil.
func newMM() *Matchmaker {
	return &Matchmaker{
		playersPerMatch:          2,
		baseRatingWindow:         100,
		ratingWindowGrowthPerSec: 50,
		regionRelaxAfter:         15 * time.Second,
	}
}

func ticketAt(created time.Time, rating *float64, region *string) *match.Ticket {
	return &match.Ticket{
		ID:        uuid.New(),
		PlayerID:  uuid.New(),
		GameMode:  "rps",
		Rating:    rating,
		Region:    region,
		CreatedAt: created,
	}
}

func f(v float64) *float64 { return &v }
func s(v string) *string   { return &v }

func TestSelectBatch_PairsWithinRatingWindow(t *testing.T) {
	mm := newMM()
	now := time.Now()
	// Both just queued: window is the base 100. Ratings 1000 and 1080 are
	// within it.
	pending := []*match.Ticket{
		ticketAt(now, f(1000), nil),
		ticketAt(now, f(1080), nil),
	}
	if got := mm.selectBatch(now, pending); got == nil {
		t.Fatal("ratings within the base window did not pair")
	}
}

func TestSelectBatch_RejectsRatingOutsideWindow(t *testing.T) {
	mm := newMM()
	now := time.Now()
	// Fresh tickets, base window 100, ratings differ by 500 → no pair yet.
	pending := []*match.Ticket{
		ticketAt(now, f(1000), nil),
		ticketAt(now, f(1500), nil),
	}
	if got := mm.selectBatch(now, pending); got != nil {
		t.Fatal("ratings 500 apart paired inside the base window")
	}
}

func TestSelectBatch_WindowWidensWithWait(t *testing.T) {
	mm := newMM()
	now := time.Now()
	// Seed waited 10s: window = 100 + 50*10 = 600, so a 500 gap fits.
	created := now.Add(-10 * time.Second)
	pending := []*match.Ticket{
		ticketAt(created, f(1000), nil),
		ticketAt(now, f(1500), nil),
	}
	if got := mm.selectBatch(now, pending); got == nil {
		t.Fatal("widened window (600) failed to admit a 500-point gap")
	}
}

func TestSelectBatch_RegionEnforcedThenRelaxed(t *testing.T) {
	mm := newMM()
	now := time.Now()
	us, eu := s("us"), s("eu")

	// Fresh: different regions must not pair.
	fresh := []*match.Ticket{
		ticketAt(now, f(1000), us),
		ticketAt(now, f(1010), eu),
	}
	if got := mm.selectBatch(now, fresh); got != nil {
		t.Fatal("cross-region tickets paired before RegionRelaxAfter")
	}

	// Seed waited past RegionRelaxAfter (15s): region is ignored.
	relaxed := []*match.Ticket{
		ticketAt(now.Add(-20*time.Second), f(1000), us),
		ticketAt(now, f(1010), eu),
	}
	if got := mm.selectBatch(now, relaxed); got == nil {
		t.Fatal("cross-region tickets did not pair after RegionRelaxAfter")
	}
}

func TestSelectBatch_MissingRatingSkipsCheck(t *testing.T) {
	mm := newMM()
	now := time.Now()
	// One ticket has no rating → rating dimension is skipped, they pair.
	pending := []*match.Ticket{
		ticketAt(now, nil, nil),
		ticketAt(now, f(9999), nil),
	}
	if got := mm.selectBatch(now, pending); got == nil {
		t.Fatal("a ticket without a rating should pair regardless of window")
	}
}

func TestSelectBatch_NilWhenNoPartner(t *testing.T) {
	mm := newMM()
	now := time.Now()
	// A lone compatible candidate is missing: only the seed is present
	// after filtering, so no group of 2 forms.
	pending := []*match.Ticket{
		ticketAt(now, f(1000), s("us")),
		ticketAt(now, f(1000), s("eu")), // region-incompatible while fresh
	}
	if got := mm.selectBatch(now, pending); got != nil {
		t.Fatal("formed a group despite no compatible partner")
	}
}
