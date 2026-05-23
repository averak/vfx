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
}
