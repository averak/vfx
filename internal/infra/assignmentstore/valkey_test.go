package assignmentstore_test

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	domainmatch "github.com/averak/vfx/internal/domain/match"
	"github.com/averak/vfx/internal/infra/assignmentstore"
	"github.com/averak/vfx/internal/infra/valkey"
)

// newValkeyStore connects to the Valkey named by VALKEY_URL, skipping
// the test when it is unset so pure-logic runs and machines without a
// Valkey still pass.
func newValkeyStore(t *testing.T) *assignmentstore.Valkey {
	t.Helper()
	url := os.Getenv("VALKEY_URL")
	if url == "" {
		t.Skip("VALKEY_URL not set; skipping Valkey-backed test")
	}
	client, err := valkey.NewClient(url)
	if err != nil {
		t.Fatalf("connect valkey: %v", err)
	}
	t.Cleanup(client.Close)
	return assignmentstore.NewValkey(client)
}

func TestValkey_RoundTrip(t *testing.T) {
	store := newValkeyStore(t)
	playerID := uuid.New()
	want := &domainmatch.Assignment{
		MatchID:      uuid.New(),
		Endpoint:     "room:7777",
		SessionToken: "tok-abc",
		ExpiresAt:    time.Now().Add(time.Minute).UTC().Truncate(time.Second),
	}

	if err := store.Put(t.Context(), playerID, want, time.Minute); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := store.Get(t.Context(), playerID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil after Put")
	}
	if got.MatchID != want.MatchID || got.Endpoint != want.Endpoint ||
		got.SessionToken != want.SessionToken || !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", got, want)
	}
}

func TestValkey_GetMissingReturnsNil(t *testing.T) {
	store := newValkeyStore(t)

	got, err := store.Get(t.Context(), uuid.New())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Errorf("Get for an unknown player = %+v, want nil", got)
	}
}

func TestValkey_PutOverwrites(t *testing.T) {
	store := newValkeyStore(t)
	playerID := uuid.New()

	first := &domainmatch.Assignment{MatchID: uuid.New(), Endpoint: "room:1", SessionToken: "a"}
	second := &domainmatch.Assignment{MatchID: uuid.New(), Endpoint: "room:2", SessionToken: "b"}
	if err := store.Put(t.Context(), playerID, first, time.Minute); err != nil {
		t.Fatalf("Put first: %v", err)
	}
	if err := store.Put(t.Context(), playerID, second, time.Minute); err != nil {
		t.Fatalf("Put second: %v", err)
	}

	got, err := store.Get(t.Context(), playerID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.MatchID != second.MatchID {
		t.Errorf("after overwrite got %+v, want match %s", got, second.MatchID)
	}
}
