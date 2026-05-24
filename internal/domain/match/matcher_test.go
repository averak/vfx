package match_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/match"
)

// party builds the member tickets of one party: every member's ticket carries the same normalized roster, as the usecase produces.
func party(created time.Time, members ...uuid.UUID) []*match.Ticket {
	roster := match.NormalizeParty(members[0], members[1:])
	out := make([]*match.Ticket, len(members))
	for i, pid := range members {
		t, err := match.NewTicket(uuid.New(), pid, "rps", created)
		if err != nil {
			panic(err)
		}
		t.PartyMembers = roster
		out[i] = t
	}
	return out
}

func playerSet(group []*match.Ticket) map[uuid.UUID]bool {
	out := make(map[uuid.UUID]bool, len(group))
	for _, g := range group {
		out[g.PlayerID] = true
	}
	return out
}

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

func TestMatcher_KeepsPartyTogether(t *testing.T) {
	m := match.NewMatcher(2, policy())
	now := time.Now()
	a, b := uuid.New(), uuid.New()
	group := m.SelectGroup(now, party(now, a, b))
	if len(group) != 2 {
		t.Fatalf("party of 2 not matched: %v", group)
	}
	players := playerSet(group)
	if !players[a] || !players[b] {
		t.Errorf("party members missing from match: %v", players)
	}
}

// A party member must not be matched as a solo while the rest of the party is still queuing.
func TestMatcher_IncompletePartyWaits(t *testing.T) {
	m := match.NewMatcher(2, policy())
	now := time.Now()
	a, b := uuid.New(), uuid.New()
	full := party(now, a, b)
	// Only member a is present; b never queued. A lone solo also waits.
	pending := []*match.Ticket{full[0], ticket(now, nil, nil)}
	if g := m.SelectGroup(now, pending); g != nil {
		t.Fatalf("incomplete party was matched (possibly with the solo): %v", g)
	}
}

func TestMatcher_PartyPlusSoloFillsMatch(t *testing.T) {
	m := match.NewMatcher(3, policy())
	now := time.Now()
	a, b := uuid.New(), uuid.New()
	pending := append(party(now, a, b), ticket(now, nil, nil))
	group := m.SelectGroup(now, pending)
	if len(group) != 3 {
		t.Fatalf("party + solo did not fill a 3-player match: %v", group)
	}
}

func TestMatcher_TwoPartiesFillMatch(t *testing.T) {
	m := match.NewMatcher(4, policy())
	now := time.Now()
	a, b, c, d := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	pending := append(party(now, a, b), party(now, c, d)...)
	group := m.SelectGroup(now, pending)
	if len(group) != 4 {
		t.Fatalf("two 2-player parties did not fill a 4-player match: %v", group)
	}
}

func TestMatcher_PartyLargerThanMatchNeverPlaced(t *testing.T) {
	m := match.NewMatcher(2, policy())
	now := time.Now()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	if g := m.SelectGroup(now, party(now, a, b, c)); g != nil {
		t.Fatalf("a party larger than the match was placed: %v", g)
	}
}

// Even when a single party member could fill the last slot, the matcher places the party whole or not at all.
func TestMatcher_DoesNotSplitParty(t *testing.T) {
	m := match.NewMatcher(2, policy())
	now := time.Now()
	a, b := uuid.New(), uuid.New()
	pty := party(now, a, b)
	// Solo first, so a naive matcher would pair solo + a and strand b.
	pending := []*match.Ticket{ticket(now, nil, nil), pty[0], pty[1]}
	group := m.SelectGroup(now, pending)
	if len(group) != 2 {
		t.Fatalf("no group formed: %v", group)
	}
	players := playerSet(group)
	if !players[a] || !players[b] {
		t.Errorf("party was split; group players = %v", players)
	}
}

// A party mixed into a seed's match still respects that seed's rating tier.
func TestMatcher_PartyRespectsSeedTier(t *testing.T) {
	m := match.NewMatcher(3, policy())
	now := time.Now()
	a, b := uuid.New(), uuid.New()
	pty := party(now, a, b)
	pty[0].Rating, pty[1].Rating = f(1000), f(1000)
	// A fresh solo seed at 3000 cannot admit a 1000-rated party (gap 2000 > base window 100), so no match forms.
	pending := append([]*match.Ticket{ticket(now, f(3000), nil)}, pty...)
	if g := m.SelectGroup(now, pending); g != nil {
		t.Fatalf("party outside the seed's rating window was placed: %v", g)
	}
}

func TestNormalizeParty(t *testing.T) {
	a := uuid.New()
	b := uuid.New()
	if r := match.NormalizeParty(a, nil); r != nil {
		t.Errorf("solo (no members) roster = %v, want nil", r)
	}
	if r := match.NormalizeParty(a, []uuid.UUID{a}); r != nil {
		t.Errorf("self-only roster = %v, want nil", r)
	}
	// Self is auto-included and duplicates collapse, so either member yields the identical canonical roster.
	r1 := match.NormalizeParty(a, []uuid.UUID{b, b})
	r2 := match.NormalizeParty(b, []uuid.UUID{a})
	if len(r1) != 2 || r1[0] != r2[0] || r1[1] != r2[1] {
		t.Errorf("rosters not canonical/equal: %v vs %v", r1, r2)
	}
	if r1[0].String() > r1[1].String() {
		t.Error("roster is not sorted")
	}
}

func TestPartyKey(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	pty := party(time.Now(), a, b)
	if !pty[0].IsParty() || !pty[1].IsParty() {
		t.Fatal("party tickets not detected as a party")
	}
	if pty[0].PartyKey() != pty[1].PartyKey() {
		t.Errorf("party members disagree on key: %q vs %q", pty[0].PartyKey(), pty[1].PartyKey())
	}
	solo := ticket(time.Now(), nil, nil)
	if solo.IsParty() || solo.PartyKey() != "" {
		t.Error("solo ticket misclassified as a party")
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
