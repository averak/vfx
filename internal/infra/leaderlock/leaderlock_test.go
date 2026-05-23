package leaderlock_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	valkeygo "github.com/valkey-io/valkey-go"

	"github.com/averak/vfx/internal/infra/leaderlock"
	"github.com/averak/vfx/internal/infra/valkey"
)

func dialClient(t *testing.T) valkeygo.Client {
	t.Helper()
	url := os.Getenv("VALKEY_URL")
	if url == "" {
		t.Skip("VALKEY_URL not set; skipping leader-lock test")
	}
	c, err := valkey.NewClient(url)
	if err != nil {
		t.Fatalf("connect valkey: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// waitFor polls cond until it holds or the timeout elapses, so a test
// reacts to a leadership change as soon as it happens rather than sleeping
// a fixed window and hoping the transition already occurred.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

// TestRun_OnlyOneLeaderRunsFn starts two contenders for the same key and
// asserts fn is only ever active in one of them at a time.
func TestRun_OnlyOneLeaderRunsFn(t *testing.T) {
	c1 := dialClient(t)
	c2 := dialClient(t)

	key := "vfx:test:leader:" + uuid.NewString()
	cfg := leaderlock.Config{Key: key, TTL: time.Second, Logger: discardLogger()}

	var active, maxActive atomic.Int32
	fn := func(ctx context.Context) error {
		n := active.Add(1)
		for {
			if n > maxActive.Load() {
				maxActive.Store(n)
			}
			select {
			case <-ctx.Done():
				active.Add(-1)
				return ctx.Err()
			case <-time.After(20 * time.Millisecond):
				n = active.Load()
			}
		}
	}

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{}, 2)
	go func() { _ = leaderlock.Run(ctx, c1, cfg, fn); done <- struct{}{} }()
	go func() { _ = leaderlock.Run(ctx, c2, cfg, fn); done <- struct{}{} }()

	// Wait until one contender holds leadership, then sample a window:
	// mutual exclusion means the concurrent count never exceeds 1.
	waitFor(t, 2*time.Second, func() bool { return active.Load() >= 1 })
	time.Sleep(500 * time.Millisecond)
	if got := maxActive.Load(); got != 1 {
		t.Errorf("max concurrent leaders = %d, want 1", got)
	}

	cancel()
	<-done
	<-done
}

// TestRun_FailoverToSecond checks a second contender takes over after
// the first stops holding the lease.
func TestRun_FailoverToSecond(t *testing.T) {
	c1 := dialClient(t)
	c2 := dialClient(t)

	key := "vfx:test:leader:" + uuid.NewString()
	cfg := leaderlock.Config{Key: key, TTL: time.Second, Logger: discardLogger()}

	var firstRan, secondRan atomic.Bool
	first := func(ctx context.Context) error {
		firstRan.Store(true)
		<-ctx.Done()
		return ctx.Err()
	}
	second := func(ctx context.Context) error {
		secondRan.Store(true)
		<-ctx.Done()
		return ctx.Err()
	}

	ctx1, cancel1 := context.WithCancel(t.Context())
	ctx2, cancel2 := context.WithCancel(t.Context())
	defer cancel2()
	done1 := make(chan struct{})
	go func() { _ = leaderlock.Run(ctx1, c1, cfg, first); close(done1) }()

	// c1 becomes leader; c2 then stands by (its standby is the mutual
	// exclusion that TestRun_OnlyOneLeaderRunsFn already verifies).
	waitFor(t, 2*time.Second, firstRan.Load)
	go func() { _ = leaderlock.Run(ctx2, c2, cfg, second) }()

	// Stop the first; the second should acquire within ~1 lease TTL.
	cancel1()
	<-done1
	waitFor(t, 3*time.Second, secondRan.Load)
}
