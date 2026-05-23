package assignmentstore

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/match"
)

// InMem is a single-process AssignmentStore.
// It honours TTLs with a per-entry expiry checked lazily on read, which is enough for tests and single-node deployments where there is no second replica to serve.
type InMem struct {
	mu      sync.Mutex
	entries map[uuid.UUID]inMemEntry
}

type inMemEntry struct {
	assignment match.Assignment
	expiresAt  time.Time
}

var _ match.AssignmentStore = (*InMem)(nil)

func NewInMem() *InMem {
	return &InMem{entries: make(map[uuid.UUID]inMemEntry)}
}

func (s *InMem) Put(_ context.Context, playerID uuid.UUID, a *match.Assignment, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[playerID] = inMemEntry{
		assignment: *a,
		expiresAt:  time.Now().Add(ttl),
	}
	return nil
}

func (s *InMem) Get(_ context.Context, playerID uuid.UUID) (*match.Assignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[playerID]
	if !ok {
		return nil, nil //nolint:nilnil // "no current match" is the absence of an entry.
	}
	if time.Now().After(entry.expiresAt) {
		delete(s.entries, playerID)
		return nil, nil //nolint:nilnil // expired entries read as "no current match".
	}
	a := entry.assignment
	return &a, nil
}
