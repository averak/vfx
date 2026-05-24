package match

import (
	"sort"
	"time"

	"github.com/google/uuid"
)

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

// SelectGroup returns a compatible group of exactly PlayersPerMatch tickets from pending, or nil if none can be formed at time now.
//
// The pool is first collapsed into indivisible units — a solo ticket, or a complete party that must be placed whole — and each unit is tried as the seed oldest-first, so the longest-waiting entry is preferred but a single incompatible outlier at the head does not block compatible entries behind it.
// The seed's wait time governs the group's tier, which keeps the oldest entry from starving as its window widens.
func (m *Matcher) SelectGroup(now time.Time, pending []*Ticket) []*Ticket {
	units := buildUnits(pending)
	for i := range units {
		if group := m.groupFromSeed(now, units, i); group != nil {
			return group
		}
	}
	return nil
}

// matchUnit is an indivisible matchmaking entry: a solo ticket, or a complete party (one ticket per member) that is placed together or not at all.
type matchUnit struct {
	tickets []*Ticket
	seedAt  time.Time // oldest EnqueuedAt among the unit's tickets; governs its place in line and its tier
}

func (u matchUnit) size() int { return len(u.tickets) }

// anchor is the unit's longest-waiting ticket, used as the rating/region reference when this unit seeds a group.
func (u matchUnit) anchor() *Ticket {
	a := u.tickets[0]
	for _, t := range u.tickets[1:] {
		if t.EnqueuedAt.Before(a.EnqueuedAt) {
			a = t
		}
	}
	return a
}

// buildUnits collapses the pending pool into units, oldest-first.
// Solo tickets become single-ticket units. Party tickets are grouped by their shared roster key; a party becomes a unit only once every member has a pending ticket for it (the oldest per member is kept), so an incomplete party simply waits without blocking anyone else.
func buildUnits(pending []*Ticket) []matchUnit {
	var units []matchUnit
	rosters := make(map[string][]uuid.UUID)
	byKeyPlayer := make(map[string]map[uuid.UUID]*Ticket)
	for _, t := range pending {
		if !t.IsParty() {
			units = append(units, matchUnit{tickets: []*Ticket{t}, seedAt: t.EnqueuedAt})
			continue
		}
		key := t.PartyKey()
		rosters[key] = t.PartyMembers
		members := byKeyPlayer[key]
		if members == nil {
			members = make(map[uuid.UUID]*Ticket)
			byKeyPlayer[key] = members
		}
		if cur, ok := members[t.PlayerID]; !ok || t.EnqueuedAt.Before(cur.EnqueuedAt) {
			members[t.PlayerID] = t
		}
	}

	for key, roster := range rosters {
		members := byKeyPlayer[key]
		tickets := make([]*Ticket, 0, len(roster))
		var seedAt time.Time
		complete := true
		for _, id := range roster {
			tk, ok := members[id]
			if !ok {
				complete = false
				break
			}
			tickets = append(tickets, tk)
			if seedAt.IsZero() || tk.EnqueuedAt.Before(seedAt) {
				seedAt = tk.EnqueuedAt
			}
		}
		if complete {
			units = append(units, matchUnit{tickets: tickets, seedAt: seedAt})
		}
	}

	// Oldest-first, with a deterministic tie-break so an equal-age pool selects reproducibly across replicas and runs.
	sort.SliceStable(units, func(i, j int) bool {
		if !units[i].seedAt.Equal(units[j].seedAt) {
			return units[i].seedAt.Before(units[j].seedAt)
		}
		return units[i].tickets[0].ID.String() < units[j].tickets[0].ID.String()
	})
	return units
}

func (m *Matcher) groupFromSeed(now time.Time, units []matchUnit, seedIdx int) []*Ticket {
	seed := units[seedIdx]
	if seed.size() > m.playersPerMatch {
		return nil // a party larger than a match can never be placed
	}
	anchor := seed.anchor()
	waited := now.Sub(anchor.EnqueuedAt)
	window := m.policy.BaseRatingWindow + m.policy.RatingWindowGrowthPerSec*waited.Seconds()
	ignoreRegion := waited >= m.policy.RegionRelaxAfter

	group := make([]*Ticket, 0, m.playersPerMatch)
	group = append(group, seed.tickets...)
	for j := range units {
		if j == seedIdx {
			continue
		}
		if len(group) == m.playersPerMatch {
			break
		}
		u := units[j]
		if len(group)+u.size() > m.playersPerMatch {
			continue // this unit would overflow the match; a smaller later unit may still fit the gap
		}
		if !unitCompatibleWith(u, anchor, window, ignoreRegion) {
			continue
		}
		group = append(group, u.tickets...)
	}
	// A party can leave the group short of a full match when no remaining units fit the gap; that is not a match, so wait.
	if len(group) != m.playersPerMatch {
		return nil
	}
	return group
}

// unitCompatibleWith requires every member of a candidate unit to be compatible with the seed anchor.
// A party's own internal rating/region spread is never checked — its members chose to play together — but mixing the party into a seed's match still respects that seed's tier.
func unitCompatibleWith(u matchUnit, anchor *Ticket, window float64, ignoreRegion bool) bool {
	for _, t := range u.tickets {
		if !anchor.CompatibleWith(t, window, ignoreRegion) {
			return false
		}
	}
	return true
}
