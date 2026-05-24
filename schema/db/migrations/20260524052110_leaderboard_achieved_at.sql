-- Rename updated_at to achieved_at: the column records when a player's current best was reached (the tie-break key), which is domain data, not a row-audit timestamp.
-- A rename preserves the existing values, and Postgres carries the ranking index over to the renamed column automatically.
ALTER TABLE "leaderboard_entries" RENAME COLUMN "updated_at" TO "achieved_at";
