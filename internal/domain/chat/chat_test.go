package chat_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/chat"
)

func TestNewMessage_Validation(t *testing.T) {
	a := uuid.New()
	b := uuid.New()
	now := time.Now()

	if _, err := chat.NewMessage(uuid.New(), a, a, "hi", now); !errors.Is(err, chat.ErrSelfMessage) {
		t.Errorf("self message: err = %v, want ErrSelfMessage", err)
	}
	if _, err := chat.NewMessage(uuid.New(), a, b, "   ", now); !errors.Is(err, chat.ErrInvalidBody) {
		t.Errorf("blank body: err = %v, want ErrInvalidBody", err)
	}
	if _, err := chat.NewMessage(uuid.New(), a, b, strings.Repeat("x", chat.MaxBodyLength+1), now); !errors.Is(err, chat.ErrInvalidBody) {
		t.Errorf("over-length body: err = %v, want ErrInvalidBody", err)
	}
	if _, err := chat.NewMessage(uuid.New(), a, b, "hello", now); err != nil {
		t.Errorf("valid message rejected: %v", err)
	}
}

func TestNewChannelMessage_Validation(t *testing.T) {
	channelID := uuid.New()
	sender := uuid.New()
	now := time.Now()

	if _, err := chat.NewChannelMessage(uuid.New(), channelID, sender, "   ", now); !errors.Is(err, chat.ErrInvalidBody) {
		t.Errorf("blank body: err = %v, want ErrInvalidBody", err)
	}
	if _, err := chat.NewChannelMessage(uuid.New(), channelID, sender, strings.Repeat("x", chat.MaxBodyLength+1), now); !errors.Is(err, chat.ErrInvalidBody) {
		t.Errorf("over-length body: err = %v, want ErrInvalidBody", err)
	}
	if _, err := chat.NewChannelMessage(uuid.New(), channelID, sender, "hello", now); err != nil {
		t.Errorf("valid channel message rejected: %v", err)
	}
}

// Conversation is canonical: either argument order yields the same (low, high) pair.
func TestConversation_Canonical(t *testing.T) {
	a := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	b := uuid.MustParse("00000000-0000-0000-0000-0000000000ff")
	l1, h1 := chat.Conversation(a, b)
	l2, h2 := chat.Conversation(b, a)
	if l1 != l2 || h1 != h2 || l1 != a || h1 != b {
		t.Errorf("Conversation not canonical: (%s,%s) vs (%s,%s)", l1, h1, l2, h2)
	}
}
