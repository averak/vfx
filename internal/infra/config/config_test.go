package config_test

import (
	"testing"
	"time"

	"github.com/averak/vfx/internal/infra/config"
)

// requireGatewaySecrets sets the two required gateway variables so a test
// can focus on whatever else it is asserting.
func requireGatewaySecrets(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("VFX_JWT_SECRET", "test-secret")
}

func TestLoadGateway_AppliesDefaults(t *testing.T) {
	requireGatewaySecrets(t)

	cfg, err := config.LoadGateway()
	if err != nil {
		t.Fatalf("LoadGateway: %v", err)
	}
	if cfg.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want :8080", cfg.ListenAddr)
	}
	if cfg.AccessTokenTTL != 15*time.Minute {
		t.Errorf("AccessTokenTTL = %v, want 15m", cfg.AccessTokenTTL)
	}
	if cfg.PlayersPerMatch != 2 {
		t.Errorf("PlayersPerMatch = %d, want 2", cfg.PlayersPerMatch)
	}
	if cfg.MatchQueue != "inmem" {
		t.Errorf("MatchQueue = %q, want inmem", cfg.MatchQueue)
	}
}

func TestLoadGateway_RequiresSecret(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("VFX_JWT_SECRET", "") // notEmpty: an empty secret is rejected

	if _, err := config.LoadGateway(); err == nil {
		t.Error("LoadGateway accepted an empty JWT secret")
	}
}

func TestLoadGateway_ParsesOverrides(t *testing.T) {
	requireGatewaySecrets(t)
	t.Setenv("VFX_GAME_MODES", "rps,pong,quiz")
	t.Setenv("VFX_PLAYERS_PER_MATCH", "4")
	t.Setenv("VFX_MATCHMAKER_INTERVAL", "500ms")

	cfg, err := config.LoadGateway()
	if err != nil {
		t.Fatalf("LoadGateway: %v", err)
	}
	if len(cfg.GameModes) != 3 || cfg.GameModes[0] != "rps" || cfg.GameModes[2] != "quiz" {
		t.Errorf("GameModes = %v, want [rps pong quiz]", cfg.GameModes)
	}
	if cfg.PlayersPerMatch != 4 {
		t.Errorf("PlayersPerMatch = %d, want 4", cfg.PlayersPerMatch)
	}
	if cfg.MatchmakerInterval != 500*time.Millisecond {
		t.Errorf("MatchmakerInterval = %v, want 500ms", cfg.MatchmakerInterval)
	}
}

func TestLoadGateway_RejectsBadDuration(t *testing.T) {
	requireGatewaySecrets(t)
	t.Setenv("VFX_ACCESS_TOKEN_TTL", "not-a-duration")

	if _, err := config.LoadGateway(); err == nil {
		t.Error("LoadGateway accepted an unparseable duration")
	}
}
