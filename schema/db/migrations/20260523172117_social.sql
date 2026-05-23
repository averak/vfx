-- Create "friend_requests" table
CREATE TABLE "friend_requests" (
  "id" uuid NOT NULL,
  "requester_id" uuid NOT NULL,
  "addressee_id" uuid NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "friend_requests_requester_id_addressee_id_key" UNIQUE ("requester_id", "addressee_id"),
  CONSTRAINT "friend_requests_addressee_id_fkey" FOREIGN KEY ("addressee_id") REFERENCES "players" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "friend_requests_requester_id_fkey" FOREIGN KEY ("requester_id") REFERENCES "players" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_friend_requests_addressee" to table: "friend_requests"
CREATE INDEX "idx_friend_requests_addressee" ON "friend_requests" ("addressee_id");
-- Create "friendships" table
CREATE TABLE "friendships" (
  "id" uuid NOT NULL,
  "player_low" uuid NOT NULL,
  "player_high" uuid NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "friendships_player_low_player_high_key" UNIQUE ("player_low", "player_high"),
  CONSTRAINT "friendships_player_high_fkey" FOREIGN KEY ("player_high") REFERENCES "players" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "friendships_player_low_fkey" FOREIGN KEY ("player_low") REFERENCES "players" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "friendships_canonical_order" CHECK (player_low < player_high)
);
-- Create index "idx_friendships_player_high" to table: "friendships"
CREATE INDEX "idx_friendships_player_high" ON "friendships" ("player_high");
