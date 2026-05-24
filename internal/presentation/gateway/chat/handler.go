// Package chat wires the ChatService onto the usecase.
//
// The handler reads the sender from the auth context, parses the other party's id, and maps domain validation errors to Connect codes; message rules stay in the domain/usecase.
package chat

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatv1 "github.com/averak/vfx/gen/go/vfx/v1/chat"
	"github.com/averak/vfx/gen/go/vfx/v1/chat/chatconnect"
	domainchat "github.com/averak/vfx/internal/domain/chat"
	"github.com/averak/vfx/internal/infra/connectrpc/authctx"
	usecasechat "github.com/averak/vfx/internal/usecase/chat"
)

type Handler struct {
	uc *usecasechat.Usecase
}

var _ chatconnect.ChatServiceHandler = (*Handler)(nil)

func New(uc *usecasechat.Usecase) *Handler {
	return &Handler{uc: uc}
}

func (h *Handler) SendDirectMessage(ctx context.Context, req *connect.Request[chatv1.SendDirectMessageRequest]) (*connect.Response[chatv1.SendDirectMessageResponse], error) {
	sender, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	recipient, err := parsePlayerID(req.Msg.GetRecipientId())
	if err != nil {
		return nil, err
	}
	msg, err := h.uc.SendDirectMessage(ctx, sender, recipient, req.Msg.GetBody())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&chatv1.SendDirectMessageResponse{Message: toMessagePb(msg)}), nil
}

func (h *Handler) ListDirectMessages(ctx context.Context, req *connect.Request[chatv1.ListDirectMessagesRequest]) (*connect.Response[chatv1.ListDirectMessagesResponse], error) {
	me, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	other, err := parsePlayerID(req.Msg.GetOtherPlayerId())
	if err != nil {
		return nil, err
	}
	var before time.Time
	if req.Msg.GetBefore() != nil {
		before = req.Msg.GetBefore().AsTime()
	}
	messages, err := h.uc.ListDirectMessages(ctx, me, other, before, int(req.Msg.GetLimit()))
	if err != nil {
		return nil, toConnectError(err)
	}
	out := make([]*chatv1.Message, len(messages))
	for i, m := range messages {
		out[i] = toMessagePb(m)
	}
	return connect.NewResponse(&chatv1.ListDirectMessagesResponse{Messages: out}), nil
}

func (h *Handler) SendChannelMessage(ctx context.Context, req *connect.Request[chatv1.SendChannelMessageRequest]) (*connect.Response[chatv1.SendChannelMessageResponse], error) {
	sender, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	channelID, err := parsePlayerID(req.Msg.GetChannelId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid channel id"))
	}
	msg, err := h.uc.SendChannelMessage(ctx, sender, channelID, req.Msg.GetBody())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&chatv1.SendChannelMessageResponse{Message: toChannelMessagePb(msg)}), nil
}

func (h *Handler) ListChannelMessages(ctx context.Context, req *connect.Request[chatv1.ListChannelMessagesRequest]) (*connect.Response[chatv1.ListChannelMessagesResponse], error) {
	me, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	channelID, err := parsePlayerID(req.Msg.GetChannelId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid channel id"))
	}
	var before time.Time
	if req.Msg.GetBefore() != nil {
		before = req.Msg.GetBefore().AsTime()
	}
	messages, err := h.uc.ListChannelMessages(ctx, me, channelID, before, int(req.Msg.GetLimit()))
	if err != nil {
		return nil, toConnectError(err)
	}
	out := make([]*chatv1.ChannelMessage, len(messages))
	for i, m := range messages {
		out[i] = toChannelMessagePb(m)
	}
	return connect.NewResponse(&chatv1.ListChannelMessagesResponse{Messages: out}), nil
}

func (h *Handler) SubscribeChannel(ctx context.Context, req *connect.Request[chatv1.SubscribeChannelRequest], stream *connect.ServerStream[chatv1.SubscribeChannelResponse]) error {
	me, err := requireAuth(ctx)
	if err != nil {
		return err
	}
	channelID, err := parsePlayerID(req.Msg.GetChannelId())
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("invalid channel id"))
	}
	messages, err := h.uc.SubscribeChannel(ctx, me, channelID)
	if err != nil {
		return toConnectError(err)
	}
	for m := range messages {
		if err := stream.Send(&chatv1.SubscribeChannelResponse{Message: toChannelMessagePb(m)}); err != nil {
			return err
		}
	}
	return nil
}

func requireAuth(ctx context.Context) (uuid.UUID, error) {
	id, ok := authctx.From(ctx)
	if !ok {
		return uuid.Nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	return id, nil
}

func parsePlayerID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid player id"))
	}
	return id, nil
}

func toMessagePb(m *domainchat.Message) *chatv1.Message {
	return &chatv1.Message{
		Id:          m.ID.String(),
		SenderId:    m.SenderID.String(),
		RecipientId: m.RecipientID.String(),
		Body:        m.Body,
		SentAt:      timestamppb.New(m.SentAt),
	}
}

func toChannelMessagePb(m *domainchat.ChannelMessage) *chatv1.ChannelMessage {
	return &chatv1.ChannelMessage{
		Id:        m.ID.String(),
		ChannelId: m.ChannelID.String(),
		SenderId:  m.SenderID.String(),
		Body:      m.Body,
		SentAt:    timestamppb.New(m.SentAt),
	}
}

func toConnectError(err error) error {
	switch {
	case errors.Is(err, domainchat.ErrSelfMessage), errors.Is(err, domainchat.ErrInvalidBody):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, domainchat.ErrNotChannelMember):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}
