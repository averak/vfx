// Package assignmentstore holds implementations of the
// match.AssignmentStore contract.
//
// Valkey is the shared-state backend: a player's current assignment is
// written under a per-player key with a TTL, so any gateway replica can
// answer GetCurrentMatch and a reconnecting client recovers its room
// without re-queuing. InMem is the single-process fallback used in
// tests and single-node deployments.
package assignmentstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	valkeygo "github.com/valkey-io/valkey-go"

	"github.com/averak/vfx/internal/domain/match"
)

// keyPrefix namespaces assignment keys so they never collide with other
// vfx state sharing the same Valkey database.
const keyPrefix = "vfx:assignment:"

// Valkey persists assignments in a Valkey/Redis-compatible store.
type Valkey struct {
	client valkeygo.Client
}

var _ match.AssignmentStore = (*Valkey)(nil)

// NewValkey wraps a connected client.
func NewValkey(client valkeygo.Client) *Valkey {
	return &Valkey{client: client}
}

// assignmentDTO is the wire form written to Valkey. Keeping it separate
// from the domain type means a field rename in match.Assignment does not
// silently change the persisted schema.
type assignmentDTO struct {
	MatchID      string    `json:"match_id"`
	Endpoint     string    `json:"endpoint"`
	SessionToken string    `json:"session_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func (s *Valkey) Put(ctx context.Context, playerID uuid.UUID, a *match.Assignment, ttl time.Duration) error {
	// Persisting the session token is the point of this store: a
	// reconnecting client recovers the exact token it needs for the room.
	// The token is short-lived (TTL below) and the key expires with it.
	payload, err := json.Marshal(assignmentDTO{ //nolint:gosec // G117: storing the session token is intentional and TTL-bounded.
		MatchID:      a.MatchID.String(),
		Endpoint:     a.Endpoint,
		SessionToken: a.SessionToken,
		ExpiresAt:    a.ExpiresAt,
	})
	if err != nil {
		return fmt.Errorf("assignmentstore: marshal: %w", err)
	}

	// Round the TTL up to whole seconds; a sub-second floor would expire
	// the key before the token it guards.
	seconds := int64(ttl.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	cmd := s.client.B().Set().Key(key(playerID)).Value(string(payload)).ExSeconds(seconds).Build()
	if err := s.client.Do(ctx, cmd).Error(); err != nil {
		return fmt.Errorf("assignmentstore: set: %w", err)
	}
	return nil
}

func (s *Valkey) Get(ctx context.Context, playerID uuid.UUID) (*match.Assignment, error) {
	cmd := s.client.B().Get().Key(key(playerID)).Build()
	raw, err := s.client.Do(ctx, cmd).ToString()
	if err != nil {
		if valkeygo.IsValkeyNil(err) {
			return nil, nil //nolint:nilnil // "no current match" is an absent key, not an error.
		}
		return nil, fmt.Errorf("assignmentstore: get: %w", err)
	}

	var dto assignmentDTO
	if err := json.Unmarshal([]byte(raw), &dto); err != nil {
		return nil, fmt.Errorf("assignmentstore: unmarshal: %w", err)
	}
	matchID, parseErr := uuid.Parse(dto.MatchID)
	if parseErr != nil {
		return nil, fmt.Errorf("assignmentstore: parse match id: %w", parseErr)
	}
	return &match.Assignment{
		MatchID:      matchID,
		Endpoint:     dto.Endpoint,
		SessionToken: dto.SessionToken,
		ExpiresAt:    dto.ExpiresAt,
	}, nil
}

func key(playerID uuid.UUID) string { return keyPrefix + playerID.String() }
