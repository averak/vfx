// Package leaderlock provides a best-effort single-leader election over Valkey, so only one gateway replica runs the matchmaker loop at a time instead of all of them scanning the shared queue.
//
// The election is intentionally loose: a brief overlap between an old leader losing its lease and a new one acquiring it is acceptable, because matchmaking correctness does not depend on exclusivity (the queue's atomic Claim already prevents two matchmakers from pairing the same ticket).
// The lock is purely an optimisation to avoid redundant work, so it needs no fencing tokens or Redlock-style quorum.
package leaderlock

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	valkeygo "github.com/valkey-io/valkey-go"
)

type Config struct {
	Key    string
	TTL    time.Duration
	Logger *slog.Logger
}

// renewScript extends the lease only if we still own it.
const renewScript = `if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('PEXPIRE', KEYS[1], ARGV[2]) else return 0 end`

// releaseScript deletes the lease only if we still own it, so we never delete a lease a successor already took.
const releaseScript = `if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) else return 0 end`

// Run contends for leadership and runs fn while this instance holds it.
// fn receives a context cancelled when leadership is lost (lease expired or stolen) or when the outer ctx is done.
// Run returns only when ctx is done; it keeps re-contending after losing leadership.
func Run(ctx context.Context, client valkeygo.Client, cfg Config, fn func(context.Context) error) error {
	id := uuid.NewString()
	if cfg.TTL <= 0 {
		cfg.TTL = 15 * time.Second
	}
	renewEvery := cfg.TTL / 3
	pollEvery := cfg.TTL / 2

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		acquired, err := acquire(ctx, client, cfg.Key, id, cfg.TTL)
		if err != nil || !acquired {
			if !sleep(ctx, pollEvery) {
				return ctx.Err()
			}
			continue
		}

		cfg.Logger.Info("matchmaker leadership acquired", "instance", id)
		if lerr := leadAndHold(ctx, client, cfg, id, renewEvery, fn); lerr != nil && ctx.Err() == nil {
			cfg.Logger.Warn("matchmaker leadership lost", "instance", id, "err", lerr)
		}
		// Best-effort release so a successor can take over promptly.
		release(client, cfg.Key, id)
	}
}

// leadAndHold runs fn under a leadership context and renews the lease until ctx ends, the lease can no longer be renewed, or fn returns.
func leadAndHold(ctx context.Context, client valkeygo.Client, cfg Config, id string, renewEvery time.Duration, fn func(context.Context) error) error {
	leaderCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	fnDone := make(chan error, 1)
	go func() { fnDone <- fn(leaderCtx) }()

	ticker := time.NewTicker(renewEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			cancel()
			<-fnDone
			return nil
		case err := <-fnDone:
			// fn exited on its own; stop being leader.
			return err
		case <-ticker.C:
			ok, err := renew(ctx, client, cfg.Key, id, cfg.TTL)
			if err != nil || !ok {
				cancel()
				<-fnDone
				if err != nil {
					return err
				}
				return fmt.Errorf("leaderlock: lease no longer held")
			}
		}
	}
}

func acquire(ctx context.Context, client valkeygo.Client, key, id string, ttl time.Duration) (bool, error) {
	res := client.Do(ctx, client.B().Set().Key(key).Value(id).Nx().PxMilliseconds(ttl.Milliseconds()).Build())
	if err := res.Error(); err != nil {
		if valkeygo.IsValkeyNil(err) {
			return false, nil // held by someone else
		}
		return false, fmt.Errorf("leaderlock: acquire: %w", err)
	}
	return true, nil
}

func renew(ctx context.Context, client valkeygo.Client, key, id string, ttl time.Duration) (bool, error) {
	n, err := client.Do(ctx, client.B().Eval().Script(renewScript).Numkeys(1).Key(key).Arg(id, fmt.Sprintf("%d", ttl.Milliseconds())).Build()).ToInt64()
	if err != nil {
		return false, fmt.Errorf("leaderlock: renew: %w", err)
	}
	return n == 1, nil
}

func release(client valkeygo.Client, key, id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Best-effort; a failure just means the lease expires on its own.
	_ = client.Do(ctx, client.B().Eval().Script(releaseScript).Numkeys(1).Key(key).Arg(id).Build()).Error() //nolint:errcheck // release is best-effort; the lease expires on its own.
}

func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
