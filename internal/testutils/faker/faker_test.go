package faker_test

import (
	"testing"

	"github.com/averak/vfx/internal/testutils/faker"
)

func TestUUIDv5_IsDeterministic(t *testing.T) {
	first := faker.UUIDv5("alice")
	second := faker.UUIDv5("alice")

	if first != second {
		t.Errorf("UUIDv5 should be deterministic: %v != %v", first, second)
	}
}

func TestUUIDv5_DifferentNamesProduceDifferentUUIDs(t *testing.T) {
	a := faker.UUIDv5("alice")
	b := faker.UUIDv5("bob")

	if a == b {
		t.Errorf("UUIDv5 collided for different names: %v", a)
	}
}

func TestUUIDv5_StableAcrossRuns(t *testing.T) {
	// Hard-coded expected value so a future refactor that changes the
	// namespace fails loudly rather than silently invalidating every
	// downstream fixture-based test.
	got := faker.UUIDv5("player-1").String()
	const want = "88f90df9-28f7-5652-9722-c1e77a28f85f"
	if got != want {
		t.Errorf("UUIDv5(%q) = %q, want %q (namespace changed?)", "player-1", got, want)
	}
}
