package matchqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	valkeygo "github.com/valkey-io/valkey-go"

	"github.com/averak/vfx/internal/domain/match"
)

// Valkey is a match.Queue shared across gateway replicas. Tickets live
// in a per-game-mode sorted set (scored by creation time so Pending is
// FIFO), the latest event per ticket is a snapshot key, and live updates
// fan out over Valkey pub/sub. Claim removes tickets from the pending
// set atomically via a Lua script, so two gateways' matchmakers can run
// concurrently without double-matching.
type Valkey struct {
	client valkeygo.Client
	ttl    time.Duration
}

var _ match.Queue = (*Valkey)(nil)

// NewValkey wraps a connected client. ttl bounds how long a ticket and
// its event snapshot live; matchmaking is short, so an hour is ample and
// guarantees abandoned tickets are reclaimed.
func NewValkey(client valkeygo.Client) *Valkey {
	return &Valkey{client: client, ttl: time.Hour}
}

func ticketKey(id uuid.UUID) string  { return "vfx:mq:ticket:" + id.String() }
func eventKey(id uuid.UUID) string   { return "vfx:mq:event:" + id.String() }
func channelKey(id uuid.UUID) string { return "vfx:mq:channel:" + id.String() }
func pendingKey(gameMode string) string {
	return "vfx:mq:pending:" + gameMode
}

func (q *Valkey) ttlSeconds() int64 { return int64(q.ttl.Seconds()) }

// --- ticket / event serialization -----------------------------------

