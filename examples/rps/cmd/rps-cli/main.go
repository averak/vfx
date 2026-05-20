// Package main is the RPS CLI client.
//
// It speaks the same Connect RPC + WebTransport stack as a real
// browser client, just from the command line. Two instances against
// the same vfx-rps deployment are enough to play a complete match.
//
// Flow:
//  1. Login anonymously to the gateway.
//  2. CreateTicket for the "rps" game mode.
//  3. WatchTicket until the matchmaker emits Matched.
//  4. Open a WebTransport session to the room with the session token.
//  5. Send R/P/S choices read from stdin (or chosen at random with
//     --auto), receiving state deltas back as JSON.
package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/webtransport-go"
	"google.golang.org/protobuf/proto"

	authv1 "github.com/averak/vfx/gen/go/vfx/v1/auth"
	"github.com/averak/vfx/gen/go/vfx/v1/auth/authconnect"
	matchv1 "github.com/averak/vfx/gen/go/vfx/v1/match"
	"github.com/averak/vfx/gen/go/vfx/v1/match/matchconnect"
	realtimev1 "github.com/averak/vfx/gen/go/vfx/v1/realtime"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "rps-cli: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	gatewayURL := flag.String("gateway", "http://localhost:8080", "gateway base URL")
	deviceID := flag.String("device", "", "anonymous device id (default: random per run)")
	nickname := flag.String("nickname", "", "nickname to register on first login")
	auto := flag.Bool("auto", false, "pick choices automatically every 500ms")
	flag.Parse()

	if *deviceID == "" {
		*deviceID = fmt.Sprintf("rps-cli-%d", time.Now().UnixNano())
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	httpClient := &http.Client{Timeout: 30 * time.Second}

	accessToken, playerID, err := login(ctx, httpClient, *gatewayURL, *deviceID, *nickname)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	fmt.Printf("[rps-cli] logged in as %s (player_id=%s)\n", *deviceID, playerID)

	ticketID, err := createTicket(ctx, httpClient, *gatewayURL, accessToken)
	if err != nil {
		return fmt.Errorf("create ticket: %w", err)
	}
	fmt.Printf("[rps-cli] ticket %s queued, waiting for match...\n", ticketID)

	endpoint, sessionToken, err := watchUntilMatched(ctx, httpClient, *gatewayURL, accessToken, ticketID)
	if err != nil {
		return fmt.Errorf("watch ticket: %w", err)
	}
	fmt.Printf("[rps-cli] matched! endpoint=%s\n", endpoint)

	return runRoomSession(ctx, endpoint, sessionToken, *auto)
}

func login(ctx context.Context, httpClient *http.Client, gatewayURL, deviceID, nickname string) (accessToken, playerID string, err error) {
	client := authconnect.NewAuthServiceClient(httpClient, gatewayURL)
	cred := &authv1.AnonymousCredential{DeviceId: &deviceID}
	if nickname != "" {
		cred.Nickname = &nickname
	}
	resp, err := client.Login(ctx, connect.NewRequest(&authv1.LoginRequest{
		Credential: &authv1.LoginRequest_Anonymous{Anonymous: cred},
	}))
	if err != nil {
		return "", "", err
	}
	return resp.Msg.GetAccessToken(), resp.Msg.GetPlayer().GetId(), nil
}

func createTicket(ctx context.Context, httpClient *http.Client, gatewayURL, accessToken string) (string, error) {
	client := matchconnect.NewMatchServiceClient(httpClient, gatewayURL)
	req := connect.NewRequest(&matchv1.CreateTicketRequest{GameMode: "rps"})
	req.Header().Set("Authorization", "Bearer "+accessToken)
	resp, err := client.CreateTicket(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Msg.GetTicketId(), nil
}

func watchUntilMatched(ctx context.Context, httpClient *http.Client, gatewayURL, accessToken, ticketID string) (endpoint, sessionToken string, err error) {
	client := matchconnect.NewMatchServiceClient(httpClient, gatewayURL)
	req := connect.NewRequest(&matchv1.WatchTicketRequest{TicketId: ticketID})
	req.Header().Set("Authorization", "Bearer "+accessToken)
	stream, err := client.WatchTicket(ctx, req)
	if err != nil {
		return "", "", err
	}
	//nolint:errcheck // Close errors at end-of-stream are not actionable.
	defer func() { _ = stream.Close() }()

	for stream.Receive() {
		msg := stream.Msg()
		switch ev := msg.GetEvent().(type) {
		case *matchv1.WatchTicketResponse_Queued:
			fmt.Printf("[rps-cli] queued (depth=%d)\n", ev.Queued.GetQueueDepth())
		case *matchv1.WatchTicketResponse_Matched:
			return ev.Matched.GetEndpoint(), ev.Matched.GetSessionToken(), nil
		case *matchv1.WatchTicketResponse_Failed:
			return "", "", fmt.Errorf("matchmaking failed: %s (%s)", ev.Failed.GetReason(), ev.Failed.GetMessage())
		}
	}
	if err := stream.Err(); err != nil {
		return "", "", err
	}
	return "", "", errors.New("stream closed without match")
}

func runRoomSession(ctx context.Context, endpoint, sessionToken string, auto bool) error {
	dialer := &webtransport.Dialer{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // Dev: self-signed cert on localhost.
			MinVersion:         tls.VersionTLS13,
		},
		QUICConfig: &quic.Config{
			EnableDatagrams:                  true,
			EnableStreamResetPartialDelivery: true,
		},
	}
	u := url.URL{
		Scheme: "https",
		Host:   endpoint,
		Path:   "/room/" + matchIDFromToken(sessionToken),
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+sessionToken)

	// In webtransport-go the http.Response body returned by Dial wraps
	// the WebTransport upgrade stream itself — closing it terminates
	// the session before any datagram can flow. We deliberately leave
	// the body open and silence the linter.
	_, session, err := dialer.Dial(ctx, u.String(), header) //nolint:bodyclose // see comment above.
	if err != nil {
		return fmt.Errorf("webtransport dial: %w", err)
	}
	//nolint:errcheck // best-effort cleanup
	defer func() { _ = session.CloseWithError(0, "client done") }()

	fmt.Printf("[rps-cli] connected to %s\n", u.String())

	go listenForFrames(ctx, session)

	if auto {
		return playAuto(ctx, session)
	}
	return playInteractive(ctx, session)
}

