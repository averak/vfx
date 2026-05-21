package match_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	domainmatch "github.com/averak/vfx/internal/domain/match"
	"github.com/averak/vfx/internal/infra/allocator"
	"github.com/averak/vfx/internal/infra/matchqueue"
	"github.com/averak/vfx/internal/infra/token"
	"github.com/averak/vfx/internal/stdx/clock"
	usecasematch "github.com/averak/vfx/internal/usecase/match"
)

const gameMode = "rps"

func TestMatchmaker_PairsTwoTicketsInSameMode(t *testing.T) {
	queue := matchqueue.NewInMem()
	uc := usecasematch.New(queue)
	mm := usecasematch.NewMatchmaker(queue, allocator.NewStub("room:7777"), token.NewSigner("secret"),
		usecasematch.Config{
			Interval:        10 * time.Millisecond,
			SessionTokenTTL: time.Minute,
			PlayersPerMatch: 2,
			GameModes:       []string{gameMode},
		})

	ctx, cancel := context.WithCancel(clock.With(context.Background(), time.Now()))
	defer cancel()

	// Two players queue tickets and watch them.
	ticketA, err := uc.CreateTicket(ctx, &usecasematch.TicketInput{PlayerID: uuid.New(), GameMode: gameMode})
	if err != nil {
		t.Fatalf("CreateTicket A: %v", err)
	}
	ticketB, err := uc.CreateTicket(ctx, &usecasematch.TicketInput{PlayerID: uuid.New(), GameMode: gameMode})
	if err != nil {
		t.Fatalf("CreateTicket B: %v", err)
	}

	watchA, err := uc.WatchTicket(ctx, ticketA)
	if err != nil {
		t.Fatalf("WatchTicket A: %v", err)
	}
	watchB, err := uc.WatchTicket(ctx, ticketB)
	if err != nil {
		t.Fatalf("WatchTicket B: %v", err)
	}

	go func() { _ = mm.Run(ctx) }()

	assignA := awaitMatched(t, watchA)
	assignB := awaitMatched(t, watchB)

	if assignA.Endpoint != "room:7777" || assignB.Endpoint != "room:7777" {
		t.Errorf("endpoints = %q / %q, want room:7777", assignA.Endpoint, assignB.Endpoint)
	}
	if assignA.MatchID != assignB.MatchID {
		t.Errorf("players landed in different matches: %s vs %s", assignA.MatchID, assignB.MatchID)
	}
	if assignA.SessionToken == "" || assignB.SessionToken == "" {
		t.Error("a paired ticket has an empty session token")
	}
}

func TestMatchmaker_LeavesLoneTicketQueued(t *testing.T) {
	queue := matchqueue.NewInMem()
	uc := usecasematch.New(queue)
	mm := usecasematch.NewMatchmaker(queue, allocator.NewStub("room:7777"), token.NewSigner("secret"),
		usecasematch.Config{
			Interval:        10 * time.Millisecond,
			SessionTokenTTL: time.Minute,
			PlayersPerMatch: 2,
			GameModes:       []string{gameMode},
		})

	ctx, cancel := context.WithCancel(clock.With(context.Background(), time.Now()))
	defer cancel()

	ticketID, err := uc.CreateTicket(ctx, &usecasematch.TicketInput{PlayerID: uuid.New(), GameMode: gameMode})
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	watch, err := uc.WatchTicket(ctx, ticketID)
	if err != nil {
		t.Fatalf("WatchTicket: %v", err)
	}

	go func() { _ = mm.Run(ctx) }()

	// The first event is Queued; with no partner, no Matched should
	// follow within a few matchmaker ticks.
	<-watch
	select {
	case ev := <-watch:
		if _, ok := ev.(domainmatch.EventMatched); ok {
			t.Fatal("a lone ticket was matched")
		}
	case <-time.After(100 * time.Millisecond):
		// Expected: still waiting.
	}
}

func awaitMatched(t *testing.T, ch <-chan domainmatch.Event) *domainmatch.Assignment {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatal("watch channel closed before a match")
			}
			if matched, ok := ev.(domainmatch.EventMatched); ok {
				return matched.Assignment
			}
		case <-timeout:
			t.Fatal("timed out waiting for a match")
		}
	}
}
