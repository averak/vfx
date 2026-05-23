package admin_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/match"
	"github.com/averak/vfx/internal/domain/player"
	"github.com/averak/vfx/internal/usecase/admin"
)

// roInline runs the work without a real database; the usecase only needs
// the boundary, not a transaction.
type roInline struct{}

func (roInline) RO(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

// fakePlayerRepo embeds the interface so only the method under test needs
// a body; the rest panic if unexpectedly called.
type fakePlayerRepo struct {
	player.Repository
	p   *player.Player
	err error
}

func (f fakePlayerRepo) GetByID(context.Context, uuid.UUID) (*player.Player, error) {
	return f.p, f.err
}

type fakeQueue struct {
	match.Queue
	depth int32
	err   error
}

func (f fakeQueue) Depth(context.Context, string) (int32, error) { return f.depth, f.err }

func TestGetPlayer_ReturnsPlayer(t *testing.T) {
	want, err := player.New(uuid.New(), nil, time.Now())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	uc := admin.New(roInline{}, fakePlayerRepo{p: want}, fakeQueue{})

	got, err := uc.GetPlayer(t.Context(), want.ID)
	if err != nil {
		t.Fatalf("GetPlayer: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("GetPlayer id = %v, want %v", got.ID, want.ID)
	}
}

func TestGetPlayer_PropagatesNotFound(t *testing.T) {
	uc := admin.New(roInline{}, fakePlayerRepo{err: player.ErrPlayerNotFound}, fakeQueue{})
	if _, err := uc.GetPlayer(t.Context(), uuid.New()); !errors.Is(err, player.ErrPlayerNotFound) {
		t.Errorf("err = %v, want ErrPlayerNotFound", err)
	}
}

func TestQueueDepth(t *testing.T) {
	uc := admin.New(roInline{}, fakePlayerRepo{}, fakeQueue{depth: 7})
	depth, err := uc.QueueDepth(t.Context(), "rps")
	if err != nil {
		t.Fatalf("QueueDepth: %v", err)
	}
	if depth != 7 {
		t.Errorf("QueueDepth = %d, want 7", depth)
	}
}
