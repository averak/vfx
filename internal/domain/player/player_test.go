package player_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/averak/vfx/internal/domain/player"
)

func ptr(s string) *string { return &s }

// Equivalence partitions for the nickname invariant: nil (unnamed), valid,
// blank, and over-length. Boundary cases at exactly the length limit live
// in TestNew_NicknameLengthBoundary.
func TestNew_NicknameInvariant(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		nickname *string
		wantErr  bool
	}{
		{"nil is allowed", nil, false},
		{"ordinary name", ptr("Alice"), false},
		{"empty string rejected", ptr(""), true},
		{"whitespace-only rejected", ptr("   "), true},
		{"over-length rejected", ptr(strings.Repeat("a", player.MaxNicknameLength+1)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := player.New(uuid.New(), tt.nickname, now)
			if tt.wantErr {
				if !errors.Is(err, player.ErrInvalidNickname) {
					t.Fatalf("err = %v, want ErrInvalidNickname", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.RegisteredAt != now {
				t.Errorf("RegisteredAt = %v, want %v", p.RegisteredAt, now)
			}
		})
	}
}

// Boundary-value analysis on MaxNicknameLength: exactly the limit passes,
// one rune over fails. The count is in runes, so a multibyte name of the
// same rune length must also pass (proving it is not a byte count).
func TestNew_NicknameLengthBoundary(t *testing.T) {
	now := time.Now()

	atLimit := strings.Repeat("a", player.MaxNicknameLength)
	if _, err := player.New(uuid.New(), &atLimit, now); err != nil {
		t.Errorf("nickname of exactly MaxNicknameLength runes rejected: %v", err)
	}

	overLimit := strings.Repeat("a", player.MaxNicknameLength+1)
	if _, err := player.New(uuid.New(), &overLimit, now); !errors.Is(err, player.ErrInvalidNickname) {
		t.Errorf("nickname one rune over the limit accepted")
	}

	multibyteAtLimit := strings.Repeat("あ", player.MaxNicknameLength) // 32 runes, 96 bytes
	if _, err := player.New(uuid.New(), &multibyteAtLimit, now); err != nil {
		t.Errorf("multibyte nickname of MaxNicknameLength runes rejected (byte-counting?): %v", err)
	}
}

func TestSetNickname_UpdatesAndValidates(t *testing.T) {
	p, err := player.New(uuid.New(), ptr("Alice"), time.Now())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := p.SetNickname(ptr("Bob")); err != nil {
		t.Fatalf("SetNickname: %v", err)
	}
	if p.Nickname == nil || *p.Nickname != "Bob" {
		t.Errorf("nickname not updated: %v", p.Nickname)
	}

	// A nil nickname clears the field rather than being rejected.
	if err := p.SetNickname(nil); err != nil {
		t.Fatalf("clearing nickname with nil failed: %v", err)
	}
	if p.Nickname != nil {
		t.Errorf("nil nickname did not clear the field: %v", p.Nickname)
	}

	// An invalid nickname is rejected and leaves the player untouched.
	bad := strings.Repeat("x", player.MaxNicknameLength+1)
	if err := p.SetNickname(&bad); !errors.Is(err, player.ErrInvalidNickname) {
		t.Errorf("over-length SetNickname err = %v, want ErrInvalidNickname", err)
	}
	if p.Nickname != nil {
		t.Errorf("rejected SetNickname mutated the player")
	}
}

// IsActive is false at exactly ExpiresAt: the implementation uses
// After(now), so the expiry instant itself is already inactive.
func TestRefreshToken_IsActiveExpiryBoundary(t *testing.T) {
	now := time.Now()
	rt := &player.RefreshToken{ExpiresAt: now}
	if rt.IsActive(now) {
		t.Error("token active at exactly its expiry instant")
	}
	rt.ExpiresAt = now.Add(time.Nanosecond)
	if !rt.IsActive(now) {
		t.Error("token inactive one nanosecond before expiry")
	}
}

func TestRefreshToken_RevokedIsInactive(t *testing.T) {
	now := time.Now()
	revoked := now.Add(-time.Minute)
	rt := &player.RefreshToken{
		ExpiresAt: now.Add(time.Hour), // not expired
		RevokedAt: &revoked,
	}
	if rt.IsActive(now) {
		t.Error("revoked token reported active despite a future expiry")
	}
}
