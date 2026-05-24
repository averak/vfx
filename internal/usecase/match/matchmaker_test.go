package match_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	domainmatch "github.com/averak/vfx/internal/domain/match"
	"github.com/averak/vfx/internal/infra/allocator"
	"github.com/averak/vfx/internal/infra/assignmentstore"
	"github.com/averak/vfx/internal/infra/matchqueue"
	"github.com/averak/vfx/internal/infra/token"
	"github.com/averak/vfx/internal/stdx/clock"
	usecasematch "github.com/averak/vfx/internal/usecase/match"
)

const gameMode = "rps"

func TestMatchmaker_PairsTwoTicketsInSameMode(t *testing.T) {
	queue := matchqueue.NewInMem()
	store := assignmentstore.NewInMem()
	uc := usecasematch.New(queue, store, 0)
	mm := usecasematch.NewMatchmaker(queue, allocator.NewStub("room:7777"), token.NewSigner("secret"),
		usecasematch.Config{
			Interval:        10 * time.Millisecond,
			SessionTokenTTL: time.Minute,
			PlayersPerMatch: 2,
			GameModes:       []string{gameMode},
			Assignments:     store,
		})

	ctx, cancel := context.WithCancel(clock.With(t.Context(), time.Now()))
	defer cancel()

	// Two players queue tickets and watch them.
	playerA, playerB := uuid.New(), uuid.New()
	ticketA, err := uc.CreateTicket(ctx, &usecasematch.TicketInput{PlayerID: playerA, GameMode: gameMode})
	if err != nil {
		t.Fatalf("CreateTicket A: %v", err)
	}
	ticketB, err := uc.CreateTicket(ctx, &usecasematch.TicketInput{PlayerID: playerB, GameMode: gameMode})
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

	// The matchmaker should have persisted each assignment so a
	// reconnecting client can recover it via GetCurrentMatch.
	currentA, err := uc.GetCurrentMatch(ctx, playerA)
	if err != nil {
		t.Fatalf("GetCurrentMatch A: %v", err)
	}
	if currentA == nil {
		t.Fatal("GetCurrentMatch A returned no assignment after a match")
	}
	if currentA.MatchID != assignA.MatchID || currentA.SessionToken != assignA.SessionToken {
		t.Errorf("stored assignment for A = %+v, want match %s with the streamed token",
			currentA, assignA.MatchID)
	}
	if currentB, err := uc.GetCurrentMatch(ctx, playerB); err != nil || currentB == nil {
		t.Fatalf("GetCurrentMatch B = (%+v, %v), want a stored assignment", currentB, err)
	}
}

func TestMatchmaker_LeavesLoneTicketQueued(t *testing.T) {
	queue := matchqueue.NewInMem()
	uc := usecasematch.New(queue, assignmentstore.NewInMem(), 0)
	mm := usecasematch.NewMatchmaker(queue, allocator.NewStub("room:7777"), token.NewSigner("secret"),
		usecasematch.Config{
			Interval:        10 * time.Millisecond,
			SessionTokenTTL: time.Minute,
			PlayersPerMatch: 2,
			GameModes:       []string{gameMode},
		})

	ctx, cancel := context.WithCancel(clock.With(t.Context(), time.Now()))
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

// CreateTicket normalizes the roster to include the submitter and rejects a party that cannot fit a match.
func TestCreateTicket_NormalizesAndValidatesParty(t *testing.T) {
	queue := matchqueue.NewInMem()
	uc := usecasematch.New(queue, assignmentstore.NewInMem(), 4)
	ctx := clock.With(t.Context(), time.Now())

	a, b := uuid.New(), uuid.New()
	if _, err := uc.CreateTicket(ctx, &usecasematch.TicketInput{PlayerID: a, GameMode: gameMode, PartyMembers: []uuid.UUID{b}}); err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	pending, err := queue.Pending(ctx, gameMode)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("want 1 pending ticket, got %d", len(pending))
	}
	// The client listed only b; the submitter a must be folded in, yielding a 2-member party.
	if !pending[0].IsParty() || len(pending[0].PartyMembers) != 2 {
		t.Errorf("roster not normalized to include the submitter: %v", pending[0].PartyMembers)
	}

	// A party of 5 cannot fit a 4-player match, so it is rejected up front.
	oversized := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	if _, err := uc.CreateTicket(ctx, &usecasematch.TicketInput{PlayerID: a, GameMode: gameMode, PartyMembers: oversized}); !errors.Is(err, domainmatch.ErrPartyTooLarge) {
		t.Errorf("oversized party err = %v, want ErrPartyTooLarge", err)
	}
}

