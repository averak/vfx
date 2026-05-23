// Package main is the RPS CLI client.
//
// It uses the vfx Go client SDK to log in, queue a ticket, wait for a match, and play rock-paper-scissors over WebTransport.
// Two instances against the same vfx-rps deployment play a complete match.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	realtimev1 "github.com/averak/vfx/gen/go/vfx/v1/realtime"
	vfxclient "github.com/averak/vfx/sdk/client/go"
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
	auto := flag.Bool("auto", false, "pick choices automatically every ~800ms")
	flag.Parse()

	if *deviceID == "" {
		*deviceID = fmt.Sprintf("rps-cli-%d", time.Now().UnixNano())
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	client := vfxclient.New(*gatewayURL)

	if err := client.LoginAnonymous(ctx, *deviceID, *nickname); err != nil {
		return err
	}
	fmt.Printf("[rps-cli] logged in as %s (player_id=%s)\n", *deviceID, client.Player().GetId())

	ticketID, err := client.CreateTicket(ctx, "rps")
	if err != nil {
		return err
	}
	fmt.Printf("[rps-cli] ticket %s queued, waiting for match...\n", ticketID)

	match, err := client.WaitForMatch(ctx, ticketID)
	if err != nil {
		return err
	}
	fmt.Printf("[rps-cli] matched! endpoint=%s\n", match.Endpoint)

	session, err := match.Connect(ctx, vfxclient.WithInsecureSkipVerify())
	if err != nil {
		return err
	}
	//nolint:errcheck // Close errors during teardown are not actionable.
	defer func() { _ = session.Close() }()
	fmt.Printf("[rps-cli] connected to room\n")

	matchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go watchFrames(cancel, session)

	if *auto {
		return playAuto(matchCtx, session)
	}
	return playInteractive(matchCtx, session)
}

// watchFrames prints inbound frames and stops the client when the match ends (a "game_ended" system event) or the session closes, cancelling the play loop via cancel.
func watchFrames(cancel context.CancelFunc, session *vfxclient.Session) {
	defer cancel()
	for frame := range session.Frames() {
		switch body := frame.GetBody().(type) {
		case *realtimev1.Frame_Delta:
			var state map[string]any
			if err := json.Unmarshal(body.Delta.GetPayload(), &state); err != nil {
				fmt.Printf("[rps-cli] delta (raw): %s\n", body.Delta.GetPayload())
				continue
			}
			pretty, err := json.MarshalIndent(state, "", "  ")
			if err != nil {
				fmt.Printf("[rps-cli] delta (unformatted): %v\n", state)
				continue
			}
			fmt.Printf("[rps-cli] state delta:\n%s\n", pretty)
		case *realtimev1.Frame_Event:
			fmt.Printf("[rps-cli] event %q: %s\n", body.Event.GetType(), body.Event.GetPayload())
			if body.Event.GetType() == "game_ended" {
				fmt.Println("[rps-cli] match ended; disconnecting")
				return
			}
		case *realtimev1.Frame_Error:
			fmt.Printf("[rps-cli] server error %q: %s\n", body.Error.GetCode(), body.Error.GetMessage())
		}
	}
}

func playAuto(ctx context.Context, session *vfxclient.Session) error {
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
			if err := session.SendInput(tick, []byte{c}); err != nil {
				return err
			}
			fmt.Printf("[rps-cli] sent choice: %c\n", c)
			tick++
		}
	}
}

func playInteractive(ctx context.Context, session *vfxclient.Session) error {
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
		if err := session.SendInput(tick, []byte{choice}); err != nil {
			return err
		}
		tick++
	}
}
