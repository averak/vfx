-- vfx PostgreSQL schema (declarative source of truth).
--
-- This file describes the desired state of the database. Migrations are
-- generated from the diff between this file and the applied migration
-- history using `atlas migrate diff`. Edit schema.sql, then run the
-- mise task `db-diff <name>` to produce a new migration.
--
-- Conventions:
--   - All primary keys are UUID, supplied by the application (never SERIAL).
--   - All TIMESTAMPTZ columns store wall-clock time in UTC.
--   - Soft-delete columns are avoided; cascading deletes handle cleanup.
--   - Indexes are named idx_<table>_<columns>.

-- ============================================================================
-- players
--   Core player record. One per game profile. A player can carry multiple
--   identities across providers (anonymous → linked Google, etc.) by the
--   player_identities table below.
-- ============================================================================

CREATE TABLE players (
  id          UUID         PRIMARY KEY,
  nickname    VARCHAR(32),
  created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_players_nickname
  ON players (nickname)
  WHERE nickname IS NOT NULL;

-- ============================================================================
-- player_identities
--   Bridges a player to one or more authentication providers.
--   - provider = "anonymous", provider_uid = device_id (or random for ephemeral)
--   - provider = "google" | "apple" | "github" | ..., provider_uid = OAuth sub
-- ============================================================================

CREATE TABLE player_identities (
  id            UUID         PRIMARY KEY,
  player_id     UUID         NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  provider      TEXT         NOT NULL,
  provider_uid  TEXT         NOT NULL,
  created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  UNIQUE (provider, provider_uid)
);

CREATE INDEX idx_player_identities_player_id
  ON player_identities (player_id);

-- ============================================================================
-- refresh_tokens
--   Long-lived tokens used to mint new access tokens. Raw token strings
--   are never stored; token_hash holds SHA-256 of the token bytes so a
--   database leak cannot be used directly to impersonate players.
-- ============================================================================

CREATE TABLE refresh_tokens (
  id          UUID         PRIMARY KEY,
  player_id   UUID         NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  token_hash  BYTEA        NOT NULL UNIQUE,
  expires_at  TIMESTAMPTZ  NOT NULL,
  revoked_at  TIMESTAMPTZ,
  created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_refresh_tokens_player_id
  ON refresh_tokens (player_id);

CREATE INDEX idx_refresh_tokens_expires_at
  ON refresh_tokens (expires_at);

-- ============================================================================
-- matches
--   Persisted record of every match. final_state holds opaque bytes that
--   the plugin returned from OnGameEnd; format is plugin-specific.
-- ============================================================================

CREATE TABLE matches (
  id            UUID         PRIMARY KEY,
  game_mode     TEXT         NOT NULL,
  status        TEXT         NOT NULL,
  started_at    TIMESTAMPTZ,
  ended_at      TIMESTAMPTZ,
  final_state   BYTEA,
  created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  CONSTRAINT matches_status_valid
    CHECK (status IN ('pending', 'in_progress', 'finished', 'cancelled'))
);

CREATE INDEX idx_matches_status
  ON matches (status);

CREATE INDEX idx_matches_created_at
  ON matches (created_at DESC);

-- ============================================================================
-- match_players
--   Many-to-many between matches and players, with the per-player result
--   (rank, optional plugin-defined stats) captured at game end.
-- ============================================================================

CREATE TABLE match_players (
  id          UUID         PRIMARY KEY,
  match_id    UUID         NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
  player_id   UUID         NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  rank        INTEGER,
  stats       JSONB,
  created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  UNIQUE (match_id, player_id)
);

CREATE INDEX idx_match_players_player_id
  ON match_players (player_id);

CREATE INDEX idx_match_players_match_id
  ON match_players (match_id);
