// Package vfxclient is the Go client SDK for vfx.
//
// It wraps the generated Connect RPC clients and the WebTransport handshake behind a small, task-oriented API:
//
//	c := vfxclient.New("http://localhost:8080")
//	if err := c.LoginAnonymous(ctx, "device-1", "Alice"); err != nil { ... }
//	ticket, _ := c.CreateTicket(ctx, "rps")
//	match, _ := c.WaitForMatch(ctx, ticket)
//	session, _ := match.Connect(ctx)
//	session.SendInput(0, []byte{'R'})
//	for frame := range session.Frames() { ... }
//
// The SDK is used by the example rps-cli and by integration tests; it is the reference for how a native (non-browser) client talks to vfx.
package vfxclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"

	authv1 "github.com/averak/vfx/gen/go/vfx/v1/auth"
	"github.com/averak/vfx/gen/go/vfx/v1/auth/authconnect"
	matchv1 "github.com/averak/vfx/gen/go/vfx/v1/match"
	"github.com/averak/vfx/gen/go/vfx/v1/match/matchconnect"
	"github.com/averak/vfx/gen/go/vfx/v1/storage/storageconnect"
)

// Client is a logged-in (or about-to-be) vfx client.
// It is not safe for concurrent use across goroutines; create one per player.
type Client struct {
	gatewayURL string
	httpClient *http.Client

	auth         authconnect.AuthServiceClient
	match        matchconnect.MatchServiceClient
	playerData   storageconnect.PlayerDataStorageServiceClient
	titleStorage storageconnect.TitleStorageServiceClient

	accessToken  string
	refreshToken string
	player       *authv1.Player
}

type Option func(*Client)

func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

func New(gatewayURL string, opts ...Option) *Client {
	c := &Client{
		gatewayURL: gatewayURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	c.auth = authconnect.NewAuthServiceClient(c.httpClient, gatewayURL)
	c.match = matchconnect.NewMatchServiceClient(c.httpClient, gatewayURL)
	c.playerData = storageconnect.NewPlayerDataStorageServiceClient(c.httpClient, gatewayURL)
	c.titleStorage = storageconnect.NewTitleStorageServiceClient(c.httpClient, gatewayURL)
	return c
}

// Player returns the authenticated player, or nil before login.
func (c *Client) Player() *authv1.Player { return c.player }

// AccessToken returns the current access token, or "" before login.
func (c *Client) AccessToken() string { return c.accessToken }

// LoginAnonymous logs in with an anonymous credential.
// A stable deviceID returns the same player across calls; an empty deviceID mints a fresh player.
// nickname is applied only on first registration.
func (c *Client) LoginAnonymous(ctx context.Context, deviceID, nickname string) error {
	cred := &authv1.AnonymousCredential{}
	if deviceID != "" {
		cred.DeviceId = &deviceID
	}
	if nickname != "" {
		cred.Nickname = &nickname
	}
	resp, err := c.auth.Login(ctx, connect.NewRequest(&authv1.LoginRequest{
		Credential: &authv1.LoginRequest_Anonymous{Anonymous: cred},
	}))
	if err != nil {
		return fmt.Errorf("vfxclient: login: %w", err)
	}
	c.accessToken = resp.Msg.GetAccessToken()
	c.refreshToken = resp.Msg.GetRefreshToken()
	c.player = resp.Msg.GetPlayer()
	return nil
}

func (c *Client) CreateTicket(ctx context.Context, gameMode string) (string, error) {
	req := connect.NewRequest(&matchv1.CreateTicketRequest{GameMode: gameMode})
	c.authorize(req.Header())
	resp, err := c.match.CreateTicket(ctx, req)
	if err != nil {
		return "", fmt.Errorf("vfxclient: create ticket: %w", err)
	}
	return resp.Msg.GetTicketId(), nil
}

type Match struct {
	client       *Client
	Endpoint     string
	SessionToken string
}

// WaitForMatch follows the WatchTicket stream until the ticket is matched, then returns the connection details.
// It returns an error if matchmaking fails or the context is cancelled.
func (c *Client) WaitForMatch(ctx context.Context, ticketID string) (*Match, error) {
	req := connect.NewRequest(&matchv1.WatchTicketRequest{TicketId: ticketID})
	c.authorize(req.Header())
	stream, err := c.match.WatchTicket(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vfxclient: watch ticket: %w", err)
	}
	//nolint:errcheck // Close errors at end-of-stream are not actionable.
	defer func() { _ = stream.Close() }()

	for stream.Receive() {
		switch ev := stream.Msg().GetEvent().(type) {
		case *matchv1.WatchTicketResponse_Queued:
			// keep waiting
		case *matchv1.WatchTicketResponse_Matched:
			return &Match{
				client:       c,
				Endpoint:     ev.Matched.GetEndpoint(),
				SessionToken: ev.Matched.GetSessionToken(),
			}, nil
		case *matchv1.WatchTicketResponse_Failed:
			return nil, fmt.Errorf("vfxclient: matchmaking failed: %s (%s)",
				ev.Failed.GetReason(), ev.Failed.GetMessage())
		}
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("vfxclient: watch ticket stream: %w", err)
	}
	return nil, errors.New("vfxclient: ticket stream closed without a match")
}

// GetCurrentMatch returns the player's active match assignment, or nil if they are not in one.
// It is how a client recovers after a dropped WebTransport session: when Session.Frames closes unexpectedly (rather than after a game_ended event), call this and reconnect to the returned Match without re-queuing.
func (c *Client) GetCurrentMatch(ctx context.Context) (*Match, error) {
	req := connect.NewRequest(&matchv1.GetCurrentMatchRequest{})
	c.authorize(req.Header())
	resp, err := c.match.GetCurrentMatch(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vfxclient: get current match: %w", err)
	}
	m := resp.Msg.GetMatch()
	if m == nil {
		return nil, nil //nolint:nilnil // no active match is a normal "nothing to reconnect to".
	}
	return &Match{
		client:       c,
		Endpoint:     m.GetEndpoint(),
		SessionToken: m.GetSessionToken(),
	}, nil
}

func (c *Client) authorize(h http.Header) {
	if c.accessToken != "" {
		h.Set("Authorization", "Bearer "+c.accessToken)
	}
}
