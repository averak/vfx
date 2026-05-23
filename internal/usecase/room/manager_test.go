package room_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	pluginv1 "github.com/averak/vfx/gen/go/plugin/v1"
	"github.com/averak/vfx/internal/domain/plugin"
	room "github.com/averak/vfx/internal/usecase/room"
)

type fakeFactory struct{}

func (fakeFactory) Name() string { return "fake" }

func (fakeFactory) Create(context.Context) (plugin.Plugin, error) { return fakePlugin{}, nil }

// fakePlugin never ends the game, so a match it backs stays alive until
// the manager's context is cancelled.
type fakePlugin struct{}

func (fakePlugin) Init(context.Context, *pluginv1.InitRequest) (*pluginv1.InitResponse, error) {
	return &pluginv1.InitResponse{TickRateHz: 0}, nil
}

func (fakePlugin) OnTick(context.Context, *pluginv1.OnTickRequest) (*pluginv1.OnTickResponse, error) {
	return &pluginv1.OnTickResponse{}, nil
}

func (fakePlugin) OnGameEnd(context.Context, *pluginv1.OnGameEndRequest) (*pluginv1.OnGameEndResponse, error) {
	return &pluginv1.OnGameEndResponse{}, nil
}

func (fakePlugin) Close() error { return nil }

func newManager(ctx context.Context) *room.Manager {
	return room.NewManager(ctx, fakeFactory{}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
}

// A second player connecting to the same match must join the instance the
// first connection lazily created, not spin up a parallel match.
func TestManager_FindOrCreateDedupsByMatchID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr := newManager(ctx)
	defer mgr.Close()

	matchID := uuid.New()
	players := []uuid.UUID{uuid.New(), uuid.New()}

	first, err := mgr.FindOrCreate(ctx, matchID, players)
	if err != nil {
		t.Fatalf("first FindOrCreate: %v", err)
	}
	second, err := mgr.FindOrCreate(ctx, matchID, players)
	if err != nil {
		t.Fatalf("second FindOrCreate: %v", err)
	}
	if first != second {
		t.Error("FindOrCreate created a second instance for the same match id")
	}
	if mgr.Count() != 1 {
		t.Errorf("Count = %d, want 1", mgr.Count())
	}

	other, err := mgr.FindOrCreate(ctx, uuid.New(), players)
	if err != nil {
		t.Fatalf("third FindOrCreate: %v", err)
	}
	if other == first {
		t.Error("a different match id reused an existing instance")
	}
	if mgr.Count() != 2 {
		t.Errorf("Count = %d, want 2", mgr.Count())
	}
}

func TestManager_GetUnknownMatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr := newManager(ctx)
	defer mgr.Close()

	if _, ok := mgr.Get(uuid.New()); ok {
		t.Error("Get reported a match that was never created")
	}
}