// Two players who queue as a party land in the same match.
func TestMatchmaker_KeepsPartyInSameMatch(t *testing.T) {
	queue := matchqueue.NewInMem()
	store := assignmentstore.NewInMem()
	uc := usecasematch.New(queue, store, 0)
	mm := usecasematch.NewMatchmaker(queue, allocator.NewStub("room:7777"), token.NewSigner("secret"),
		usecasematch.Config{
			Interval:        10 * time.Millisecond,
			SessionTokenTTL: time.Minute,
			PlayersPerMatch: 2,
			GameModes:       []string{gameMode},
			Assignments:     store,
		})

	ctx, cancel := context.WithCancel(clock.With(t.Context(), time.Now()))
	defer cancel()

	a, b := uuid.New(), uuid.New()
	ticketA, err := uc.CreateTicket(ctx, &usecasematch.TicketInput{PlayerID: a, GameMode: gameMode, PartyMembers: []uuid.UUID{b}})
	if err != nil {
		t.Fatalf("CreateTicket A: %v", err)
	}
	ticketB, err := uc.CreateTicket(ctx, &usecasematch.TicketInput{PlayerID: b, GameMode: gameMode, PartyMembers: []uuid.UUID{a}})
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
	if assignA.MatchID != assignB.MatchID {
		t.Errorf("party split across matches: %s vs %s", assignA.MatchID, assignB.MatchID)
	}
}

// A party is not matched until every member has queued, even when the match could otherwise fill.
func TestMatchmaker_WaitsForIncompleteParty(t *testing.T) {
	queue := matchqueue.NewInMem()
	uc := usecasematch.New(queue, assignmentstore.NewInMem(), 0)
	mm := usecasematch.NewMatchmaker(queue, allocator.NewStub("room:7777"), token.NewSigner("secret"),
		usecasematch.Config{
			Interval:        10 * time.Millisecond,
			SessionTokenTTL: time.Minute,
			PlayersPerMatch: 2,
			GameModes:       []string{gameMode},
		})

	ctx, cancel := context.WithCancel(clock.With(t.Context(), time.Now()))
	defer cancel()

	a, b := uuid.New(), uuid.New()
	// Only a queues for the party; b has not. A lone solo also waits — but it must not be paired with the party member.
	partyTicket, err := uc.CreateTicket(ctx, &usecasematch.TicketInput{PlayerID: a, GameMode: gameMode, PartyMembers: []uuid.UUID{b}})
	if err != nil {
		t.Fatalf("CreateTicket party: %v", err)
	}
	if _, soloErr := uc.CreateTicket(ctx, &usecasematch.TicketInput{PlayerID: uuid.New(), GameMode: gameMode}); soloErr != nil {
		t.Fatalf("CreateTicket solo: %v", soloErr)
	}
	watch, err := uc.WatchTicket(ctx, partyTicket)
	if err != nil {
		t.Fatalf("WatchTicket: %v", err)
	}

	go func() { _ = mm.Run(ctx) }()

	<-watch // Queued
	select {
	case ev := <-watch:
		if _, ok := ev.(domainmatch.EventMatched); ok {
			t.Fatal("an incomplete party was matched")
		}
	case <-time.After(100 * time.Millisecond):
		// Expected: still waiting for b.
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
