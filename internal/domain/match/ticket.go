// Package match is the matchmaking domain.
// The matchmaking rules themselves — pairing eligibility and tier relaxation — live here (Ticket.CompatibleWith, Matcher), not in the usecase, because they are intrinsic to matchmaking rather than to any one application flow.
package match

import (
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrTicketNoPlayer   = errors.New("match: ticket requires a player")
	ErrTicketNoGameMode = errors.New("match: ticket requires a game mode")
	ErrPartyTooLarge    = errors.New("match: party exceeds the match size")
)

type Ticket struct {
	ID       uuid.UUID
	PlayerID uuid.UUID
	GameMode string

	Rating       *float64
	Region       *string
	PartyMembers []uuid.UUID
	Attributes   map[string]string

	CreatedAt time.Time
}

func NewTicket(id, playerID uuid.UUID, gameMode string, now time.Time) (*Ticket, error) {
	if playerID == uuid.Nil {
		return nil, ErrTicketNoPlayer
	}
	if gameMode == "" {
		return nil, ErrTicketNoGameMode
	}
	return &Ticket{
		ID:        id,
		PlayerID:  playerID,
		GameMode:  gameMode,
		CreatedAt: now,
	}, nil
}

// IsParty reports whether the ticket queues a party (a roster of two or more) rather than a solo player.
func (t *Ticket) IsParty() bool { return len(t.PartyMembers) >= 2 }

// PartyKey identifies the party a ticket belongs to: the canonical roster that every member's ticket shares.
// Solo tickets return "". The matchmaker groups party tickets by this key.
func (t *Ticket) PartyKey() string {
	if !t.IsParty() {
		return ""
	}
	ids := make([]string, len(t.PartyMembers))
	for i, m := range t.PartyMembers {
		ids[i] = m.String()
	}
	sort.Strings(ids)
	return strings.Join(ids, ",")
}

// NormalizeParty returns the canonical party roster for a queue request: the submitter plus the requested members, deduplicated and sorted.
// It returns nil for a solo queue (no members, or only the submitter), so a party is unambiguously a roster of two or more, and every member's ticket carries an identical roster regardless of who they listed or in what order.
func NormalizeParty(self uuid.UUID, members []uuid.UUID) []uuid.UUID {
	set := make(map[uuid.UUID]struct{}, len(members)+1)
	set[self] = struct{}{}
	for _, m := range members {
		set[m] = struct{}{}
	}
	if len(set) <= 1 {
		return nil
	}
	roster := make([]uuid.UUID, 0, len(set))
	for id := range set {
		roster = append(roster, id)
	}
	sort.Slice(roster, func(i, j int) bool { return roster[i].String() < roster[j].String() })
	return roster
}

// CompatibleWith treats a missing rating or region as "skip that dimension", not as a mismatch.
func (t *Ticket) CompatibleWith(other *Ticket, ratingWindow float64, ignoreRegion bool) bool {
	if !ignoreRegion && t.Region != nil && other.Region != nil && *t.Region != *other.Region {
		return false
	}
	if t.Rating != nil && other.Rating != nil {
		if math.Abs(*t.Rating-*other.Rating) > ratingWindow {
			return false
		}
	}
	return true
}