func listenForFrames(ctx context.Context, session *webtransport.Session) {
	for {
		raw, err := session.ReceiveDatagram(ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
				fmt.Printf("[rps-cli] datagram recv ended: %v\n", err)
			}
			return
		}
		var frame realtimev1.Frame
		if err := proto.Unmarshal(raw, &frame); err != nil {
			fmt.Printf("[rps-cli] malformed frame: %v\n", err)
			continue
		}
		printFrame(&frame)
	}
}

func printFrame(frame *realtimev1.Frame) {
	switch body := frame.GetBody().(type) {
	case *realtimev1.Frame_Delta:
		var state map[string]any
		if err := json.Unmarshal(body.Delta.GetPayload(), &state); err != nil {
			fmt.Printf("[rps-cli] delta (raw): %s\n", body.Delta.GetPayload())
			return
		}
		pretty, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			fmt.Printf("[rps-cli] delta (unformatted): %v\n", state)
			return
		}
		fmt.Printf("[rps-cli] state delta:\n%s\n", pretty)
	case *realtimev1.Frame_Event:
		fmt.Printf("[rps-cli] event %q: %s\n", body.Event.GetType(), body.Event.GetPayload())
	case *realtimev1.Frame_Error:
		fmt.Printf("[rps-cli] server error %q: %s\n", body.Error.GetCode(), body.Error.GetMessage())
	}
}

func playAuto(ctx context.Context, session *webtransport.Session) error {
	choices := []byte{'R', 'P', 'S'}
	ticker := time.NewTicker(800 * time.Millisecond)
	defer ticker.Stop()

	tick := uint32(0)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			//nolint:gosec // not cryptographic
			c := choices[rand.Intn(len(choices))]
			if err := sendChoice(session, tick, c); err != nil {
				return err
			}
			fmt.Printf("[rps-cli] sent choice: %c\n", c)
			tick++
		}
	}
}

func playInteractive(ctx context.Context, session *webtransport.Session) error {
	fmt.Println("[rps-cli] enter R/P/S each round, ENTER to send")
	reader := bufio.NewScanner(os.Stdin)
	tick := uint32(0)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if !reader.Scan() {
			return reader.Err()
		}
		line := strings.TrimSpace(reader.Text())
		if line == "" {
			continue
		}
		choice := strings.ToUpper(line)[0]
		if choice != 'R' && choice != 'P' && choice != 'S' {
			fmt.Println("[rps-cli] expected R, P, or S")
			continue
		}
		if err := sendChoice(session, tick, choice); err != nil {
			return err
		}
		tick++
	}
}

func sendChoice(session *webtransport.Session, tick uint32, choice byte) error {
	frame := &realtimev1.Frame{
		Body: &realtimev1.Frame_Input{
			Input: &realtimev1.PlayerInput{
				Tick:    tick,
				Payload: []byte{choice},
			},
		},
	}
	data, err := proto.Marshal(frame)
	if err != nil {
		return fmt.Errorf("marshal frame: %w", err)
	}
	return session.SendDatagram(data)
}

// matchIDFromToken extracts the match id from a session token's JWT
// payload. The client does not verify the signature — the server will
// do that on accept — but it needs the match id to build the URL.
func matchIDFromToken(tokenStr string) string {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64URLDecode(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		MID string `json:"mid"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	return claims.MID
}

func base64URLDecode(s string) ([]byte, error) {
	// JWT uses base64url without padding. encoding/base64 needs the
	// padding restored.
	if pad := len(s) % 4; pad != 0 {
		s += strings.Repeat("=", 4-pad)
	}
	return base64URLDecodeStd(s)
}
