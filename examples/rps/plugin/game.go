// Package plugin implements rock-paper-scissors as a vfx plugin.
//
// The rules live in Game, which is free of any host- or guest-transport concern: it takes and returns the plugin proto messages directly.
// Two thin shells drive the same Game:
//
//   - plugin.go adapts Game to the host-side plugin.Plugin interface so the vfx-rps binary can run it natively (Go).
//   - cmd/wasm registers Game with the guest SDK and is compiled to WebAssembly by TinyGo, then loaded by the room's wazero host.
//
// Because Game has no host function calls and no goroutines, the same source compiles identically for both paths.
package plugin

import (
	"encoding/json"
	"errors"
	"strconv"

	pluginv1 "github.com/averak/vfx/gen/go/plugin/v1"
)

const (
	pluginName = "rps"
	rounds     = 3
	winsToWin  = 2
)

type Game struct {
	state *gameState
}

func NewGame() *Game {
	return &Game{state: newGameState()}
}

// Init validates the two-player roster and returns the initial snapshot.
func (g *Game) Init(req *pluginv1.InitRequest) (*pluginv1.InitResponse, error) {
	if len(req.GetPlayerIds()) != 2 {
		return nil, errors.New("rps: requires exactly two players")
	}
	g.state.playerA = req.GetPlayerIds()[0]
	g.state.playerB = req.GetPlayerIds()[1]

	snapshot, err := g.state.encode()
	if err != nil {
		return nil, err
	}
	return &pluginv1.InitResponse{
		// 0 means event-driven; the room daemon still wakes up every 50ms to drain inputs, which is plenty for a turn-based game.
		TickRateHz:      0,
		InitialSnapshot: snapshot,
	}, nil
}

// OnTick applies queued actions and emits a state delta when a round resolves.
// It sets game_ended once a match winner is decided.
func (g *Game) OnTick(req *pluginv1.OnTickRequest) (*pluginv1.OnTickResponse, error) {
	progressed := false
	for _, action := range req.GetActions() {
		if g.state.matchWinner != "" {
			break
		}
		if g.state.applyAction(action) {
			progressed = true
		}
	}

	resp := &pluginv1.OnTickResponse{}
	if progressed {
		delta, err := g.state.encode()
		if err != nil {
			return nil, err
		}
		resp.StateDelta = delta
	}
	if g.state.matchWinner != "" {
		resp.GameEnded = true
	}
	return resp, nil
}

func (g *Game) OnGameEnd(_ *pluginv1.OnGameEndRequest) (*pluginv1.OnGameEndResponse, error) {
	final, err := g.state.encode()
	if err != nil {
		return nil, err
	}
	return &pluginv1.OnGameEndResponse{
		FinalState: final,
		PlayerResults: []*pluginv1.PlayerResult{
			{
				PlayerId: g.state.playerA,
				Rank:     g.state.rankOf(g.state.playerA),
				Stats:    map[string]string{"wins": strconv.Itoa(g.state.scoreA)},
			},
			{
				PlayerId: g.state.playerB,
				Rank:     g.state.rankOf(g.state.playerB),
				Stats:    map[string]string{"wins": strconv.Itoa(g.state.scoreB)},
			},
		},
	}, nil
}

type gameState struct {
	playerA string
	playerB string

	Round       int             `json:"round"`        // 1-indexed
	Scores      map[string]int  `json:"scores"`       // player_id → rounds won
	Choices     map[string]byte `json:"-"`            // current-round inputs ('R'/'P'/'S')
	History     []roundResult   `json:"history"`      // resolved rounds
	MatchWinner string          `json:"match_winner"` // player_id or ""

	scoreA      int
	scoreB      int
	matchWinner string
}

type roundResult struct {
	Round   int    `json:"round"`
	Winner  string `json:"winner"` // player_id or "" for tie
	ChoiceA string `json:"choice_a"`
	ChoiceB string `json:"choice_b"`
}

func newGameState() *gameState {
	return &gameState{
		Round:   1,
		Scores:  map[string]int{},
		Choices: map[string]byte{},
		History: []roundResult{},
	}
}

func (s *gameState) applyAction(action *pluginv1.PlayerAction) bool {
	payload := action.GetPayload()
	if len(payload) == 0 {
		return false
	}
	choice := normalize(payload[0])
	if choice == 0 {
		return false
	}
	if _, already := s.Choices[action.GetPlayerId()]; already {
		// First choice each round is authoritative; ignore re-submissions until the round resolves.
		return false
	}
	s.Choices[action.GetPlayerId()] = choice

	if len(s.Choices) < 2 {
		return false
	}
	s.resolveRound()
	return true
}

func (s *gameState) resolveRound() {
	a := s.Choices[s.playerA]
	b := s.Choices[s.playerB]

	winner := decideRound(a, b, s.playerA, s.playerB)
	switch winner {
	case s.playerA:
		s.scoreA++
		s.Scores[s.playerA] = s.scoreA
	case s.playerB:
		s.scoreB++
		s.Scores[s.playerB] = s.scoreB
	}
	s.History = append(s.History, roundResult{
		Round: s.Round, Winner: winner,
		ChoiceA: string(rune(a)), ChoiceB: string(rune(b)),
	})

	switch {
	case s.scoreA >= winsToWin:
		s.matchWinner = s.playerA
	case s.scoreB >= winsToWin:
		s.matchWinner = s.playerB
	case s.Round >= rounds:
		switch {
		case s.scoreA > s.scoreB:
			s.matchWinner = s.playerA
		case s.scoreB > s.scoreA:
			s.matchWinner = s.playerB
		default:
			s.matchWinner = "draw"
		}
	}
	s.MatchWinner = s.matchWinner

	s.Round++
	s.Choices = map[string]byte{}
}

func (s *gameState) rankOf(playerID string) int32 {
	if s.matchWinner == "" || s.matchWinner == "draw" {
		return 0
	}
	if s.matchWinner == playerID {
		return 1
	}
	return -1
}

func (s *gameState) encode() ([]byte, error) {
	return json.Marshal(s)
}

// decideRound implements the canonical RPS rules: R beats S, S beats P, P beats R; equal inputs tie.
func decideRound(a, b byte, idA, idB string) string {
	if a == b {
		return ""
	}
	if (a == 'R' && b == 'S') || (a == 'P' && b == 'R') || (a == 'S' && b == 'P') {
		return idA
	}
	return idB
}

// normalize accepts upper- or lower-case R/P/S input.
func normalize(c byte) byte {
	switch c {
	case 'R', 'r':
		return 'R'
	case 'P', 'p':
		return 'P'
	case 'S', 's':
		return 'S'
	}
	return 0
}
