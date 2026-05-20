package room

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/google/uuid"

	pluginv1 "github.com/averak/vfx/gen/go/plugin/v1"
	"github.com/averak/vfx/internal/domain/plugin"
)

// Manager tracks the room daemon's currently active matches. The
// WebTransport handler calls FindOrCreate when a player connects so
// the first arrival lazily creates the match, and subsequent arrivals
// join the same one.
//
// The manager owns a long-lived context (the daemon's context) that is
// passed to every Match.Run goroutine; this prevents a player's HTTP
// request cancellation from tearing down a match that other players
// are still part of.
type Manager struct {
	logger    *slog.Logger
	factory   plugin.Factory
	matchCtx  context.Context //nolint:containedctx // Long-lived per-daemon context; never per-request.
	cancelAll context.CancelFunc

	mu      sync.Mutex
	matches map[uuid.UUID]*Match
}

// NewManager constructs a Manager wired to a Factory. The supplied ctx
// outlives every Match the manager creates; cancel it (typically at
// daemon shutdown) to tear them all down.
func NewManager(ctx context.Context, factory plugin.Factory, logger *slog.Logger) *Manager {
	mctx, cancel := context.WithCancel(ctx)
	return &Manager{
		logger:    logger,
		factory:   factory,
		matchCtx:  mctx,
		cancelAll: cancel,
		matches:   make(map[uuid.UUID]*Match),
	}
}

// Close cancels every running match. Safe to call multiple times.
func (mgr *Manager) Close() { mgr.cancelAll() }

// FindOrCreate returns the active Match for matchID, creating one if
// needed. Creation is serialised under the manager lock so two
// simultaneous joins for a new match still see the same instance.
func (mgr *Manager) FindOrCreate(ctx context.Context, matchID uuid.UUID, players []uuid.UUID) (*Match, error) {
	mgr.mu.Lock()
	if existing, ok := mgr.matches[matchID]; ok {
		mgr.mu.Unlock()
		return existing, nil
	}

	pl, err := mgr.factory.Create(ctx)
	if err != nil {
		mgr.mu.Unlock()
		return nil, err
	}

	initReq := &pluginv1.InitRequest{}
	for _, id := range players {
		initReq.PlayerIds = append(initReq.PlayerIds, id.String())
	}
	initResp, err := pl.Init(ctx, initReq)
	if err != nil {
		if closeErr := pl.Close(); closeErr != nil {
			mgr.logger.Warn("manager: close after init failure", "err", closeErr)
		}
		mgr.mu.Unlock()
		return nil, err
	}

	match := NewMatch(matchID, pl, initResp.GetTickRateHz(), mgr.logger)
	mgr.matches[matchID] = match

	// match.Run uses the manager's long-lived ctx, never the caller's
	// request ctx — a player disconnecting must not tear down the
	// whole match for everyone else.
	go func() {
		if err := match.Run(mgr.matchCtx); err != nil && !errors.Is(err, context.Canceled) {
			mgr.logger.Error("match run failed", "match_id", matchID, "err", err)
		}
		mgr.cleanup(matchID)
	}()

	mgr.mu.Unlock()
	return match, nil
}

func (mgr *Manager) cleanup(matchID uuid.UUID) {
	mgr.mu.Lock()
	delete(mgr.matches, matchID)
	mgr.mu.Unlock()
}

// Get returns the active Match for matchID without creating one.
func (mgr *Manager) Get(matchID uuid.UUID) (*Match, bool) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	m, ok := mgr.matches[matchID]
	return m, ok
}

// Count returns the number of matches currently running. Useful as a
// metric source and in health probes.
func (mgr *Manager) Count() int {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	return len(mgr.matches)
}
