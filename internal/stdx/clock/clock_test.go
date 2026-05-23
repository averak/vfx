package clock_test

import (
	"testing"
	"time"

	"github.com/averak/vfx/internal/stdx/clock"
)

func TestNow_ReturnsAttachedTime(t *testing.T) {
	fixed := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	ctx := clock.With(t.Context(), fixed)

	got := clock.Now(ctx)
	if !got.Equal(fixed) {
		t.Errorf("Now() = %v, want %v", got, fixed)
	}
}

func TestNow_FallsBackToRealTimeWhenUnset(t *testing.T) {
	before := time.Now()
	got := clock.Now(t.Context())
	after := time.Now()

	if got.Before(before) || got.After(after) {
		t.Errorf("Now() = %v, expected between %v and %v", got, before, after)
	}
}

func TestWith_DoesNotMutateParent(t *testing.T) {
	parent := t.Context()
	fixed := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	_ = clock.With(parent, fixed)

	// Calling Now on the unchanged parent must still hit the fallback,
	// confirming With produced a new context rather than mutating the
	// shared base.
	before := time.Now()
	got := clock.Now(parent)
	after := time.Now()
	if got.Before(before) || got.After(after) {
		t.Errorf("Now(parent) = %v, expected fallback to real time", got)
	}
}

func TestWith_NestedOverridesOuter(t *testing.T) {
	outer := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	inner := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	ctx := clock.With(t.Context(), outer)
	ctx = clock.With(ctx, inner)

	if got := clock.Now(ctx); !got.Equal(inner) {
		t.Errorf("Now() = %v, want innermost %v", got, inner)
	}
}
