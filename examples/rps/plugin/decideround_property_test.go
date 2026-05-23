package plugin

import (
	"testing"

	"pgregory.net/rapid"
)

// decideRound is the canonical RPS rule, so it must satisfy a handful of
// algebraic properties for every pair of choices, not just the examples
// in game_test.go. These hold for all of R/P/S.
func TestDecideRound_Properties(t *testing.T) {
	choices := rapid.SampledFrom([]byte{'R', 'P', 'S'})
	const idA, idB = "A", "B"

	rapid.Check(t, func(t *rapid.T) {
		a := choices.Draw(t, "a")
		b := choices.Draw(t, "b")
		w := decideRound(a, b, idA, idB)

		// Totality: the outcome is always a tie or one of the two players.
		if w != "" && w != idA && w != idB {
			t.Fatalf("unknown winner %q for a=%c b=%c", w, a, b)
		}
		// A tie happens exactly when the choices are equal.
		if (a == b) != (w == "") {
			t.Fatalf("tie/equality mismatch: a=%c b=%c winner=%q", a, b, w)
		}
		// The winner's position does not depend on the id labels.
		relabelled := decideRound(a, b, "X", "Y")
		if positionOf(relabelled, "X", "Y") != positionOf(w, idA, idB) {
			t.Fatalf("winner position changed when ids were relabelled")
		}
		// For distinct choices, swapping them swaps the winner.
		if a != b {
			swapped := decideRound(b, a, idA, idB)
			if swapped == "" {
				t.Fatalf("swapping distinct choices produced a tie")
			}
			if swapped == w {
				t.Fatalf("swapping choices did not swap the winner (%q)", w)
			}
		}
	})
}

func positionOf(winner, idA, idB string) int {
	switch winner {
	case idA:
		return 0
	case idB:
		return 1
	default:
		return -1
	}
}
