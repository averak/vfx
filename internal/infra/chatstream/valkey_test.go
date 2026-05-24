package chatstream_test

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/infra/chatstream"
	"github.com/averak/vfx/internal/infra/valkey"
)

// This test exercises the Valkey broker against a real Valkey (the compose stack provides it); it skips when VALKEY_URL is unset.
func TestValkey_StreamsLiveMessages(t *testing.T) {
	url := os.Getenv("VALKEY_URL")
	if url == "" {
		t.Skip("VALKEY_URL not set; skipping Valkey chatstream test")
	}
	client, err := valkey.NewClient(url)
	if err != nil {
		t.Fatalf("valkey client: %v", err)
	}
	t.Cleanup(client.Close)

	b := chatstream.NewValkey(client)
	channelID := uuid.New()

	sub, err := b.Subscribe(t.Context(), channelID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// "$" delivers only messages added after the subscriber's XREAD reaches the server, so resend until one lands.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				_ = b.Publish(t.Context(), msg(channelID, "live"))
			}
		}
	}()

	select {
	case got := <-sub:
		if got == nil || got.Body != "live" || got.ChannelID != channelID {
			t.Errorf("got = %+v, want body=live channel=%s", got, channelID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no message received from Valkey stream")
	}
}