type ticketDTO struct {
	ID           string            `json:"id"`
	PlayerID     string            `json:"player_id"`
	GameMode     string            `json:"game_mode"`
	Rating       *float64          `json:"rating,omitempty"`
	Region       *string           `json:"region,omitempty"`
	PartyMembers []string          `json:"party_members,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
}

func marshalTicket(t *match.Ticket) ([]byte, error) {
	dto := ticketDTO{
		ID:         t.ID.String(),
		PlayerID:   t.PlayerID.String(),
		GameMode:   t.GameMode,
		Rating:     t.Rating,
		Region:     t.Region,
		Attributes: t.Attributes,
		CreatedAt:  t.CreatedAt,
	}
	for _, p := range t.PartyMembers {
		dto.PartyMembers = append(dto.PartyMembers, p.String())
	}
	return json.Marshal(dto)
}

func unmarshalTicket(raw string) (*match.Ticket, error) {
	var dto ticketDTO
	if err := json.Unmarshal([]byte(raw), &dto); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(dto.ID)
	if err != nil {
		return nil, err
	}
	playerID, err := uuid.Parse(dto.PlayerID)
	if err != nil {
		return nil, err
	}
	t := &match.Ticket{
		ID:         id,
		PlayerID:   playerID,
		GameMode:   dto.GameMode,
		Rating:     dto.Rating,
		Region:     dto.Region,
		Attributes: dto.Attributes,
		CreatedAt:  dto.CreatedAt,
	}
	for _, p := range dto.PartyMembers {
		pid, perr := uuid.Parse(p)
		if perr != nil {
			return nil, perr
		}
		t.PartyMembers = append(t.PartyMembers, pid)
	}
	return t, nil
}

type eventDTO struct {
	Type       string    `json:"type"` // queued | matched | failed
	QueuedAt   time.Time `json:"queued_at,omitempty"`
	QueueDepth int32     `json:"queue_depth,omitempty"`
	Match      *struct {
		MatchID      string    `json:"match_id"`
		Endpoint     string    `json:"endpoint"`
		SessionToken string    `json:"session_token"`
		ExpiresAt    time.Time `json:"expires_at"`
	} `json:"match,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

func marshalEvent(ev match.Event) ([]byte, error) {
	var dto eventDTO
	switch e := ev.(type) {
	case match.EventQueued:
		dto.Type = "queued"
		dto.QueuedAt = e.QueuedAt
		dto.QueueDepth = e.QueueDepth
	case match.EventMatched:
		dto.Type = "matched"
		dto.Match = &struct {
			MatchID      string    `json:"match_id"`
			Endpoint     string    `json:"endpoint"`
			SessionToken string    `json:"session_token"`
			ExpiresAt    time.Time `json:"expires_at"`
		}{
			MatchID:      e.Assignment.MatchID.String(),
			Endpoint:     e.Assignment.Endpoint,
			SessionToken: e.Assignment.SessionToken,
			ExpiresAt:    e.Assignment.ExpiresAt,
		}
	case match.EventFailed:
		dto.Type = "failed"
		dto.Reason = e.Reason
		dto.Message = e.Message
	default:
		return nil, fmt.Errorf("matchqueue: unknown event type %T", ev)
	}
	return json.Marshal(dto)
}

func unmarshalEvent(raw string) (match.Event, error) {
	var dto eventDTO
	if err := json.Unmarshal([]byte(raw), &dto); err != nil {
		return nil, err
	}
	switch dto.Type {
	case "queued":
		return match.EventQueued{QueuedAt: dto.QueuedAt, QueueDepth: dto.QueueDepth}, nil
	case "matched":
		matchID, err := uuid.Parse(dto.Match.MatchID)
		if err != nil {
			return nil, err
		}
		return match.EventMatched{Assignment: &match.Assignment{
			MatchID:      matchID,
			Endpoint:     dto.Match.Endpoint,
			SessionToken: dto.Match.SessionToken,
			ExpiresAt:    dto.Match.ExpiresAt,
		}}, nil
	case "failed":
		return match.EventFailed{Reason: dto.Reason, Message: dto.Message}, nil
	default:
		return nil, fmt.Errorf("matchqueue: unknown event type %q", dto.Type)
	}
}

func isTerminalEvent(ev match.Event) bool {
	switch ev.(type) {
	case match.EventMatched, match.EventFailed:
		return true
	}
	return false
}

// --- match.Queue ------------------------------------------------------

func (q *Valkey) Enqueue(ctx context.Context, t *match.Ticket) error {
	data, err := marshalTicket(t)
	if err != nil {
		return fmt.Errorf("matchqueue: marshal ticket: %w", err)
	}

	// SET NX makes Enqueue idempotent: a retried ticket id is a no-op.
	res := q.client.Do(ctx, q.client.B().Set().Key(ticketKey(t.ID)).Value(string(data)).Nx().ExSeconds(q.ttlSeconds()).Build())
	if setErr := res.Error(); setErr != nil {
		if valkeygo.IsValkeyNil(setErr) {
			return nil // already enqueued
		}
		return fmt.Errorf("matchqueue: set ticket: %w", setErr)
	}

	depth, err := q.client.Do(ctx, q.client.B().Zcard().Key(pendingKey(t.GameMode)).Build()).ToInt64()
	if err != nil {
		return fmt.Errorf("matchqueue: zcard: %w", err)
	}
	queued := match.EventQueued{QueuedAt: t.CreatedAt, QueueDepth: int32(depth) + 1} //nolint:gosec // queue depth fits int32.
	if err := q.setEvent(ctx, t.ID, queued); err != nil {
		return err
	}

	score := float64(t.CreatedAt.UnixNano())
	if err := q.client.Do(ctx, q.client.B().Zadd().Key(pendingKey(t.GameMode)).ScoreMember().ScoreMember(score, t.ID.String()).Build()).Error(); err != nil {
		return fmt.Errorf("matchqueue: zadd pending: %w", err)
	}
	return nil
}

func (q *Valkey) Cancel(ctx context.Context, ticketID uuid.UUID) error {
	t, err := q.getTicket(ctx, ticketID)
	if err != nil {
		return err
	}
	latest, err := q.latestEvent(ctx, ticketID)
	if err != nil {
		return err
	}
	if latest != nil && isTerminalEvent(latest) {
		return nil
	}
	return q.publish(ctx, t, match.EventFailed{
		Reason:  "cancelled",
		Message: "ticket was cancelled by the client",
	})
}

func (q *Valkey) Publish(ctx context.Context, ticketID uuid.UUID, event match.Event) error {
	t, err := q.getTicket(ctx, ticketID)
	if err != nil {
		return err
	}
	return q.publish(ctx, t, event)
}

// publish writes the snapshot, removes the ticket from the pending pool
// on a terminal event, and fans the event out to live subscribers.
func (q *Valkey) publish(ctx context.Context, t *match.Ticket, event match.Event) error {
	if err := q.setEvent(ctx, t.ID, event); err != nil {
		return err
	}
	if isTerminalEvent(event) {
		if err := q.client.Do(ctx, q.client.B().Zrem().Key(pendingKey(t.GameMode)).Member(t.ID.String()).Build()).Error(); err != nil {
			return fmt.Errorf("matchqueue: zrem pending: %w", err)
		}
	}
	data, err := marshalEvent(event)
	if err != nil {
		return err
	}
	if err := q.client.Do(ctx, q.client.B().Publish().Channel(channelKey(t.ID)).Message(string(data)).Build()).Error(); err != nil {
		return fmt.Errorf("matchqueue: publish: %w", err)
	}
	return nil
}

func (q *Valkey) Pending(ctx context.Context, gameMode string) ([]*match.Ticket, error) {
	ids, err := q.client.Do(ctx, q.client.B().Zrange().Key(pendingKey(gameMode)).Min("0").Max("-1").Build()).AsStrSlice()
	if err != nil {
		return nil, fmt.Errorf("matchqueue: zrange pending: %w", err)
	}
	out := make([]*match.Ticket, 0, len(ids))
	for _, idStr := range ids {
		id, perr := uuid.Parse(idStr)
		if perr != nil {
			continue
		}
		t, terr := q.getTicket(ctx, id)
		if terr != nil {
			// The ticket key expired out from under the pending set; skip
			// it. (A future sweep could ZREM these.)
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

const claimScript = `
for i=1,#ARGV do
  if redis.call('ZSCORE', KEYS[1], ARGV[i]) == false then return 0 end
end
for i=1,#ARGV do
  redis.call('ZREM', KEYS[1], ARGV[i])
end
return 1
`

func (q *Valkey) Claim(ctx context.Context, gameMode string, ticketIDs []uuid.UUID) (bool, error) {
	args := make([]string, len(ticketIDs))
	for i, id := range ticketIDs {
		args[i] = id.String()
	}
	res, err := q.client.Do(ctx, q.client.B().Eval().Script(claimScript).Numkeys(1).Key(pendingKey(gameMode)).Arg(args...).Build()).ToInt64()
	if err != nil {
		return false, fmt.Errorf("matchqueue: claim eval: %w", err)
	}
	return res == 1, nil
}

func (q *Valkey) Depth(ctx context.Context, gameMode string) (int32, error) {
	depth, err := q.client.Do(ctx, q.client.B().Zcard().Key(pendingKey(gameMode)).Build()).ToInt64()
	if err != nil {
		return 0, fmt.Errorf("matchqueue: zcard: %w", err)
	}
	return int32(depth), nil //nolint:gosec // queue depth fits int32.
}

func (q *Valkey) Subscribe(ctx context.Context, ticketID uuid.UUID) (<-chan match.Event, error) {
	if _, err := q.getTicket(ctx, ticketID); err != nil {
		return nil, err
	}

	ch := make(chan match.Event, 4)
	latest, err := q.latestEvent(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if latest != nil {
		ch <- latest
		if isTerminalEvent(latest) {
			close(ch)
			return ch, nil
		}
	}

	subCtx, cancel := context.WithCancel(ctx)
	go func() {
		defer close(ch)
		defer cancel()
		// Receive blocks on a dedicated connection until subCtx is done.
		// A terminal event cancels subCtx, which unsubscribes and returns;
		// the error it returns then is the expected context cancellation.
		_ = q.client.Receive(subCtx, q.client.B().Subscribe().Channel(channelKey(ticketID)).Build(), func(msg valkeygo.PubSubMessage) { //nolint:errcheck // unsubscribe-on-cancel returns ctx.Err, which is expected.
			ev, decErr := unmarshalEvent(msg.Message)
			if decErr != nil {
				return
			}
			select {
			case ch <- ev:
			default:
			}
			if isTerminalEvent(ev) {
				cancel()
			}
		})
	}()
	return ch, nil
}

// --- helpers ----------------------------------------------------------

func (q *Valkey) setEvent(ctx context.Context, ticketID uuid.UUID, event match.Event) error {
	data, err := marshalEvent(event)
	if err != nil {
		return err
	}
	if err := q.client.Do(ctx, q.client.B().Set().Key(eventKey(ticketID)).Value(string(data)).ExSeconds(q.ttlSeconds()).Build()).Error(); err != nil {
		return fmt.Errorf("matchqueue: set event: %w", err)
	}
	return nil
}

func (q *Valkey) latestEvent(ctx context.Context, ticketID uuid.UUID) (match.Event, error) {
	raw, err := q.client.Do(ctx, q.client.B().Get().Key(eventKey(ticketID)).Build()).ToString()
	if err != nil {
		if valkeygo.IsValkeyNil(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("matchqueue: get event: %w", err)
	}
	return unmarshalEvent(raw)
}

func (q *Valkey) getTicket(ctx context.Context, ticketID uuid.UUID) (*match.Ticket, error) {
	raw, err := q.client.Do(ctx, q.client.B().Get().Key(ticketKey(ticketID)).Build()).ToString()
	if err != nil {
		if valkeygo.IsValkeyNil(err) {
			return nil, match.ErrTicketNotFound
		}
		return nil, fmt.Errorf("matchqueue: get ticket: %w", err)
	}
	return unmarshalTicket(raw)
}
