package chatstream_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/chat"
	"github.com/averak/vfx/internal/infra/chatstream"
)

func msg(channelID uuid.UUID, body string) *chat.ChannelMessage {
	return &chat.ChannelMessage{
		ID:        uuid.New(),
		ChannelID: channelID,
		SenderID:  uuid.New(),
		Body:      body,
		SentAt:    time.Now().UTC(),
	}
}

func TestInMem_DeliversToChannelSubscribers(t *testing.T) {
	b := chatstream.NewInMem()
	chA := uuid.New()
	chB := uuid.New()

	subA, err := b.Subscribe(t.Context(), chA)
	if err != nil {
		t.Fatalf("Subscribe A: %v", err)
	}
	subB, err := b.Subscribe(t.Context(), chB)
	if err != nil {
		t.Fatalf("Subscribe B: %v", err)
	}

	if err := b.Publish(t.Context(), msg(chA, "hello-a")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case got := <-subA:
		if got.Body != "hello-a" {
			t.Errorf("subA body = %q, want hello-a", got.Body)
		}
	case <-time.After(time.Second):
		t.Fatal("subA received nothing")
	}

	// A message for channel A must not reach a channel B subscriber.
	select {
	case got := <-subB:
		t.Errorf("subB received cross-channel message: %+v", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestInMem_DetachesOnContextCancel(t *testing.T) {
	b := chatstream.NewInMem()
	channelID := uuid.New()
	ctx, cancel := context.WithCancel(t.Context())

	sub, err := b.Subscribe(ctx, channelID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	cancel()

	// Detach closes the channel; the subscriber observes it as a closed receive.
	select {
	case _, ok := <-sub:
		if ok {
			t.Error("expected closed channel after cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed after cancel")
	}

	// A publish after detach must not panic on the closed channel.
	if err := b.Publish(t.Context(), msg(channelID, "after-cancel")); err != nil {
		t.Fatalf("Publish after cancel: %v", err)
	}
}

// Publish must not block on a subscriber that never drains; the message is dropped for that subscriber.
func TestInMem_DropsForSlowSubscriber(t *testing.T) {
	b := chatstream.NewInMem()
	channelID := uuid.New()
	if _, err := b.Subscribe(t.Context(), channelID); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		// The subscriber's buffer is 16; publishing well past that must still return promptly.
		for i := 0; i < 1000; i++ {
			_ = b.Publish(t.Context(), msg(channelID, "flood"))
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a slow subscriber")
	}
}
