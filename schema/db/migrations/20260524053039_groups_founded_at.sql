-- Rename created_at to founded_at: it records when the group was created (domain data the API surfaces), not a generic row audit. A rename preserves the values.
ALTER TABLE "groups" RENAME COLUMN "created_at" TO "founded_at";
