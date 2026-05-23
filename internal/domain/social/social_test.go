package social_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/social"
)

// OrderPair is canonical: it returns the same (low, high) regardless of argument order, so an undirected friendship has one representation.
func TestOrderPair_Canonical(t *testing.T) {
	a := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	b := uuid.MustParse("00000000-0000-0000-0000-0000000000ff")

	low, high := social.OrderPair(a, b)
	if low != a || high != b {
		t.Errorf("OrderPair(a, b) = (%s, %s), want (a, b)", low, high)
	}
	low, high = social.OrderPair(b, a)
	if low != a || high != b {
		t.Errorf("OrderPair(b, a) = (%s, %s), want (a, b)", low, high)
	}
}
