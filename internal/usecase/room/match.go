// Package room is the room daemon's match orchestrator.
//
// One Match value owns one in-progress game: its players, the plugin instance running its rules, the tick loop, and the I/O fan-in/fan-out between the WebTransport sessions and the plugin.
// The Manager holds the active Matches, keyed by match id.
package room

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	pluginv1 "github.com/averak/vfx/gen/go/plugin/v1"
	realtimev1 "github.com/averak/vfx/gen/go/vfx/v1/realtime"
	"github.com/averak/vfx/internal/domain/plugin"
)

// PlayerIO is the room's view of a WebTransport session, so a match can be driven by a fake in tests.
type PlayerIO interface {
	// SendFrame may drop frames when the underlying transport is congested.
	SendFrame(frame *realtimev1.Frame) error
	Close()
}

type Match struct {
	id      uuid.UUID
	logger  *slog.Logger
	plugin  plugin.Plugin
	metrics Metrics

	mu          sync.Mutex
	players     map[uuid.UUID]*matchPlayer
	currentTick uint32

	tickRateHz uint32
	inputs     chan *pluginv1.PlayerAction
	events     chan *pluginv1.NetworkEvent
	done       chan struct{}
}

type matchPlayer struct {
	id uuid.UUID
	io PlayerIO
}

// NewMatch requires an already-initialised Plugin; Match treats it as a pure tick processor.
func NewMatch(id uuid.UUID, p plugin.Plugin, tickRateHz uint32, logger *slog.Logger, metrics Metrics) *Match {
	if metrics == nil {
		metrics = noopMetrics{}
	}
	return &Match{
		id:         id,
		plugin:     p,
		metrics:    metrics,
		players:    make(map[uuid.UUID]*matchPlayer),
		tickRateHz: tickRateHz,
		inputs:     make(chan *pluginv1.PlayerAction, 256),
		events:     make(chan *pluginv1.NetworkEvent, 32),
		done:       make(chan struct{}),
		logger:     logger.With("match_id", id),
	}
}

// Join is safe to call from a handler goroutine while the tick loop is running.
func (m *Match) Join(playerID uuid.UUID, io PlayerIO) error {
	m.mu.Lock()
	if _, exists := m.players[playerID]; exists {
		m.mu.Unlock()
		return errors.New("match: player already joined")
	}
	m.players[playerID] = &matchPlayer{id: playerID, io: io}
	m.mu.Unlock()

	select {
	case m.events <- &pluginv1.NetworkEvent{
		Event: &pluginv1.NetworkEvent_Joined{
			Joined: &pluginv1.PlayerJoined{PlayerId: playerID.String()},
		},
	}:
	default:
		// If the event channel is full we drop the join notice; the next tick still exposes the player via any inputs they submit.
		// Recording the drop helps diagnose stalls.
		m.logger.Warn("match: dropped join event", "player_id", playerID)
	}
	return nil
}

// Leave makes the plugin see a PlayerLeft event on the next tick.
func (m *Match) Leave(playerID uuid.UUID, reason string) {
	m.mu.Lock()
	p, ok := m.players[playerID]
	if ok {
		delete(m.players, playerID)
	}
	m.mu.Unlock()
	if !ok {
		return
	}
	p.io.Close()

	select {
	case m.events <- &pluginv1.NetworkEvent{
		Event: &pluginv1.NetworkEvent_Left{
			Left: &pluginv1.PlayerLeft{PlayerId: playerID.String(), Reason: reason},
		},
	}:
	default:
		m.logger.Warn("match: dropped leave event", "player_id", playerID)
	}
}

// SubmitInput buffers the action so a fast client cannot block the tick loop; when the buffer fills, inputs are dropped and the client recovers on the next snapshot.
func (m *Match) SubmitInput(playerID uuid.UUID, clientTick uint32, payload []byte) {
	action := &pluginv1.PlayerAction{
		PlayerId:   playerID.String(),
		ClientTick: clientTick,
		Payload:    payload,
	}
	select {
	case m.inputs <- action:
	default:
		m.logger.Warn("match: dropped player input (queue full)",
			"player_id", playerID, "client_tick", clientTick)
	}
}

// Run returns when the plugin signals game_ended or ctx is cancelled.
func (m *Match) Run(ctx context.Context) error {
	defer close(m.done)
	defer func() {
		if err := m.plugin.Close(); err != nil {
			m.logger.Warn("match: plugin close failed", "err", err)
		}
	}()

	tickInterval := tickInterval(m.tickRateHz)
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("match: cancelled", "tick", m.currentTick)
			m.closeAllPlayers()
			return ctx.Err()
		case <-ticker.C:
			finished, err := m.tick(ctx)
			if err != nil {
				m.logger.Error("match: tick failed", "tick", m.currentTick, "err", err)
				m.closeAllPlayers()
				return err
			}
			if finished {
				m.finalise(ctx)
				return nil
			}
		}
	}
}

