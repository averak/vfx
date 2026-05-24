// Package chatstream fans channel messages out to live subscribers.
//
// Valkey is the cross-replica backend: each channel has a Valkey stream that SendChannelMessage appends to and SubscribeChannel tails.
// InMem is the single-process backend used in tests.
package chatstream

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	valkeygo "github.com/valkey-io/valkey-go"

	"github.com/averak/vfx/internal/domain/chat"
	usecasechat "github.com/averak/vfx/internal/usecase/chat"
)

// streamMaxLen bounds each channel stream so a busy channel cannot grow without limit; subscribers only tail new messages, so old entries are never re-read and trimming them is free.
const streamMaxLen = "1000"

// Valkey publishes to and tails per-channel Valkey streams.
type Valkey struct {
	client valkeygo.Client
	ttl    time.Duration
}

var _ usecasechat.Broker = (*Valkey)(nil)

// NewValkey wraps a connected client.
// ttl reclaims a channel's stream once it has been idle that long; a live subscriber re-creates it on the next publish, so the bound only affects dormant channels.
func NewValkey(client valkeygo.Client) *Valkey {
	return &Valkey{client: client, ttl: 10 * time.Minute}
}

func streamKey(channelID uuid.UUID) string { return "vfx:chat:stream:" + channelID.String() }

type messageDTO struct {
	ID        string    `json:"id"`
	ChannelID string    `json:"channel_id"`
	SenderID  string    `json:"sender_id"`
	Body      string    `json:"body"`
	SentAt    time.Time `json:"sent_at"`
}

func marshalMessage(m *chat.ChannelMessage) (string, error) {
	data, err := json.Marshal(messageDTO{
		ID:        m.ID.String(),
		ChannelID: m.ChannelID.String(),
		SenderID:  m.SenderID.String(),
		Body:      m.Body,
		SentAt:    m.SentAt,
	})
	return string(data), err
}

func unmarshalMessage(raw string) (*chat.ChannelMessage, error) {
	var dto messageDTO
	if err := json.Unmarshal([]byte(raw), &dto); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(dto.ID)
	if err != nil {
		return nil, err
	}
	channelID, err := uuid.Parse(dto.ChannelID)
	if err != nil {
		return nil, err
	}
	senderID, err := uuid.Parse(dto.SenderID)
	if err != nil {
		return nil, err
	}
	return &chat.ChannelMessage{
		ID:        id,
		ChannelID: channelID,
		SenderID:  senderID,
		Body:      dto.Body,
		SentAt:    dto.SentAt,
	}, nil
}

func (v *Valkey) Publish(ctx context.Context, m *chat.ChannelMessage) error {
	data, err := marshalMessage(m)
	if err != nil {
		return fmt.Errorf("chatstream: marshal message: %w", err)
	}
	key := streamKey(m.ChannelID)
	if err := v.client.Do(ctx,
		v.client.B().Xadd().Key(key).Maxlen().Almost().Threshold(streamMaxLen).Id("*").FieldValue().FieldValue("msg", data).Build()).Error(); err != nil {
		return fmt.Errorf("chatstream: xadd: %w", err)
	}
	if err := v.client.Do(ctx, v.client.B().Expire().Key(key).Seconds(int64(v.ttl.Seconds())).Build()).Error(); err != nil {
		return fmt.Errorf("chatstream: stream expire: %w", err)
	}
	return nil
}

func (v *Valkey) Subscribe(ctx context.Context, channelID uuid.UUID) (<-chan *chat.ChannelMessage, error) {
	ch := make(chan *chat.ChannelMessage, 16)
	subCtx, cancel := context.WithCancel(ctx)
	go func() {
		defer close(ch)
		defer cancel()
		key := streamKey(channelID)
		// Start at "$" so only messages published after this subscription attaches are delivered; backlog is served by ListChannelMessages.
		// Once a real entry arrives we track its id; until then we re-issue "$" on each BLOCK timeout, which keeps the read pinned to "new only".
		lastID := "$"
		for {
			if subCtx.Err() != nil {
				return
			}
			streams, err := v.client.Do(subCtx,
				v.client.B().Xread().Count(64).Block(1000).Streams().Key(key).Id(lastID).Build()).AsXRead()
			if err != nil {
				if valkeygo.IsValkeyNil(err) {
					continue // BLOCK timed out with no new messages
				}
				return // ctx cancelled or a real error
			}
			for _, entry := range streams[key] {
				lastID = entry.ID
				m, decErr := unmarshalMessage(entry.FieldValues["msg"])
				if decErr != nil {
					continue
				}
				select {
				case ch <- m:
				case <-subCtx.Done():
					return
				}
			}
		}
	}()
	return ch, nil
}
