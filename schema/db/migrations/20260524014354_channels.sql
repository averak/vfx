-- Create "channel_messages" table
CREATE TABLE "channel_messages" (
  "id" uuid NOT NULL,
  "channel_id" uuid NOT NULL,
  "sender_id" uuid NOT NULL,
  "body" text NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "channel_messages_channel_id_fkey" FOREIGN KEY ("channel_id") REFERENCES "groups" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "channel_messages_sender_id_fkey" FOREIGN KEY ("sender_id") REFERENCES "players" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_channel_messages_history" to table: "channel_messages"
CREATE INDEX "idx_channel_messages_history" ON "channel_messages" ("channel_id", "created_at" DESC);
