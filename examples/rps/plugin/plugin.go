// Package plugin implements rock-paper-scissors as a vfx plugin.
//
// The plugin satisfies vfx's Plugin/Factory contract directly in Go.
// A future iteration compiles the same logic to WebAssembly via TinyGo
// and loads it through wazero, but the rules below stay unchanged —
// they are pure game logic.
//
// Rules:
//   - Two players.
//   - Best of three rounds.
//   - Each round both players send one byte: 'R', 'P', or 'S'.
//   - As soon as both choices are recorded the round is resolved and
//     a state delta is broadcast.
//   - The match ends when one player reaches two round wins, or when
//     three rounds have been played.
package plugin

import (
	"context"
	"encoding/json"
	"errors"

	pluginv1 "github.com/averak/vfx/gen/go/plugin/v1"
	"github.com/averak/vfx/internal/domain/plugin"
)

const (
	pluginName = "rps"
	rounds     = 3
	winsToWin  = 2
)

// Factory satisfies plugin.Factory for the RPS example.
type Factory struct{}

// NewFactory returns a Factory ready to register with a vfx plugin
// Registry.
func NewFactory() *Factory { return &Factory{} }

// Name is the identifier used by VFX_ROOM_PLUGIN_PATH.
func (*Factory) Name() string { return pluginName }

// Create instantiates a fresh RPS plugin for one match.
func (*Factory) Create(_ context.Context) (plugin.Plugin, error) {
	return &rps{state: newGameState()}, nil
}

// rps is one instance of an RPS game.
type rps struct {
	state *gameState
}

func (r *rps) Init(_ context.Context, req *pluginv1.InitRequest) (*pluginv1.InitResponse, error) {
	if len(req.GetPlayerIds()) != 2 {
		return nil, errors.New("rps: requires exactly two players")
	}
	r.state.playerA = req.GetPlayerIds()[0]
	r.state.playerB = req.GetPlayerIds()[1]

	snapshot, err := r.state.encode()
	if err != nil {
		return nil, err
	}
	return &pluginv1.InitResponse{
		// 0 means event-driven; the room daemon still wakes up every
		// 50ms to drain inputs, which is plenty for a turn-based game.
		TickRateHz:      0,
		InitialSnapshot: snapshot,
	}, nil
}

func (r *rps) OnTick(_ context.Context, req *pluginv1.OnTickRequest) (*pluginv1.OnTickResponse, error) {
	progressed := false
	for _, action := range req.GetActions() {
		if r.state.matchWinner != "" {
			break
		}
		if r.state.applyAction(action) {
			progressed = true
		}
	}

	resp := &pluginv1.OnTickResponse{}
	if progressed {
		delta, err := r.state.encode()
		if err != nil {
			return nil, err
		}
		resp.StateDelta = delta
	}
	if r.state.matchWinner != "" {
		resp.GameEnded = true
	}
	return resp, nil
}

func (r *rps) OnGameEnd(_ context.Context, _ *pluginv1.OnGameEndRequest) (*pluginv1.OnGameEndResponse, error) {
	final, err := r.state.encode()
	if err != nil {
		return nil, err
	}
	return &pluginv1.OnGameEndResponse{
		FinalState: final,
		PlayerResults: []*pluginv1.PlayerResult{
			{
				PlayerId: r.state.playerA,
				Rank:     r.state.rankOf(r.state.playerA),
				Stats:    map[string]string{"wins": itoa(r.state.scoreA)},
			},
			{
				PlayerId: r.state.playerB,
				Rank:     r.state.rankOf(r.state.playerB),
				Stats:    map[string]string{"wins": itoa(r.state.scoreB)},
			},
		},
	}, nil
}

func (*rps) Close() error { return nil }

// gameState carries everything the rules need between ticks.
type gameState struct {
	playerA string `json:"-"`
	playerB string `json:"-"`

	Round       int             `json:"round"`        // 1-indexed
	Scores      map[string]int  `json:"scores"`       // player_id → rounds won
	Choices     map[string]rune `json:"-"`            // current-round inputs
	History     []roundResult   `json:"history"`      // resolved rounds
	MatchWinner string          `json:"match_winner"` // player_id or ""

	scoreA      int
	scoreB      int
	matchWinner string
}

type roundResult struct {
	Round   int    `json:"round"`
	Winner  string `json:"winner"` // player_id or "" for tie
	ChoiceA rune   `json:"choice_a"`
	ChoiceB rune   `json:"choice_b"`
}

func newGameState() *gameState {
	return &gameState{
		Round:   1,
		Scores:  map[string]int{},
		Choices: map[string]rune{},
		History: []roundResult{},
	}
}

func (s *gameState) applyAction(action *pluginv1.PlayerAction) bool {
	payload := action.GetPayload()
	if len(payload) == 0 {
		return false
	}
	choice := normalize(rune(payload[0]))
	if choice == 0 {
		return false
	}
	if _, already := s.Choices[action.GetPlayerId()]; already {
		// Treat the first choice each round as authoritative; ignore
		// re-submissions until the round resolves.
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
		Round: s.Round, Winner: winner, ChoiceA: a, ChoiceB: b,
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
	s.Choices = map[string]rune{}
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

// decideRound implements the canonical RPS rules: R beats S, S beats P,
// P beats R; equal inputs tie.
func decideRound(a, b rune, idA, idB string) string {
	if a == b {
		return ""
	}
	if (a == 'R' && b == 'S') || (a == 'P' && b == 'R') || (a == 'S' && b == 'P') {
		return idA
	}
	return idB
}

// normalize accepts upper- or lower-case R/P/S input.
func normalize(c rune) rune {
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

// itoa converts a small non-negative int to a decimal string. Avoids
// pulling strconv just for this single conversion.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
