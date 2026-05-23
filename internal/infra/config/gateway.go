// Package config loads process configuration from the environment.
//
// Each subcommand has its own config type, so running `vfx gateway` never trips a validation rule that only `vfx room` cares about.
// The types are plain structs with env tags; loading is a single function per subcommand that returns a fully validated value or an error.
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// Gateway holds every value the gateway process needs to start.
//
// Common variables (DATABASE_URL, VALKEY_URL) follow the unprefixed convention shared with atlas, psql, redis-cli, and the like, so the same env file works for tooling.
// vfx-specific knobs carry the VFX_ prefix to avoid collisions in shared environments.
type Gateway struct {
	ListenAddr string `env:"VFX_GATEWAY_LISTEN_ADDR" envDefault:":8080"`

	DatabaseURL string `env:"DATABASE_URL,required,notEmpty"`
	ValkeyURL   string `env:"VALKEY_URL,notEmpty"     envDefault:"redis://localhost:6379"`

	// JWTSecret is the HMAC secret used to sign access tokens.
	// The room daemon must be given the same value so it can verify session tokens the gateway hands out.
	JWTSecret       string        `env:"VFX_JWT_SECRET,required,notEmpty"`
	AccessTokenTTL  time.Duration `env:"VFX_ACCESS_TOKEN_TTL"  envDefault:"15m"`
	RefreshTokenTTL time.Duration `env:"VFX_REFRESH_TOKEN_TTL" envDefault:"720h"`
	SessionTokenTTL time.Duration `env:"VFX_SESSION_TOKEN_TTL" envDefault:"5m"`

	// MatchmakerInterval is how often the worker scans the queue.
	MatchmakerInterval time.Duration `env:"VFX_MATCHMAKER_INTERVAL" envDefault:"200ms"`

	// MatchmakerLeaderTTL is the Valkey lease TTL for matchmaker leader election: only the replica holding the lease runs the matchmaker loop, so replicas don't all scan the shared queue.
	// On leader death the lease expires within this window and another replica takes over.
	MatchmakerLeaderTTL time.Duration `env:"VFX_MATCHMAKER_LEADER_TTL" envDefault:"15s"`

	// GameModes lists the modes the matchmaker scans; PlayersPerMatch is how many tickets form one match.
	// These default to the rps sample but are config-driven, so the engine is not game-specific.
	GameModes       []string `env:"VFX_GAME_MODES"        envDefault:"rps" envSeparator:","`
	PlayersPerMatch int      `env:"VFX_PLAYERS_PER_MATCH" envDefault:"2"`

	// Tier-based matching knobs (see usecase/match.Config).
	// Zero values fall back to the matchmaker's built-in defaults.
	MatchBaseRatingWindow         float64       `env:"VFX_MATCH_BASE_RATING_WINDOW"`
	MatchRatingWindowGrowthPerSec float64       `env:"VFX_MATCH_RATING_WINDOW_GROWTH_PER_SEC"`
	MatchRegionRelaxAfter         time.Duration `env:"VFX_MATCH_REGION_RELAX_AFTER"`

	// RoomEndpoint is the address handed to clients by the stub allocator; every match points at the same address.
	// The Agones allocator ignores it in favour of the allocated GameServer's address.
	RoomEndpoint string `env:"VFX_ROOM_ENDPOINT" envDefault:"localhost:7777"`

	// MatchQueue selects the matchmaking queue backend: "inmem" (the default, single-process) or "valkey" (shared across gateway replicas, required for horizontal scaling).
	MatchQueue string `env:"VFX_MATCH_QUEUE" envDefault:"inmem"`

	// Allocator selects how rooms are reserved: "stub" (the default, single fixed endpoint for local/compose runs) or "agones" (creates a GameServerAllocation per match against the in-cluster API).
	Allocator string `env:"VFX_ALLOCATOR" envDefault:"stub"`

	// AgonesNamespace is the namespace the Agones allocator creates GameServerAllocations in.
	// Only used when Allocator is "agones".
	AgonesNamespace string `env:"VFX_AGONES_NAMESPACE" envDefault:"default"`
}

// LoadGateway reads the gateway configuration from the environment.
func LoadGateway() (*Gateway, error) {
	var cfg Gateway
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return &cfg, nil
}
