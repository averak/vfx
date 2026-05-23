package match

import "time"

// MatchingPolicy parameterises matchmaking fairness: how the acceptable
// rating gap widens the longer a ticket waits, and when region matching
// is relaxed. The thresholds are operational (supplied by config), but
// the rules that interpret them are domain rules.
type MatchingPolicy struct {
	// BaseRatingWindow is the rating gap two fresh tickets may have.
	BaseRatingWindow float64
	// RatingWindowGrowthPerSec widens the window per second of wait by
	// the longest-waiting ticket in a candidate group.
	RatingWindowGrowthPerSec float64
	// RegionRelaxAfter is how long a ticket waits before region matching
	// is dropped for it.
	RegionRelaxAfter time.Duration
}

// DefaultMatchingPolicy is used when configuration leaves a field zero.
func DefaultMatchingPolicy() MatchingPolicy {
	return MatchingPolicy{
		BaseRatingWindow:         100,
		RatingWindowGrowthPerSec: 50,
		RegionRelaxAfter:         15 * time.Second,
	}
}

// withDefaults fills zero fields from DefaultMatchingPolicy so callers
// can configure only the knobs they care about.
func (p MatchingPolicy) withDefaults() MatchingPolicy {
	d := DefaultMatchingPolicy()
	if p.BaseRatingWindow == 0 {
		p.BaseRatingWindow = d.BaseRatingWindow
	}
	if p.RatingWindowGrowthPerSec == 0 {
		p.RatingWindowGrowthPerSec = d.RatingWindowGrowthPerSec
	}
	if p.RegionRelaxAfter == 0 {
		p.RegionRelaxAfter = d.RegionRelaxAfter
	}
	return p
}

// Matcher decides which queued tickets form a match. This is the
// matchmaking domain service: it owns the enterprise rule of pairing
// eligibility and tier relaxation. The usecase only orchestrates around
// the group it returns (claim, allocate, notify); it makes no matching
// decisions itself.
type Matcher struct {
	policy          MatchingPolicy
	playersPerMatch int
}

// NewMatcher builds a Matcher. playersPerMatch below 1 falls back to 2.
func NewMatcher(playersPerMatch int, policy MatchingPolicy) *Matcher {
	if playersPerMatch < 1 {
		playersPerMatch = 2
	}
	return &Matcher{policy: policy.withDefaults(), playersPerMatch: playersPerMatch}
}

// PlayersPerMatch is how many tickets form one match.
func (m *Matcher) PlayersPerMatch() int { return m.playersPerMatch }

// SelectGroup returns a compatible group of PlayersPerMatch tickets from
// pending (which must be oldest-first), or nil if none can be formed at
// time now.
//
// Each ticket is tried as the seed, oldest first, so the longest-waiting
// ticket is preferred but a single incompatible outlier at the head does
// not block compatible tickets behind it. The seed's wait time governs
// the group's tier, which keeps the oldest ticket from starving as its
// window widens.
func (m *Matcher) SelectGroup(now time.Time, pending []*Ticket) []*Ticket {
	for i := range pending {
		if group := m.groupFromSeed(now, pending, i); group != nil {
			return group
		}
	}
	return nil
}

func (m *Matcher) groupFromSeed(now time.Time, pending []*Ticket, seedIdx int) []*Ticket {
	seed := pending[seedIdx]
	waited := now.Sub(seed.CreatedAt)
	window := m.policy.BaseRatingWindow + m.policy.RatingWindowGrowthPerSec*waited.Seconds()
	ignoreRegion := waited >= m.policy.RegionRelaxAfter

	group := make([]*Ticket, 0, m.playersPerMatch)
	group = append(group, seed)
	for j, t := range pending {
		if j == seedIdx {
			continue
		}
		if len(group) == m.playersPerMatch {
			break
		}
		if seed.CompatibleWith(t, window, ignoreRegion) {
			group = append(group, t)
		}
	}
	if len(group) < m.playersPerMatch {
		return nil
	}
	return group
}
