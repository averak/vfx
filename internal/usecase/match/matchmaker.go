package match

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/match"
	"github.com/averak/vfx/internal/infra/token"
	"github.com/averak/vfx/internal/stdx/clock"
)

// Matchmaker is the long-running worker that pairs queued tickets and
// reserves rooms via the Allocator.
//
// It pairs tickets by game mode using a rating window that widens with
// wait time and region constraints that relax after a deadline (see
// selectBatch). The queue's atomic Claim keeps pairing safe when the
// matchmaker runs on more than one replica.

// Metrics is the subset of telemetry the matchmaker emits. It is an
// interface so the usecase layer stays free of the concrete Prometheus
// registry; bootstrap supplies an adapter, tests use the no-op default.
type Metrics interface {
	MatchAllocated()
	SetQueueDepth(gameMode string, depth int)
}

type noopMetrics struct{}

func (noopMetrics) MatchAllocated()           {}
func (noopMetrics) SetQueueDepth(string, int) {}

type Matchmaker struct {
	queue       match.Queue
	allocator   match.Allocator
	signer      *token.Signer
	assignments match.AssignmentStore
	metrics     Metrics

	interval        time.Duration
	sessionTokenTTL time.Duration
	playersPerMatch int
	candidateModes  []string

	baseRatingWindow         float64
	ratingWindowGrowthPerSec float64
	regionRelaxAfter         time.Duration
}

// Config groups the matchmaker's tuning knobs.
type Config struct {
	Interval        time.Duration
	SessionTokenTTL time.Duration
	PlayersPerMatch int
	GameModes       []string

	// Tier-based matching. Two tickets may pair when their ratings are
	// within a window that starts at BaseRatingWindow and widens by
	// RatingWindowGrowthPerSec for every second the oldest of them has
	// waited. Region is enforced until that ticket has waited
	// RegionRelaxAfter, after which cross-region pairing is allowed.
	// Tickets without a rating or region skip the corresponding check.
	BaseRatingWindow         float64
	RatingWindowGrowthPerSec float64
	RegionRelaxAfter         time.Duration

	// Assignments persists each paired player's assignment so it can be
	// recovered via GetCurrentMatch. When nil, assignments are only
	// delivered over the live WatchTicket stream.
	Assignments match.AssignmentStore

	// Metrics is optional; when nil the matchmaker records nothing.
	Metrics Metrics
}

// NewMatchmaker constructs a Matchmaker. GameModes lists the modes the
// worker will scan each tick; an empty list disables matchmaking.
func NewMatchmaker(queue match.Queue, allocator match.Allocator, signer *token.Signer, cfg Config) *Matchmaker {
	if cfg.PlayersPerMatch == 0 {
		cfg.PlayersPerMatch = 2
	}
	if cfg.Interval == 0 {
		cfg.Interval = 200 * time.Millisecond
	}
	if cfg.Metrics == nil {
		cfg.Metrics = noopMetrics{}
	}
	if cfg.Assignments == nil {
		cfg.Assignments = noopAssignmentStore{}
	}
	if cfg.BaseRatingWindow == 0 {
		cfg.BaseRatingWindow = 100
	}
	if cfg.RatingWindowGrowthPerSec == 0 {
		cfg.RatingWindowGrowthPerSec = 50
	}
	if cfg.RegionRelaxAfter == 0 {
		cfg.RegionRelaxAfter = 15 * time.Second
	}
	return &Matchmaker{
		queue:                    queue,
		allocator:                allocator,
		signer:                   signer,
		assignments:              cfg.Assignments,
		metrics:                  cfg.Metrics,
		interval:                 cfg.Interval,
		sessionTokenTTL:          cfg.SessionTokenTTL,
		playersPerMatch:          cfg.PlayersPerMatch,
		candidateModes:           cfg.GameModes,
		baseRatingWindow:         cfg.BaseRatingWindow,
		ratingWindowGrowthPerSec: cfg.RatingWindowGrowthPerSec,
		regionRelaxAfter:         cfg.RegionRelaxAfter,
	}
}

// Run starts the matchmaker loop and returns when ctx is cancelled.
func (m *Matchmaker) Run(ctx context.Context) error {
	logger := slog.Default().With("worker", "matchmaker")
	logger.Info("matchmaker starting", "interval", m.interval, "modes", m.candidateModes)

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("matchmaker stopping")
			return nil
		case <-ticker.C:
			m.tick(ctx, logger)
		}
	}
}

func (m *Matchmaker) tick(ctx context.Context, logger *slog.Logger) {
	for _, mode := range m.candidateModes {
		if err := m.processMode(ctx, mode); err != nil {
			logger.Error("matchmaker tick failed", "mode", mode, "err", err)
		}
	}
}

func (m *Matchmaker) processMode(ctx context.Context, mode string) error {
	now := clock.Now(ctx)
	for {
		pending, err := m.queue.Pending(ctx, mode)
		if err != nil {
			return err
		}
		m.metrics.SetQueueDepth(mode, len(pending))
		if len(pending) < m.playersPerMatch {
			return nil
		}
		batch := m.selectBatch(now, pending)
		if batch == nil {
			// The longest-waiting ticket has no compatible group yet; its
			// tier widens on later ticks, so stop scanning this mode now.
			return nil
		}
		if err := m.pair(ctx, mode, batch); err != nil {
			return err
		}
	}
}

