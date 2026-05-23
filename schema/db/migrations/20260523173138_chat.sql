-- Create "direct_messages" table
CREATE TABLE "direct_messages" (
  "id" uuid NOT NULL,
  "player_low" uuid NOT NULL,
  "player_high" uuid NOT NULL,
  "sender_id" uuid NOT NULL,
  "body" text NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "direct_messages_player_high_fkey" FOREIGN KEY ("player_high") REFERENCES "players" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "direct_messages_player_low_fkey" FOREIGN KEY ("player_low") REFERENCES "players" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "direct_messages_sender_id_fkey" FOREIGN KEY ("sender_id") REFERENCES "players" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "direct_messages_canonical_order" CHECK (player_low < player_high),
  CONSTRAINT "direct_messages_sender_in_pair" CHECK ((sender_id = player_low) OR (sender_id = player_high))
);
-- Create index "idx_direct_messages_conversation" to table: "direct_messages"
CREATE INDEX "idx_direct_messages_conversation" ON "direct_messages" ("player_low", "player_high", "created_at" DESC);
