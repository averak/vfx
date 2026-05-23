package agones

import (
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type fakeSDK struct {
	mu       sync.Mutex
	ready    int
	health   int
	shutdown int
	readyErr error
}

func (f *fakeSDK) Ready() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ready++
	return f.readyErr
}

func (f *fakeSDK) Health() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.health++
	return nil
}

func (f *fakeSDK) Shutdown() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shutdown++
	return nil
}

func (f *fakeSDK) counts() (ready, health, shutdown int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ready, f.health, f.shutdown
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// waitFor polls cond until it holds or the timeout elapses, so a test
// reacts to an event as soon as it happens instead of sleeping a fixed,
// guess-the-worst-case window.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

func TestStart_ReadyHealthShutdown(t *testing.T) {
	f := &fakeSDK{}
	stop, err := start(t.Context(), f, 5*time.Millisecond, discardLogger())
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	// Wait until the health loop has pinged at least once before stopping.
	waitFor(t, time.Second, func() bool {
		_, health, _ := f.counts()
		return health > 0
	})
	stop()

	ready, health, shutdown := f.counts()
	if ready != 1 {
		t.Errorf("Ready called %d times, want 1", ready)
	}
	if health == 0 {
		t.Error("Health was never pinged")
	}
	if shutdown != 1 {
		t.Errorf("Shutdown called %d times, want 1", shutdown)
	}
}

func TestStart_ReadyFailureAborts(t *testing.T) {
	f := &fakeSDK{readyErr: errors.New("sidecar down")}
	if _, err := start(t.Context(), f, 5*time.Millisecond, discardLogger()); err == nil {
		t.Fatal("start succeeded despite a Ready failure, want an error")
	}
	if _, health, shutdown := f.counts(); health != 0 || shutdown != 0 {
		t.Errorf("after a failed Ready: health=%d shutdown=%d, want 0/0", health, shutdown)
	}
}
