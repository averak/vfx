-- Create "player_blocks" table
CREATE TABLE "player_blocks" (
  "id" uuid NOT NULL,
  "blocker_id" uuid NOT NULL,
  "blocked_id" uuid NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "player_blocks_blocker_id_blocked_id_key" UNIQUE ("blocker_id", "blocked_id"),
  CONSTRAINT "player_blocks_blocked_id_fkey" FOREIGN KEY ("blocked_id") REFERENCES "players" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "player_blocks_blocker_id_fkey" FOREIGN KEY ("blocker_id") REFERENCES "players" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_player_blocks_blocked" to table: "player_blocks"
CREATE INDEX "idx_player_blocks_blocked" ON "player_blocks" ("blocked_id");