// selectBatch picks a compatible group of playersPerMatch tickets, or
// nil if none can be formed yet. pending is oldest-first; the group is
// seeded with the longest-waiting ticket so its (most-relaxed) tier
// governs who it can pair with, which keeps the oldest ticket from
// starving.
func (m *Matchmaker) selectBatch(now time.Time, pending []*match.Ticket) []*match.Ticket {
	// Try each ticket as the seed, oldest first, so the longest-waiting
	// ticket is preferred but a single incompatible outlier at the head
	// does not block compatible groups behind it from forming.
	for i := range pending {
		if group := m.groupFromSeed(now, pending, i); group != nil {
			return group
		}
	}
	return nil
}

// groupFromSeed builds a group governed by pending[seedIdx]'s tier,
// drawing the remaining members from anywhere else in the pool. Returns
// nil if a full group cannot be formed around that seed.
func (m *Matchmaker) groupFromSeed(now time.Time, pending []*match.Ticket, seedIdx int) []*match.Ticket {
	seed := pending[seedIdx]
	waited := now.Sub(seed.CreatedAt)
	window := m.baseRatingWindow + m.ratingWindowGrowthPerSec*waited.Seconds()
	ignoreRegion := waited >= m.regionRelaxAfter

	group := make([]*match.Ticket, 0, m.playersPerMatch)
	group = append(group, seed)
	for j, t := range pending {
		if j == seedIdx {
			continue
		}
		if len(group) == m.playersPerMatch {
			break
		}
		if compatible(seed, t, window, ignoreRegion) {
			group = append(group, t)
		}
	}
	if len(group) < m.playersPerMatch {
		return nil
	}
	return group
}

// compatible reports whether other may join seed's group under the
// current tier. A missing rating or region on either ticket skips that
// dimension's check rather than blocking the match.
func compatible(seed, other *match.Ticket, ratingWindow float64, ignoreRegion bool) bool {
	if !ignoreRegion && seed.Region != nil && other.Region != nil && *seed.Region != *other.Region {
		return false
	}
	if seed.Rating != nil && other.Rating != nil {
		if math.Abs(*seed.Rating-*other.Rating) > ratingWindow {
			return false
		}
	}
	return true
}

func (m *Matchmaker) pair(ctx context.Context, mode string, tickets []*match.Ticket) error {
	// Reserve the whole group atomically so a second gateway's matchmaker
	// cannot pair any of these tickets into a different match. If the
	// claim loses the race, abandon this grouping; the next tick re-reads
	// the pending pool without the tickets the winner took.
	ids := make([]uuid.UUID, len(tickets))
	for i, t := range tickets {
		ids[i] = t.ID
	}
	claimed, err := m.queue.Claim(ctx, mode, ids)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}

	allocation, err := m.allocator.Allocate(ctx, mode, len(tickets))
	if err != nil {
		// Notify each ticket and let the player retry.
		for _, t := range tickets {
			//nolint:errcheck // Best-effort notification; we already return err.
			_ = m.queue.Publish(ctx, t.ID, match.EventFailed{
				Reason:  "allocator_failed",
				Message: err.Error(),
			})
		}
		return err
	}

	now := clock.Now(ctx)
	expiresAt := now.Add(m.sessionTokenTTL)

	matchPlayers := make([]string, 0, len(tickets))
	for _, t := range tickets {
		matchPlayers = append(matchPlayers, t.PlayerID.String())
	}

	for _, t := range tickets {
		sessionToken, signErr := m.signer.SignSession(t.PlayerID, allocation.MatchID, matchPlayers, now, m.sessionTokenTTL)
		if signErr != nil {
			// Tell the player it failed; do not leak which signing step broke.
			//nolint:errcheck // Best-effort notification; we already return err.
			_ = m.queue.Publish(ctx, t.ID, match.EventFailed{
				Reason:  "internal",
				Message: "failed to issue session token",
			})
			continue
		}

		assignment := &match.Assignment{
			MatchID:      uuidMustParse(allocation.MatchID),
			Endpoint:     allocation.Endpoint,
			SessionToken: sessionToken,
			ExpiresAt:    expiresAt,
		}

		// Persist before publishing so a client that reads the live
		// Matched event and immediately calls GetCurrentMatch (e.g. after
		// a reconnect) cannot race ahead of the store.
		if storeErr := m.assignments.Put(ctx, t.PlayerID, assignment, m.sessionTokenTTL); storeErr != nil {
			logger := slog.Default().With("worker", "matchmaker")
			logger.Error("failed to persist assignment", "player_id", t.PlayerID, "err", storeErr)
		}

		//nolint:errcheck // Best-effort notification; subscriber may have dropped.
		_ = m.queue.Publish(ctx, t.ID, match.EventMatched{Assignment: assignment})
	}
	m.metrics.MatchAllocated()
	return nil
}

// uuidMustParse is local to the matchmaker because the allocation
// carries the match id as a string for convenience; we always want a
// uuid.UUID once we hand it on to the rest of the system.
func uuidMustParse(s string) uuid.UUID {
	id, err := uuid.Parse(s)
	if err != nil {
		// allocator must produce valid UUIDs; a bad value is a bug.
		panic("matchmaker: allocator returned invalid match id: " + s)
	}
	return id
}
