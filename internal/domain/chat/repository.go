package chat

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	Save(ctx context.Context, m *Message) error

	// Newest first; before is an exclusive upper bound and its zero value means "from the latest".
	ListConversation(ctx context.Context, a, b uuid.UUID, before time.Time, limit int) ([]*Message, error)

	SaveChannelMessage(ctx context.Context, m *ChannelMessage) error

	// Newest first; before is an exclusive upper bound and its zero value means "from the latest".
	ListChannel(ctx context.Context, channelID uuid.UUID, before time.Time, limit int) ([]*ChannelMessage, error)
}
