package chatstream

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/chat"
	usecasechat "github.com/averak/vfx/internal/usecase/chat"
)

// InMem fans messages out within a single process.
// It is correct for a single-gateway deployment and for tests; the Valkey-backed broker is used when subscribers can land on a different replica than the sender.
type InMem struct {
	mu          sync.Mutex
	subscribers map[uuid.UUID][]chan *chat.ChannelMessage
}

var _ usecasechat.Broker = (*InMem)(nil)

func NewInMem() *InMem {
	return &InMem{subscribers: make(map[uuid.UUID][]chan *chat.ChannelMessage)}
}

func (b *InMem) Publish(_ context.Context, m *chat.ChannelMessage) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subscribers[m.ChannelID] {
		// Drop rather than block: a slow subscriber must not stall the sender, and the message is already persisted for history.
		select {
		case ch <- m:
		default:
		}
	}
	return nil
}

func (b *InMem) Subscribe(ctx context.Context, channelID uuid.UUID) (<-chan *chat.ChannelMessage, error) {
	ch := make(chan *chat.ChannelMessage, 16)
	b.mu.Lock()
	b.subscribers[channelID] = append(b.subscribers[channelID], ch)
	b.mu.Unlock()

	go func() {
		<-ctx.Done()
		b.detach(channelID, ch)
	}()
	return ch, nil
}

func (b *InMem) detach(channelID uuid.UUID, target chan *chat.ChannelMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	subs := b.subscribers[channelID]
	for i, ch := range subs {
		if ch == target {
			b.subscribers[channelID] = append(subs[:i], subs[i+1:]...)
			close(ch)
			break
		}
	}
	if len(b.subscribers[channelID]) == 0 {
		delete(b.subscribers, channelID)
	}
}
