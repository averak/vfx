// Package allocator implements [match.Allocator].
//
// Stub is the Phase 1 implementation: it does not talk to a real
// orchestrator. Every Allocate returns the same hardcoded endpoint and
// a fresh match id, which is enough to demonstrate the end-to-end
// gateway flow before the room daemon and Agones integration are in
// place.
package allocator

import (
	"context"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/match"
)

// Stub satisfies match.Allocator without contacting any external system.
type Stub struct {
	endpoint string
}

var _ match.Allocator = (*Stub)(nil)

// NewStub returns a Stub configured to direct clients at endpoint.
func NewStub(endpoint string) *Stub {
	return &Stub{endpoint: endpoint}
}

func (s *Stub) Allocate(_ context.Context, _ string, _ int) (*match.RoomAllocation, error) {
	return &match.RoomAllocation{
		MatchID:  uuid.NewString(),
		Endpoint: s.endpoint,
	}, nil
}
