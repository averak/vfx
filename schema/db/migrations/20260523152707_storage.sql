-- Create "title_files" table
CREATE TABLE "title_files" (
  "id" uuid NOT NULL,
  "filename" text NOT NULL,
  "size" bigint NOT NULL,
  "hash" text NOT NULL,
  "tags" text[] NOT NULL DEFAULT '{}',
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "title_files_filename_key" UNIQUE ("filename")
);
-- Create index "idx_title_files_tags" to table: "title_files"
CREATE INDEX "idx_title_files_tags" ON "title_files" USING GIN ("tags");
-- Create "player_files" table
CREATE TABLE "player_files" (
  "id" uuid NOT NULL,
  "owner_id" uuid NOT NULL,
  "filename" text NOT NULL,
  "size" bigint NOT NULL,
  "hash" text NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY ("id"),
  CONSTRAINT "player_files_owner_id_filename_key" UNIQUE ("owner_id", "filename"),
  CONSTRAINT "player_files_owner_id_fkey" FOREIGN KEY ("owner_id") REFERENCES "players" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
