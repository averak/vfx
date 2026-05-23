-- vfx PostgreSQL schema (declarative source of truth).
--
-- This file describes the desired state of the database.
-- Migrations are generated from the diff between this file and the applied migration history using `atlas migrate diff`.
-- Edit schema.sql, then run the mise task `db-diff <name>` to produce a new migration.
--
-- Conventions:
--   - All primary keys are UUID, supplied by the application (never SERIAL).
--   - All TIMESTAMPTZ columns store wall-clock time in UTC.
--   - Soft-delete columns are avoided; cascading deletes handle cleanup.
--   - Indexes are named idx_<table>_<columns>.

-- ============================================================================
-- players
--   Core player record, one per game profile.
--   A player can carry multiple identities across providers (anonymous → linked Google, etc.) via the player_identities table below.
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
--   Long-lived tokens used to mint new access tokens.
--   Raw token strings are never stored; token_hash holds SHA-256 of the token bytes so a database leak cannot be used directly to impersonate players.
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
--   Persisted record of every match.
--   final_state holds the opaque bytes the plugin returned from OnGameEnd; the format is plugin-specific.
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
--   Many-to-many between matches and players, with the per-player result (rank, optional plugin-defined stats) captured at game end.
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

-- ============================================================================
-- player_files
--   Player Data Storage metadata, one row per (owner, filename).
--   The bytes live in object storage under a key derived from owner_id and filename; only metadata is here.
--   A row exists only after a committed upload, so the table never lists a file whose bytes are absent (the reverse can happen: an uncommitted upload is an orphan object with no row).
-- ============================================================================

CREATE TABLE player_files (
  id          UUID         PRIMARY KEY,
  owner_id    UUID         NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  filename    TEXT         NOT NULL,
  size        BIGINT       NOT NULL,
  hash        TEXT         NOT NULL,
  created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  -- The unique constraint's index leads with owner_id, so it also serves owner-scoped list and prefix scans; no separate owner_id index is needed.
  UNIQUE (owner_id, filename)
);

-- ============================================================================
-- title_files
--   Title Storage metadata: operator-written, read-only for clients.
--   tags gate access; clients query by tag. The GIN index serves the "contains all requested tags" filter (tags @> requested).
-- ============================================================================

CREATE TABLE title_files (
  id          UUID         PRIMARY KEY,
  filename    TEXT         NOT NULL UNIQUE,
  size        BIGINT       NOT NULL,
  hash        TEXT         NOT NULL,
  tags        TEXT[]       NOT NULL DEFAULT '{}',
  created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_title_files_tags
  ON title_files USING GIN (tags);

-- ============================================================================
-- leaderboard_entries
--   One row per (leaderboard, player): the player's best score so far.
--   leaderboard_id is an operator-defined key (validated against config, not stored as its own table); sort order lives in config too.
--   Ranking ties break by updated_at (the player who reached the score first ranks higher), which also makes the order total so a counted rank matches a paginated one.
-- ============================================================================

CREATE TABLE leaderboard_entries (
  id             UUID         PRIMARY KEY,
  leaderboard_id TEXT         NOT NULL,
  player_id      UUID         NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  score          BIGINT       NOT NULL,
  created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  UNIQUE (leaderboard_id, player_id)
);

-- Serves both the ranked scans (ORDER BY score, updated_at) and the count-of-better-scores rank query, scoped per leaderboard.
CREATE INDEX idx_leaderboard_entries_ranking
  ON leaderboard_entries (leaderboard_id, score, updated_at);

-- ============================================================================
-- friend_requests
--   Directed, pending-only: a row exists only while requester -> addressee is outstanding.
--   Accepting deletes the row and inserts a friendship; declining/cancelling just deletes it.
-- ============================================================================

CREATE TABLE friend_requests (
  id            UUID         PRIMARY KEY,
  requester_id  UUID         NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  addressee_id  UUID         NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  UNIQUE (requester_id, addressee_id)
);

CREATE INDEX idx_friend_requests_addressee
  ON friend_requests (addressee_id);

-- ============================================================================
-- friendships
--   Undirected, stored once with player_low < player_high so a pair has a single row regardless of who initiated.
-- ============================================================================

CREATE TABLE friendships (
  id           UUID         PRIMARY KEY,
  player_low   UUID         NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  player_high  UUID         NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  UNIQUE (player_low, player_high),
  CONSTRAINT friendships_canonical_order CHECK (player_low < player_high)
);

-- The unique constraint indexes player_low; this covers lookups landing on the high side.
CREATE INDEX idx_friendships_player_high
  ON friendships (player_high);

-- ============================================================================
-- direct_messages
--   One row per DM. (player_low, player_high) is the canonical conversation key so a pair maps to one conversation; sender_id is whichever participant sent it.
-- ============================================================================

CREATE TABLE direct_messages (
  id           UUID         PRIMARY KEY,
  player_low   UUID         NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  player_high  UUID         NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  sender_id    UUID         NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  body         TEXT         NOT NULL,
  created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  CONSTRAINT direct_messages_canonical_order CHECK (player_low < player_high),
  CONSTRAINT direct_messages_sender_in_pair CHECK (sender_id = player_low OR sender_id = player_high)
);

-- Serves the newest-first, before-cursor history scan scoped to a conversation.
CREATE INDEX idx_direct_messages_conversation
  ON direct_messages (player_low, player_high, created_at DESC);

-- ============================================================================
-- player_blocks
--   Directed: blocker_id has blocked blocked_id. A friend request is refused when a block exists in either direction.
-- ============================================================================

CREATE TABLE player_blocks (
  id          UUID         PRIMARY KEY,
  blocker_id  UUID         NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  blocked_id  UUID         NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  UNIQUE (blocker_id, blocked_id)
);

-- Covers the reverse-direction check (has the other player blocked me?).
CREATE INDEX idx_player_blocks_blocked
  ON player_blocks (blocked_id);

-- ============================================================================
-- groups / group_members
--   A player-owned group (clan/guild) and its membership. Deleting a group cascades its memberships.
-- ============================================================================

CREATE TABLE groups (
  id          UUID         PRIMARY KEY,
  name        TEXT         NOT NULL,
  owner_id    UUID         NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_groups_owner
  ON groups (owner_id);

CREATE TABLE group_members (
  id         UUID         PRIMARY KEY,
  group_id   UUID         NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  player_id  UUID         NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  joined_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  UNIQUE (group_id, player_id)
);

CREATE INDEX idx_group_members_player
  ON group_members (player_id);
