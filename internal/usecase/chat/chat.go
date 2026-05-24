// Package chat orchestrates the ChatService (direct messages and channel/group chat).
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

// Membership reports whether a player belongs to a channel; a channel is a group, so the group repository satisfies this.
type Membership interface {
	IsMember(ctx context.Context, channelID, playerID uuid.UUID) (bool, error)
}

type Usecase struct {
	rw      tx.ReadWriter
	ro      tx.Reader
	repo    domainchat.Repository
	members Membership
	cfg     Config
}

func New(rw tx.ReadWriter, ro tx.Reader, repo domainchat.Repository, members Membership, cfg Config) *Usecase {
	return &Usecase{rw: rw, ro: ro, repo: repo, members: members, cfg: cfg}
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

// SendChannelMessage stores a message in a channel after checking the sender is a member.
func (u *Usecase) SendChannelMessage(ctx context.Context, sender, channelID uuid.UUID, body string) (*domainchat.ChannelMessage, error) {
	msg, err := domainchat.NewChannelMessage(uuid.New(), channelID, sender, body, clock.Now(ctx))
	if err != nil {
		return nil, err
	}
	if err := u.rw.RW(ctx, func(ctx context.Context) error {
		member, err := u.members.IsMember(ctx, channelID, sender)
		if err != nil {
			return err
		}
		if !member {
			return domainchat.ErrNotChannelMember
		}
		return u.repo.SaveChannelMessage(ctx, msg)
	}); err != nil {
		return nil, err
	}
	return msg, nil
}

// ListChannelMessages returns a channel's history newest-first; the caller must be a member.
func (u *Usecase) ListChannelMessages(ctx context.Context, me, channelID uuid.UUID, before time.Time, limit int) ([]*domainchat.ChannelMessage, error) {
	limit = u.clampLimit(limit)
	var messages []*domainchat.ChannelMessage
	err := u.ro.RO(ctx, func(ctx context.Context) error {
		member, err := u.members.IsMember(ctx, channelID, me)
		if err != nil {
			return err
		}
		if !member {
			return domainchat.ErrNotChannelMember
		}
		messages, err = u.repo.ListChannel(ctx, channelID, before, limit)
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
