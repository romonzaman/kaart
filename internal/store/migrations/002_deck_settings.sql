-- +goose Up
-- Per-deck scheduling settings. Kept on the deck rather than in a global config
-- file: a user learning Estonian vocabulary and reviewing medical terminology
-- wants different daily loads for each.
ALTER TABLE decks ADD COLUMN new_cards_per_day   INTEGER NOT NULL DEFAULT 20;
ALTER TABLE decks ADD COLUMN max_reviews_per_day INTEGER NOT NULL DEFAULT 200;
ALTER TABLE decks ADD COLUMN desired_retention   REAL    NOT NULL DEFAULT 0.9;
-- fsrs_weights is a JSON array of 21 floats, or NULL to use the library defaults.
ALTER TABLE decks ADD COLUMN fsrs_weights TEXT;

-- +goose Down
ALTER TABLE decks DROP COLUMN fsrs_weights;
ALTER TABLE decks DROP COLUMN desired_retention;
ALTER TABLE decks DROP COLUMN max_reviews_per_day;
ALTER TABLE decks DROP COLUMN new_cards_per_day;
