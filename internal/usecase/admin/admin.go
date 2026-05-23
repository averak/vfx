// Package admin orchestrates the operations API: read-only views over players and the matchmaking queue for operators.
// It deliberately holds no mutation paths; the gateway owns all state changes.
package admin

import (
	"context"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/match"
	"github.com/averak/vfx/internal/domain/player"
)

// Transactor runs work inside a read-only transaction.
// The admin API never mutates, so it needs only RO; the implementation puts the transaction on the context the repository reads from.
type Transactor interface {
	RO(ctx context.Context, fn func(context.Context) error) error
}

type Usecase struct {
	tx         Transactor
	playerRepo player.Repository
	queue      match.Queue
}

func New(tx Transactor, playerRepo player.Repository, queue match.Queue) *Usecase {
	return &Usecase{tx: tx, playerRepo: playerRepo, queue: queue}
}

func (u *Usecase) GetPlayer(ctx context.Context, id uuid.UUID) (*player.Player, error) {
	var p *player.Player
	err := u.tx.RO(ctx, func(ctx context.Context) error {
		found, e := u.playerRepo.GetByID(ctx, id)
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

func (u *Usecase) QueueDepth(ctx context.Context, gameMode string) (int32, error) {
	return u.queue.Depth(ctx, gameMode)
}
