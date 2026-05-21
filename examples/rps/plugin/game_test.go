package plugin_test

import (
	"testing"

	rps "github.com/averak/vfx/examples/rps/plugin"
	pluginv1 "github.com/averak/vfx/gen/go/plugin/v1"
)

const (
	alice = "alice"
	bob   = "bob"
)

func newInitedGame(t *testing.T) *rps.Game {
	t.Helper()
	g := rps.NewGame()
	if _, err := g.Init(&pluginv1.InitRequest{PlayerIds: []string{alice, bob}}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return g
}

// playRound submits both choices and returns the tick response from the
// tick that resolved the round (the second submission).
func playRound(t *testing.T, g *rps.Game, aChoice, bChoice byte) *pluginv1.OnTickResponse {
	t.Helper()
	if _, err := g.OnTick(&pluginv1.OnTickRequest{
		Actions: []*pluginv1.PlayerAction{{PlayerId: alice, Payload: []byte{aChoice}}},
	}); err != nil {
		t.Fatalf("OnTick(alice): %v", err)
	}
	resp, err := g.OnTick(&pluginv1.OnTickRequest{
		Actions: []*pluginv1.PlayerAction{{PlayerId: bob, Payload: []byte{bChoice}}},
	})
	if err != nil {
		t.Fatalf("OnTick(bob): %v", err)
	}
	return resp
}

func TestInit_RequiresTwoPlayers(t *testing.T) {
	cases := map[string][]string{
		"zero":  {},
		"one":   {alice},
		"three": {alice, bob, "carol"},
	}
	for name, players := range cases {
		t.Run(name, func(t *testing.T) {
			g := rps.NewGame()
			if _, err := g.Init(&pluginv1.InitRequest{PlayerIds: players}); err == nil {
				t.Errorf("Init(%d players) = nil error, want error", len(players))
			}
		})
	}
}

func TestGame_AliceSweeps(t *testing.T) {
	g := newInitedGame(t)

	// Round 1: Rock beats Scissors → alice.
	if resp := playRound(t, g, 'R', 'S'); resp.GetGameEnded() {
		t.Fatal("game ended after one round")
	}
	// Round 2: Paper beats Rock → alice reaches 2 wins, match ends.
	resp := playRound(t, g, 'P', 'R')
	if !resp.GetGameEnded() {
		t.Fatal("game did not end after alice's second win")
	}

	end, err := g.OnGameEnd(&pluginv1.OnGameEndRequest{})
	if err != nil {
		t.Fatalf("OnGameEnd: %v", err)
	}
	assertRank(t, end, alice, 1)
	assertRank(t, end, bob, -1)
}

func TestGame_DrawAfterThreeRounds(t *testing.T) {
	g := newInitedGame(t)

	playRound(t, g, 'R', 'S')         // alice
	playRound(t, g, 'S', 'R')         // bob
	resp := playRound(t, g, 'R', 'R') // tie → 3 rounds played, 1-1

	if !resp.GetGameEnded() {
		t.Fatal("game did not end after three rounds")
	}
	end, err := g.OnGameEnd(&pluginv1.OnGameEndRequest{})
	if err != nil {
		t.Fatalf("OnGameEnd: %v", err)
	}
	// A draw ranks both at 0.
	assertRank(t, end, alice, 0)
	assertRank(t, end, bob, 0)
}

func TestGame_TieRoundDoesNotProgressScore(t *testing.T) {
	g := newInitedGame(t)
	resp := playRound(t, g, 'R', 'R')
	if resp.GetGameEnded() {
		t.Fatal("a single tie should not end the match")
	}
	if len(resp.GetStateDelta()) == 0 {
		t.Fatal("a resolved (tied) round should still emit a state delta")
	}
}

func TestGame_SecondSubmissionIgnoredUntilRoundResolves(t *testing.T) {
	g := newInitedGame(t)

	// Alice submits twice before bob; the second must be ignored, so no
	// round resolves and no delta is produced.
	_, _ = g.OnTick(&pluginv1.OnTickRequest{
		Actions: []*pluginv1.PlayerAction{{PlayerId: alice, Payload: []byte{'R'}}},
	})
	resp, err := g.OnTick(&pluginv1.OnTickRequest{
		Actions: []*pluginv1.PlayerAction{{PlayerId: alice, Payload: []byte{'P'}}},
	})
	if err != nil {
		t.Fatalf("OnTick: %v", err)
	}
	if len(resp.GetStateDelta()) != 0 {
		t.Fatal("round resolved on a duplicate submission from the same player")
	}
}

func assertRank(t *testing.T, end *pluginv1.OnGameEndResponse, playerID string, want int32) {
	t.Helper()
	for _, r := range end.GetPlayerResults() {
		if r.GetPlayerId() == playerID {
			if r.GetRank() != want {
				t.Errorf("rank of %s = %d, want %d", playerID, r.GetRank(), want)
			}
			return
		}
	}
	t.Errorf("no result for player %s", playerID)
}
