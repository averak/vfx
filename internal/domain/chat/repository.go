package chat

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository persists direct messages.
type Repository interface {
	// Save stores a message; the conversation key is derived from its participants.
	Save(ctx context.Context, m *Message) error

	// ListConversation returns messages between the two players newest-first, older than before (zero time means latest), capped at limit.
	ListConversation(ctx context.Context, a, b uuid.UUID, before time.Time, limit int) ([]*Message, error)

	// SaveChannelMessage stores a channel (group) message.
	SaveChannelMessage(ctx context.Context, m *ChannelMessage) error

	// ListChannel returns a channel's messages newest-first, older than before (zero means latest), capped at limit.
	ListChannel(ctx context.Context, channelID uuid.UUID, before time.Time, limit int) ([]*ChannelMessage, error)
}
