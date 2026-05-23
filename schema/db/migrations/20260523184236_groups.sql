-- Create "groups" table
CREATE TABLE "groups" (
  "id" uuid NOT NULL,
  "name" text NOT NULL,
  "owner_id" uuid NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "groups_owner_id_fkey" FOREIGN KEY ("owner_id") REFERENCES "players" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_groups_owner" to table: "groups"
CREATE INDEX "idx_groups_owner" ON "groups" ("owner_id");
-- Create "group_members" table
CREATE TABLE "group_members" (
  "id" uuid NOT NULL,
  "group_id" uuid NOT NULL,
  "player_id" uuid NOT NULL,
  "joined_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "group_members_group_id_player_id_key" UNIQUE ("group_id", "player_id"),
  CONSTRAINT "group_members_group_id_fkey" FOREIGN KEY ("group_id") REFERENCES "groups" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "group_members_player_id_fkey" FOREIGN KEY ("player_id") REFERENCES "players" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_group_members_player" to table: "group_members"
CREATE INDEX "idx_group_members_player" ON "group_members" ("player_id");
