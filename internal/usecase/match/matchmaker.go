package match

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/match"
	"github.com/averak/vfx/internal/infra/token"
	"github.com/averak/vfx/internal/stdx/clock"
)

// Matchmaker is the long-running worker that pairs queued tickets and
// reserves rooms via the Allocator.
//
// Phase 1 uses the simplest possible policy: as soon as two tickets in
// the same game_mode are waiting, pair them up. Rating/region/party
// hints on Ticket are recorded but not yet used for filtering or tier
// relaxation; that lands when the rock-paper-scissors example demands it.
type Matchmaker struct {
	queue     match.Queue
	allocator match.Allocator
	signer    *token.Signer

	interval        time.Duration
	sessionTokenTTL time.Duration
	playersPerMatch int
	candidateModes  []string
}

// Config groups the matchmaker's tuning knobs.
type Config struct {
	Interval        time.Duration
	SessionTokenTTL time.Duration
	PlayersPerMatch int
	GameModes       []string
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
	return &Matchmaker{
		queue:           queue,
		allocator:       allocator,
		signer:          signer,
		interval:        cfg.Interval,
		sessionTokenTTL: cfg.SessionTokenTTL,
		playersPerMatch: cfg.PlayersPerMatch,
		candidateModes:  cfg.GameModes,
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
	for {
		pending, err := m.queue.Pending(ctx, mode)
		if err != nil {
			return err
		}
		if len(pending) < m.playersPerMatch {
			return nil
		}
		batch := pending[:m.playersPerMatch]
		if err := m.pair(ctx, mode, batch); err != nil {
			return err
		}
	}
}

func (m *Matchmaker) pair(ctx context.Context, mode string, tickets []*match.Ticket) error {
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
		//nolint:errcheck // Best-effort notification; subscriber may have dropped.
		_ = m.queue.Publish(ctx, t.ID, match.EventMatched{Assignment: assignment})
	}
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
