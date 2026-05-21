// Package admin orchestrates the operations API: read-only views over
// players and the matchmaking queue for operators. It deliberately holds
// no mutation paths — the gateway owns all state changes.
package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/averak/vfx/internal/domain/match"
	"github.com/averak/vfx/internal/domain/player"
	"github.com/averak/vfx/internal/stdx/db"
)

// Usecase exposes the operational queries to the admin handler.
type Usecase struct {
	session    *db.Session
	playerRepo player.Repository
	queue      match.Queue
}

// New wires the usecase.
func New(session *db.Session, playerRepo player.Repository, queue match.Queue) *Usecase {
	return &Usecase{session: session, playerRepo: playerRepo, queue: queue}
}

// GetPlayer looks up a player by id. It returns player.ErrPlayerNotFound
// when the id is unknown.
func (u *Usecase) GetPlayer(ctx context.Context, id uuid.UUID) (*player.Player, error) {
	var p *player.Player
	err := u.session.RO(ctx, func(ctx context.Context, tx pgx.Tx) error {
		found, e := u.playerRepo.GetByID(ctx, tx, id)
		if e != nil {
			return e
		}
		p = found
		return nil
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

// QueueDepth reports how many tickets are waiting in the given game mode.
func (u *Usecase) QueueDepth(ctx context.Context, gameMode string) (int32, error) {
	return u.queue.Depth(ctx, gameMode)
}
