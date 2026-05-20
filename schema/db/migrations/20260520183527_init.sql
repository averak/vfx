-- Create "matches" table
CREATE TABLE "matches" (
  "id" uuid NOT NULL,
  "game_mode" text NOT NULL,
  "status" text NOT NULL,
  "started_at" timestamptz NULL,
  "ended_at" timestamptz NULL,
  "final_state" bytea NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "matches_status_valid" CHECK (status = ANY (ARRAY['pending'::text, 'in_progress'::text, 'finished'::text, 'cancelled'::text]))
);
-- Create index "idx_matches_created_at" to table: "matches"
CREATE INDEX "idx_matches_created_at" ON "matches" ("created_at" DESC);
-- Create index "idx_matches_status" to table: "matches"
CREATE INDEX "idx_matches_status" ON "matches" ("status");
-- Create "players" table
CREATE TABLE "players" (
  "id" uuid NOT NULL,
  "nickname" character varying(32) NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id")
);
-- Create index "idx_players_nickname" to table: "players"
CREATE INDEX "idx_players_nickname" ON "players" ("nickname") WHERE (nickname IS NOT NULL);
-- Create "match_players" table
CREATE TABLE "match_players" (
  "id" uuid NOT NULL,
  "match_id" uuid NOT NULL,
  "player_id" uuid NOT NULL,
  "rank" integer NULL,
  "stats" jsonb NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "match_players_match_id_player_id_key" UNIQUE ("match_id", "player_id"),
  CONSTRAINT "match_players_match_id_fkey" FOREIGN KEY ("match_id") REFERENCES "matches" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "match_players_player_id_fkey" FOREIGN KEY ("player_id") REFERENCES "players" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_match_players_match_id" to table: "match_players"
CREATE INDEX "idx_match_players_match_id" ON "match_players" ("match_id");
-- Create index "idx_match_players_player_id" to table: "match_players"
CREATE INDEX "idx_match_players_player_id" ON "match_players" ("player_id");
-- Create "player_identities" table
CREATE TABLE "player_identities" (
  "id" uuid NOT NULL,
  "player_id" uuid NOT NULL,
  "provider" text NOT NULL,
  "provider_uid" text NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "player_identities_provider_provider_uid_key" UNIQUE ("provider", "provider_uid"),
  CONSTRAINT "player_identities_player_id_fkey" FOREIGN KEY ("player_id") REFERENCES "players" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_player_identities_player_id" to table: "player_identities"
CREATE INDEX "idx_player_identities_player_id" ON "player_identities" ("player_id");
-- Create "refresh_tokens" table
CREATE TABLE "refresh_tokens" (
  "id" uuid NOT NULL,
  "player_id" uuid NOT NULL,
  "token_hash" bytea NOT NULL,
  "expires_at" timestamptz NOT NULL,
  "revoked_at" timestamptz NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "refresh_tokens_token_hash_key" UNIQUE ("token_hash"),
  CONSTRAINT "refresh_tokens_player_id_fkey" FOREIGN KEY ("player_id") REFERENCES "players" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_refresh_tokens_expires_at" to table: "refresh_tokens"
CREATE INDEX "idx_refresh_tokens_expires_at" ON "refresh_tokens" ("expires_at");
-- Create index "idx_refresh_tokens_player_id" to table: "refresh_tokens"
CREATE INDEX "idx_refresh_tokens_player_id" ON "refresh_tokens" ("player_id");
