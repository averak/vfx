package assignmentstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/match"
	"github.com/averak/vfx/internal/infra/assignmentstore"
)

func assignment() *match.Assignment {
	return &match.Assignment{
		MatchID:      uuid.New(),
		Endpoint:     "localhost:7777",
		SessionToken: "tok",
		ExpiresAt:    time.Now().Add(time.Minute),
	}
}

func TestInMem_PutGetRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := assignmentstore.NewInMem()
	playerID := uuid.New()
	want := assignment()

	if err := store.Put(ctx, playerID, want, time.Minute); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := store.Get(ctx, playerID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.MatchID != want.MatchID || got.SessionToken != want.SessionToken {
		t.Errorf("Get = %+v, want %+v", got, want)
	}
}

// A player with no stored assignment is the normal "no current match"
// case, reported as (nil, nil) rather than an error.
func TestInMem_GetMissing(t *testing.T) {
	got, err := assignmentstore.NewInMem().Get(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Errorf("Get for an unknown player = %+v, want nil", got)
	}
}

// The store honours TTLs lazily on read: an entry whose deadline has
// already passed reads back as absent.
func TestInMem_ExpiredEntryReadsAbsent(t *testing.T) {
	ctx := context.Background()
	store := assignmentstore.NewInMem()
	playerID := uuid.New()

	if err := store.Put(ctx, playerID, assignment(), -time.Hour); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := store.Get(ctx, playerID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Errorf("expired entry returned %+v, want nil", got)
	}
}
