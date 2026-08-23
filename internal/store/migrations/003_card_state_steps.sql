-- +goose Up
-- go-fsrs/v4 tracks a card's position in the learning/relearning step ladder on
-- the card itself (Card.RemainingSteps). Without persisting it, every review of
-- a learning card restarts the ladder and short-term steps never advance, so
-- this column is required for correct scheduling rather than optional metadata.
ALTER TABLE card_states ADD COLUMN remaining_steps INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE card_states DROP COLUMN remaining_steps;
