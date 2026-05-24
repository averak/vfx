// Package chat is the direct-message aggregate.
//
// A message belongs to the conversation between two players, identified by the canonical (low, high) ordering of their ids so each pair has a single conversation.
package chat

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// MaxBodyLength bounds a message body in runes.
const MaxBodyLength = 2000

var (
	ErrSelfMessage      = errors.New("chat: cannot message yourself")
	ErrInvalidBody      = errors.New("chat: body is blank or too long")
	ErrNotChannelMember = errors.New("chat: not a member of the channel")
)

type Message struct {
	ID          uuid.UUID
	SenderID    uuid.UUID
	RecipientID uuid.UUID
	Body        string
	SentAt      time.Time
}

func NewMessage(id, sender, recipient uuid.UUID, body string, now time.Time) (*Message, error) {
	if sender == recipient {
		return nil, ErrSelfMessage
	}
	if strings.TrimSpace(body) == "" || utf8.RuneCountInString(body) > MaxBodyLength {
		return nil, ErrInvalidBody
	}
	return &Message{
		ID:          id,
		SenderID:    sender,
		RecipientID: recipient,
		Body:        body,
		SentAt:      now,
	}, nil
}

// ChannelMessage's ChannelID is the group id the channel maps to.
type ChannelMessage struct {
	ID        uuid.UUID
	ChannelID uuid.UUID
	SenderID  uuid.UUID
	Body      string
	SentAt    time.Time
}

func NewChannelMessage(id, channelID, sender uuid.UUID, body string, now time.Time) (*ChannelMessage, error) {
	if strings.TrimSpace(body) == "" || utf8.RuneCountInString(body) > MaxBodyLength {
		return nil, ErrInvalidBody
	}
	return &ChannelMessage{ID: id, ChannelID: channelID, SenderID: sender, Body: body, SentAt: now}, nil
}

func Conversation(a, b uuid.UUID) (low, high uuid.UUID) {
	for i := range a {
		switch {
		case a[i] < b[i]:
			return a, b
		case a[i] > b[i]:
			return b, a
		}
	}
	return a, b
}
