package leaderboard_test

import (
	"testing"

	"github.com/averak/vfx/internal/domain/leaderboard"
)

// Beats is strict: an equal score never beats the incumbent, so keep-best does not churn updated_at on a re-submit of the same value.
func TestLeaderboard_Beats(t *testing.T) {
	desc := leaderboard.Leaderboard{SortOrder: leaderboard.Descending}
	asc := leaderboard.Leaderboard{SortOrder: leaderboard.Ascending}

	tests := []struct {
		name string
		lb   leaderboard.Leaderboard
		a, b int64
		want bool
	}{
		{"descending higher beats", desc, 10, 5, true},
		{"descending lower does not", desc, 5, 10, false},
		{"descending equal does not", desc, 5, 5, false},
		{"ascending lower beats", asc, 5, 10, true},
		{"ascending higher does not", asc, 10, 5, false},
		{"ascending equal does not", asc, 5, 5, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.lb.Beats(tt.a, tt.b); got != tt.want {
				t.Errorf("Beats(%d, %d) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
