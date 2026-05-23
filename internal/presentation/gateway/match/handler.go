// Package match wires MatchService onto the usecase.
package match

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	matchv1 "github.com/averak/vfx/gen/go/vfx/v1/match"
	"github.com/averak/vfx/gen/go/vfx/v1/match/matchconnect"
	domainmatch "github.com/averak/vfx/internal/domain/match"
	"github.com/averak/vfx/internal/infra/connectrpc/authctx"
	usecasematch "github.com/averak/vfx/internal/usecase/match"
)

type Handler struct {
	uc *usecasematch.Usecase
}

var _ matchconnect.MatchServiceHandler = (*Handler)(nil)

func New(uc *usecasematch.Usecase) *Handler {
	return &Handler{uc: uc}
}

func (h *Handler) CreateTicket(ctx context.Context, req *connect.Request[matchv1.CreateTicketRequest]) (*connect.Response[matchv1.CreateTicketResponse], error) {
	playerID, ok := authctx.From(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if req.Msg.GetGameMode() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("game_mode is required"))
	}

	partyMembers, err := parsePartyMembers(req.Msg.GetPartyMembers())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	input := &usecasematch.TicketInput{
		PlayerID:     playerID,
		GameMode:     req.Msg.GetGameMode(),
		PartyMembers: partyMembers,
		Attributes:   req.Msg.GetAttributes(),
	}
	if req.Msg.Rating != nil {
		v := req.Msg.GetRating()
		input.Rating = &v
	}
	if req.Msg.Region != nil {
		v := req.Msg.GetRegion()
		input.Region = &v
	}

	ticketID, err := h.uc.CreateTicket(ctx, input)
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&matchv1.CreateTicketResponse{
		TicketId: ticketID.String(),
	}), nil
}

func (h *Handler) WatchTicket(ctx context.Context, req *connect.Request[matchv1.WatchTicketRequest], stream *connect.ServerStream[matchv1.WatchTicketResponse]) error {
	if _, ok := authctx.From(ctx); !ok {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	ticketID, err := uuid.Parse(req.Msg.GetTicketId())
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("invalid ticket_id"))
	}

	events, err := h.uc.WatchTicket(ctx, ticketID)
	if err != nil {
		return toConnectError(err)
	}

	for ev := range events {
		if err := stream.Send(toWatchResponse(ev)); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) CancelTicket(ctx context.Context, req *connect.Request[matchv1.CancelTicketRequest]) (*connect.Response[matchv1.CancelTicketResponse], error) {
	if _, ok := authctx.From(ctx); !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	ticketID, err := uuid.Parse(req.Msg.GetTicketId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid ticket_id"))
	}
	if err := h.uc.CancelTicket(ctx, ticketID); err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&matchv1.CancelTicketResponse{}), nil
}

func (h *Handler) GetCurrentMatch(ctx context.Context, _ *connect.Request[matchv1.GetCurrentMatchRequest]) (*connect.Response[matchv1.GetCurrentMatchResponse], error) {
	playerID, ok := authctx.From(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	assignment, err := h.uc.GetCurrentMatch(ctx, playerID)
	if err != nil {
		return nil, toConnectError(err)
	}
	resp := &matchv1.GetCurrentMatchResponse{}
	if assignment != nil {
		resp.Match = toCurrentMatchPb(assignment)
	}
	return connect.NewResponse(resp), nil
}

func parsePartyMembers(ids []string) ([]uuid.UUID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		parsed, err := uuid.Parse(id)
		if err != nil {
			return nil, errors.New("invalid party_member id")
		}
		out = append(out, parsed)
	}
	return out, nil
}

func toWatchResponse(ev domainmatch.Event) *matchv1.WatchTicketResponse {
	switch e := ev.(type) {
	case domainmatch.EventQueued:
		return &matchv1.WatchTicketResponse{
			Event: &matchv1.WatchTicketResponse_Queued{
				Queued: &matchv1.TicketQueued{
					QueuedAt:   timestamppb.New(e.QueuedAt),
					QueueDepth: e.QueueDepth,
				},
			},
		}
	case domainmatch.EventMatched:
		return &matchv1.WatchTicketResponse{
			Event: &matchv1.WatchTicketResponse_Matched{
				Matched: &matchv1.TicketMatched{
					Endpoint:     e.Assignment.Endpoint,
					SessionToken: e.Assignment.SessionToken,
					ExpiresAt:    timestamppb.New(e.Assignment.ExpiresAt),
				},
			},
		}
	case domainmatch.EventFailed:
		return &matchv1.WatchTicketResponse{
			Event: &matchv1.WatchTicketResponse_Failed{
				Failed: &matchv1.TicketFailed{
					Reason:  e.Reason,
					Message: e.Message,
				},
			},
		}
	}
	return &matchv1.WatchTicketResponse{}
}

func toCurrentMatchPb(a *domainmatch.Assignment) *matchv1.CurrentMatch {
	return &matchv1.CurrentMatch{
		MatchId:      a.MatchID.String(),
		Endpoint:     a.Endpoint,
		SessionToken: a.SessionToken,
		ExpiresAt:    timestamppb.New(a.ExpiresAt),
	}
}

func toConnectError(err error) error {
	switch {
	case errors.Is(err, domainmatch.ErrTicketNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}
