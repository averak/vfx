package match

import "time"

type MatchingPolicy struct {
	BaseRatingWindow         float64       // rating gap two fresh tickets may have
	RatingWindowGrowthPerSec float64       // window growth per second the oldest ticket has waited
	RegionRelaxAfter         time.Duration // wait after which region matching is dropped
}

func DefaultMatchingPolicy() MatchingPolicy {
	return MatchingPolicy{
		BaseRatingWindow:         100,
		RatingWindowGrowthPerSec: 50,
		RegionRelaxAfter:         15 * time.Second,
	}
}

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

type Matcher struct {
	policy          MatchingPolicy
	playersPerMatch int
}

// NewMatcher treats a playersPerMatch below 1 as 2.
func NewMatcher(playersPerMatch int, policy MatchingPolicy) *Matcher {
	if playersPerMatch < 1 {
		playersPerMatch = 2
	}
	return &Matcher{policy: policy.withDefaults(), playersPerMatch: playersPerMatch}
}

func (m *Matcher) PlayersPerMatch() int { return m.playersPerMatch }

// SelectGroup returns a compatible group of PlayersPerMatch tickets from pending (oldest-first), or nil if none can be formed at time now.
//
// Each ticket is tried as the seed, oldest first, so the longest-waiting ticket is preferred but a single incompatible outlier at the head does not block compatible tickets behind it.
// The seed's wait time governs the group's tier, which keeps the oldest ticket from starving as its window widens.
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
