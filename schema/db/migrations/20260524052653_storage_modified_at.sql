-- Rename updated_at to modified_at: it records when a file's bytes were last committed (domain data the client diff-syncs on), distinct from the audit created_at.
-- Renames preserve the existing values.
ALTER TABLE "player_files" RENAME COLUMN "updated_at" TO "modified_at";
ALTER TABLE "title_files" RENAME COLUMN "updated_at" TO "modified_at";