func (m *Match) tick(ctx context.Context) (bool, error) {
	now := time.Now().UTC()

	actions := drainActions(m.inputs)
	events := drainEvents(m.events)

	req := &pluginv1.OnTickRequest{
		Tick:          m.currentTick,
		Timestamp:     timestamppb.New(now),
		Actions:       actions,
		NetworkEvents: events,
	}

	tickStart := time.Now()
	resp, err := m.plugin.OnTick(ctx, req)
	m.metrics.ObserveTick(time.Since(tickStart))
	if err != nil {
		return false, fmt.Errorf("plugin OnTick: %w", err)
	}

	if len(resp.GetStateDelta()) > 0 {
		m.broadcast(&realtimev1.Frame{
			Body: &realtimev1.Frame_Delta{
				Delta: &realtimev1.StateDelta{
					FromTick: m.currentTick,
					ToTick:   m.currentTick + 1,
					Payload:  resp.GetStateDelta(),
				},
			},
		})
	}

	for _, ev := range resp.GetEvents() {
		m.dispatchEvent(ev)
	}

	m.currentTick++
	return resp.GetGameEnded(), nil
}

func (m *Match) finalise(ctx context.Context) {
	endReq := &pluginv1.OnGameEndRequest{FinalTick: m.currentTick}
	resp, err := m.plugin.OnGameEnd(ctx, endReq)
	if err != nil {
		m.logger.Error("match: OnGameEnd failed", "err", err)
	}

	// Tell clients the match is over before closing their sessions.
	// This is a SystemEvent, so SendFrame delivers it reliably over a stream; the OnGameEnd result (final ranks) rides along as the payload.
	var payload []byte
	if resp != nil {
		if data, mErr := proto.Marshal(resp); mErr == nil {
			payload = data
		}
	}
	m.broadcast(&realtimev1.Frame{
		Body: &realtimev1.Frame_Event{
			Event: &realtimev1.SystemEvent{Type: "game_ended", Payload: payload},
		},
	})

	m.closeAllPlayers()
}

func (m *Match) closeAllPlayers() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.players {
		p.io.Close()
	}
	m.players = make(map[uuid.UUID]*matchPlayer)
}

func (m *Match) broadcast(frame *realtimev1.Frame) {
	m.mu.Lock()
	players := make([]*matchPlayer, 0, len(m.players))
	for _, p := range m.players {
		players = append(players, p)
	}
	m.mu.Unlock()

	for _, p := range players {
		if err := p.io.SendFrame(frame); err != nil {
			m.logger.Warn("match: broadcast failed",
				"player_id", p.id, "err", err)
		}
	}
}

func (m *Match) dispatchEvent(ev *pluginv1.OutboundEvent) {
	frame := &realtimev1.Frame{
		Body: &realtimev1.Frame_Event{
			Event: &realtimev1.SystemEvent{
				Type:    ev.GetType(),
				Payload: ev.GetPayload(),
			},
		},
	}

	if len(ev.GetRecipients()) == 0 {
		m.broadcast(frame)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, rid := range ev.GetRecipients() {
		id, err := uuid.Parse(rid)
		if err != nil {
			continue
		}
		p, ok := m.players[id]
		if !ok {
			continue
		}
		if err := p.io.SendFrame(frame); err != nil {
			m.logger.Warn("match: targeted send failed",
				"player_id", p.id, "err", err)
		}
	}
}

func (m *Match) Done() <-chan struct{} { return m.done }

func drainActions(ch chan *pluginv1.PlayerAction) []*pluginv1.PlayerAction {
	out := make([]*pluginv1.PlayerAction, 0, len(ch))
	for {
		select {
		case a := <-ch:
			out = append(out, a)
		default:
			return out
		}
	}
}

func drainEvents(ch chan *pluginv1.NetworkEvent) []*pluginv1.NetworkEvent {
	out := make([]*pluginv1.NetworkEvent, 0, len(ch))
	for {
		select {
		case e := <-ch:
			out = append(out, e)
		default:
			return out
		}
	}
}

func tickInterval(rateHz uint32) time.Duration {
	if rateHz == 0 {
		// 0 means event-driven; we still wake up periodically to notice queued events without spinning.
		return 50 * time.Millisecond
	}
	return time.Second / time.Duration(rateHz)
}
