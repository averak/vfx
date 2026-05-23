// Package chat orchestrates the ChatService (direct messages).
package chat

import (
	"context"
	"time"

	"github.com/google/uuid"

	domainchat "github.com/averak/vfx/internal/domain/chat"
	"github.com/averak/vfx/internal/stdx/clock"
	"github.com/averak/vfx/internal/usecase/tx"
)

type Config struct {
	// DefaultLimit applies when a history request omits the limit; MaxLimit caps it.
	DefaultLimit int
	MaxLimit     int
}

type Usecase struct {
	rw   tx.ReadWriter
	ro   tx.Reader
	repo domainchat.Repository
	cfg  Config
}

func New(rw tx.ReadWriter, ro tx.Reader, repo domainchat.Repository, cfg Config) *Usecase {
	return &Usecase{rw: rw, ro: ro, repo: repo, cfg: cfg}
}

// SendDirectMessage validates and stores a message from sender to recipient.
// It returns domainchat.ErrSelfMessage / ErrInvalidBody for bad input.
func (u *Usecase) SendDirectMessage(ctx context.Context, sender, recipient uuid.UUID, body string) (*domainchat.Message, error) {
	msg, err := domainchat.NewMessage(uuid.New(), sender, recipient, body, clock.Now(ctx))
	if err != nil {
		return nil, err
	}
	if err := u.rw.RW(ctx, func(ctx context.Context) error {
		return u.repo.Save(ctx, msg)
	}); err != nil {
		return nil, err
	}
	return msg, nil
}

// ListDirectMessages returns the conversation between me and other, newest-first, older than before (zero means latest).
func (u *Usecase) ListDirectMessages(ctx context.Context, me, other uuid.UUID, before time.Time, limit int) ([]*domainchat.Message, error) {
	limit = u.clampLimit(limit)
	var messages []*domainchat.Message
	err := u.ro.RO(ctx, func(ctx context.Context) error {
		var err error
		messages, err = u.repo.ListConversation(ctx, me, other, before, limit)
		return err
	})
	return messages, err
}

func (u *Usecase) clampLimit(limit int) int {
	if limit <= 0 {
		return u.cfg.DefaultLimit
	}
	if limit > u.cfg.MaxLimit {
		return u.cfg.MaxLimit
	}
	return limit
}
