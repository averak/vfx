package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/chat"
	"github.com/averak/vfx/internal/infra/db"
	"github.com/averak/vfx/internal/infra/dbgen"
)

// farFuture stands in for "no before cursor", so the first history page is simply "everything older than the end of time".
var farFuture = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)

// Chat is the storage implementation of [chat.Repository].
type Chat struct{}

var _ chat.Repository = (*Chat)(nil)

func NewChat() *Chat {
	return &Chat{}
}

func (Chat) Save(ctx context.Context, m *chat.Message) error {
	tx, err := db.Tx(ctx)
	if err != nil {
		return err
	}
	low, high := chat.Conversation(m.SenderID, m.RecipientID)
	return dbgen.New(tx).InsertDirectMessage(ctx, dbgen.InsertDirectMessageParams{
		ID:         m.ID,
		PlayerLow:  low,
		PlayerHigh: high,
		SenderID:   m.SenderID,
		Body:       m.Body,
		CreatedAt:  toTimestamptz(m.SentAt),
	})
}

func (Chat) ListConversation(ctx context.Context, a, b uuid.UUID, before time.Time, limit int) ([]*chat.Message, error) {
	tx, err := db.Tx(ctx)
	if err != nil {
		return nil, err
	}
	if before.IsZero() {
		before = farFuture
	}
	low, high := chat.Conversation(a, b)
	rows, err := dbgen.New(tx).ListConversation(ctx, dbgen.ListConversationParams{
		PlayerLow:  low,
		PlayerHigh: high,
		CreatedAt:  toTimestamptz(before),
		//nolint:gosec // limit is a small, server-clamped page size.
		Limit: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]*chat.Message, len(rows))
	for i, row := range rows {
		// The recipient is whichever participant is not the sender.
		recipient := row.PlayerHigh
		if row.SenderID == row.PlayerHigh {
			recipient = row.PlayerLow
		}
		out[i] = &chat.Message{
			ID:          row.ID,
			SenderID:    row.SenderID,
			RecipientID: recipient,
			Body:        row.Body,
			SentAt:      row.CreatedAt.Time,
		}
	}
	return out, nil
}
