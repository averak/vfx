-- Rename created_at to registered_at: it records when the account was created (domain data the API surfaces), distinct from the audit updated_at. A rename preserves the values.
ALTER TABLE "players" RENAME COLUMN "created_at" TO "registered_at";
