package room

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	pluginv1 "github.com/averak/vfx/gen/go/plugin/v1"
	"github.com/averak/vfx/internal/domain/plugin"
)

// tracer is the room usecase instrumentation scope; spans are no-ops until tracing.Setup installs an exporter.
var tracer = otel.Tracer("github.com/averak/vfx/internal/usecase/room")

// Manager tracks the room daemon's active matches.
// The WebTransport handler calls FindOrCreate when a player connects, so the first arrival lazily creates the match and later arrivals join the same one.
//
// The manager owns a long-lived context (the daemon's) that it passes to every Match.Run goroutine, so a player's HTTP request cancellation cannot tear down a match other players are still in.
type Manager struct {
	logger    *slog.Logger
	factory   plugin.Factory
	metrics   Metrics
	matchCtx  context.Context //nolint:containedctx // Long-lived per-daemon context; never per-request.
	cancelAll context.CancelFunc

	mu      sync.Mutex
	matches map[uuid.UUID]*Match
}

// NewManager's ctx outlives every Match it creates; cancel it (typically at daemon shutdown) to tear them all down.
// A nil metrics records nothing.
func NewManager(ctx context.Context, factory plugin.Factory, logger *slog.Logger, metrics Metrics) *Manager {
	if metrics == nil {
		metrics = noopMetrics{}
	}
	mctx, cancel := context.WithCancel(ctx)
	return &Manager{
		logger:    logger,
		factory:   factory,
		metrics:   metrics,
		matchCtx:  mctx,
		cancelAll: cancel,
		matches:   make(map[uuid.UUID]*Match),
	}
}

// Close cancels every running match and is safe to call multiple times.
func (mgr *Manager) Close() { mgr.cancelAll() }

// FindOrCreate returns the active Match for matchID, creating one if needed.
func (mgr *Manager) FindOrCreate(ctx context.Context, matchID uuid.UUID, players []uuid.UUID) (*Match, error) {
	mgr.mu.Lock()
	if existing, ok := mgr.matches[matchID]; ok {
		mgr.mu.Unlock()
		return existing, nil
	}
	mgr.mu.Unlock()

	// Span covers the cold-start cost: instantiating the plugin and running its Init.
	ctx, span := tracer.Start(ctx, "room.match.create", trace.WithAttributes(
		attribute.String("vfx.match_id", matchID.String()),
		attribute.Int("vfx.player_count", len(players)),
	))
	defer span.End()

	// Build the plugin outside the lock: a cold start can be slow, and holding the lock through it would stall every other join, Get, and cleanup.
	// Two arrivals for the same new match may both build one; the re-check below keeps the first and discards the loser.
	pl, err := mgr.factory.Create(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "plugin create failed")
		return nil, err
	}
	initReq := &pluginv1.InitRequest{}
	for _, id := range players {
		initReq.PlayerIds = append(initReq.PlayerIds, id.String())
	}
	initResp, err := pl.Init(ctx, initReq)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "plugin init failed")
		if closeErr := pl.Close(); closeErr != nil {
			mgr.logger.Warn("manager: close after init failure", "err", closeErr)
		}
		return nil, err
	}
	match := NewMatch(matchID, pl, initResp.GetTickRateHz(), mgr.logger, mgr.metrics)

	mgr.mu.Lock()
	if existing, ok := mgr.matches[matchID]; ok {
		mgr.mu.Unlock()
		if closeErr := pl.Close(); closeErr != nil {
			mgr.logger.Warn("manager: close discarded race loser", "err", closeErr)
		}
		return existing, nil
	}
	mgr.matches[matchID] = match
	mgr.metrics.IncActiveMatches()
	mgr.mu.Unlock()

	// match.Run uses the manager's long-lived ctx, never the caller's request ctx: a player disconnecting must not tear down the whole match for everyone else.
	go func() {
		if err := match.Run(mgr.matchCtx); err != nil && !errors.Is(err, context.Canceled) {
			mgr.logger.Error("match run failed", "match_id", matchID, "err", err)
		}
		mgr.cleanup(matchID, match)
		mgr.metrics.DecActiveMatches()
	}()

	return match, nil
}

// cleanup removes the match only if it is still the instance self, so a finished match's late cleanup cannot evict a newer one that reused the id.
func (mgr *Manager) cleanup(matchID uuid.UUID, self *Match) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	if m, ok := mgr.matches[matchID]; ok && m == self {
		delete(mgr.matches, matchID)
	}
}

func (mgr *Manager) Get(matchID uuid.UUID) (*Match, bool) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	m, ok := mgr.matches[matchID]
	return m, ok
}

func (mgr *Manager) Count() int {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	return len(mgr.matches)
}
