-- Create "leaderboard_entries" table
CREATE TABLE "leaderboard_entries" (
  "id" uuid NOT NULL,
  "leaderboard_id" text NOT NULL,
  "player_id" uuid NOT NULL,
  "score" bigint NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "leaderboard_entries_leaderboard_id_player_id_key" UNIQUE ("leaderboard_id", "player_id"),
  CONSTRAINT "leaderboard_entries_player_id_fkey" FOREIGN KEY ("player_id") REFERENCES "players" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_leaderboard_entries_ranking" to table: "leaderboard_entries"
CREATE INDEX "idx_leaderboard_entries_ranking" ON "leaderboard_entries" ("leaderboard_id", "score", "updated_at");
