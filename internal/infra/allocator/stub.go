// Package allocator implements [match.Allocator].
//
// Stub does not talk to a real orchestrator: every Allocate returns the
// same configured endpoint and a fresh match id. It is used for local
// and compose runs where a single room daemon serves every match; the
// Agones allocator is used in a cluster.
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
		MatchID:  uuid.New(),
		Endpoint: s.endpoint,
	}, nil
}
