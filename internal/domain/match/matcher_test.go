package match_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/match"
)

// policy is a fixed test policy: base window 100, +50/sec, region
// relaxes after 15s. Boundary tests below lean on these exact numbers.
func policy() match.MatchingPolicy {
	return match.MatchingPolicy{
		BaseRatingWindow:         100,
		RatingWindowGrowthPerSec: 50,
		RegionRelaxAfter:         15 * time.Second,
	}
}

func ticket(created time.Time, rating *float64, region *string) *match.Ticket {
	t, err := match.NewTicket(uuid.New(), uuid.New(), "rps", created)
	if err != nil {
		panic(err)
	}
	t.Rating = rating
	t.Region = region
	return t
}

func f(v float64) *float64 { return &v }
func s(v string) *string   { return &v }

func TestMatcher_PairsWithinRatingWindow(t *testing.T) {
	m := match.NewMatcher(2, policy())
	now := time.Now()
	pending := []*match.Ticket{ticket(now, f(1000), nil), ticket(now, f(1080), nil)}
	if m.SelectGroup(now, pending) == nil {
		t.Fatal("ratings within the base window did not pair")
	}
}

func TestMatcher_RejectsRatingOutsideWindow(t *testing.T) {
	m := match.NewMatcher(2, policy())
	now := time.Now()
	pending := []*match.Ticket{ticket(now, f(1000), nil), ticket(now, f(1500), nil)}
	if m.SelectGroup(now, pending) != nil {
		t.Fatal("ratings 500 apart paired inside the base window")
	}
}

// Boundary-value analysis on the rating window: with a base window of
// 100 and fresh tickets, a gap of exactly 100 must pair (<=), 100+ε must
// not. CompatibleWith uses `> window` to reject, so == is inclusive.
func TestMatcher_RatingWindowBoundary(t *testing.T) {
	m := match.NewMatcher(2, policy())
	now := time.Now()

	exactlyAtWindow := []*match.Ticket{ticket(now, f(1000), nil), ticket(now, f(1100), nil)} // gap 100 == window
	if m.SelectGroup(now, exactlyAtWindow) == nil {
		t.Error("gap == window should pair (inclusive boundary)")
	}

	justOverWindow := []*match.Ticket{ticket(now, f(1000), nil), ticket(now, f(1100.001), nil)} // gap > window
	if m.SelectGroup(now, justOverWindow) != nil {
		t.Error("gap just over window should not pair")
	}
}

func TestMatcher_WindowWidensWithWait(t *testing.T) {
	m := match.NewMatcher(2, policy())
	now := time.Now()
	// Seed waited 10s → window = 100 + 50*10 = 600, admits a 500 gap.
	pending := []*match.Ticket{ticket(now.Add(-10*time.Second), f(1000), nil), ticket(now, f(1500), nil)}
	if m.SelectGroup(now, pending) == nil {
		t.Fatal("widened window (600) failed to admit a 500 gap")
	}
}

// Boundary-value analysis on region relaxation: region is ignored once
// the seed has waited >= RegionRelaxAfter. So exactly 15s relaxes,
// 15s-ε does not.
func TestMatcher_RegionRelaxBoundary(t *testing.T) {
	m := match.NewMatcher(2, policy())
	now := time.Now()
	us, eu := s("us"), s("eu")

	justBefore := []*match.Ticket{ticket(now.Add(-(15*time.Second - time.Millisecond)), f(1000), us), ticket(now, f(1000), eu)}
	if m.SelectGroup(now, justBefore) != nil {
		t.Error("cross-region paired just before RegionRelaxAfter")
	}

	exactly := []*match.Ticket{ticket(now.Add(-15*time.Second), f(1000), us), ticket(now, f(1000), eu)}
	if m.SelectGroup(now, exactly) == nil {
		t.Error("cross-region did not pair at exactly RegionRelaxAfter (>= boundary)")
	}
}

func TestMatcher_MissingRatingSkipsCheck(t *testing.T) {
	m := match.NewMatcher(2, policy())
	now := time.Now()
	pending := []*match.Ticket{ticket(now, nil, nil), ticket(now, f(9999), nil)}
	if m.SelectGroup(now, pending) == nil {
		t.Fatal("a ticket without a rating should pair regardless of window")
	}
}

func TestMatcher_SkipsIncompatibleOutlierSeed(t *testing.T) {
	m := match.NewMatcher(2, policy())
	now := time.Now()
	us, eu := s("us"), s("eu")
	// Oldest is a fresh us outlier with no us partner; the two eu tickets
	// behind it must still pair.
	pending := []*match.Ticket{
		ticket(now, f(1000), us),
		ticket(now, f(1000), eu),
		ticket(now, f(1010), eu),
	}
	group := m.SelectGroup(now, pending)
	if group == nil {
		t.Fatal("eu pair behind a us outlier failed to form")
	}
	for _, tk := range group {
		if tk.Region != nil && *tk.Region == "us" {
			t.Fatal("the us outlier was wrongly included")
		}
	}
}

func TestMatcher_NilWhenNoPartner(t *testing.T) {
	m := match.NewMatcher(2, policy())
	now := time.Now()
	pending := []*match.Ticket{ticket(now, f(1000), s("us")), ticket(now, f(1000), s("eu"))}
	if m.SelectGroup(now, pending) != nil {
		t.Fatal("formed a group despite no compatible partner")
	}
}

func TestNewTicket_Invariants(t *testing.T) {
	now := time.Now()
	if _, err := match.NewTicket(uuid.New(), uuid.Nil, "rps", now); err == nil {
		t.Error("ticket with no player should be rejected")
	}
	if _, err := match.NewTicket(uuid.New(), uuid.New(), "", now); err == nil {
		t.Error("ticket with no game mode should be rejected")
	}
	if _, err := match.NewTicket(uuid.New(), uuid.New(), "rps", now); err != nil {
		t.Errorf("valid ticket rejected: %v", err)
	}
}
