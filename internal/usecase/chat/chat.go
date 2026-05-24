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

// Broker fans channel messages out to live subscribers across gateway replicas.
// Delivery is best-effort: the persisted history is the durable record, and Subscribe streams only messages published after the subscription attaches.
type Broker interface {
	Publish(ctx context.Context, m *domainchat.ChannelMessage) error
	Subscribe(ctx context.Context, channelID uuid.UUID) (<-chan *domainchat.ChannelMessage, error)
}

type Usecase struct {
	rw      tx.ReadWriter
	ro      tx.Reader
	repo    domainchat.Repository
	members Membership
	broker  Broker
	cfg     Config
}

func New(rw tx.ReadWriter, ro tx.Reader, repo domainchat.Repository, members Membership, broker Broker, cfg Config) *Usecase {
	return &Usecase{rw: rw, ro: ro, repo: repo, members: members, broker: broker, cfg: cfg}
}

// SendDirectMessage returns domainchat.ErrSelfMessage or ErrInvalidBody for bad input.
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

// ListDirectMessages returns the conversation newest first; the zero before means "from latest".
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

// SendChannelMessage returns domainchat.ErrNotChannelMember when the sender does not belong to the channel.
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
	// Fan out only after the message is committed, so subscribers never see a write that later rolled back.
	// The push is best-effort: the message is already durable and served by ListChannelMessages, so a broker hiccup must not fail an otherwise successful send (which would push the client into a retry that double-posts).
	_ = u.broker.Publish(ctx, msg) //nolint:errcheck // Best-effort realtime fan-out; history is the durable record.
	return msg, nil
}

// SubscribeChannel streams messages posted to the channel after the subscription attaches; the caller must be a member.
// Clients pair this with ListChannelMessages for backlog and de-duplicate the small overlap by message id.
func (u *Usecase) SubscribeChannel(ctx context.Context, me, channelID uuid.UUID) (<-chan *domainchat.ChannelMessage, error) {
	var member bool
	if err := u.ro.RO(ctx, func(ctx context.Context) error {
		var err error
		member, err = u.members.IsMember(ctx, channelID, me)
		return err
	}); err != nil {
		return nil, err
	}
	if !member {
		return nil, domainchat.ErrNotChannelMember
	}
	// Subscribe on the request context, not the transaction-scoped one: the stream outlives the membership check's read transaction.
	return u.broker.Subscribe(ctx, channelID)
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
