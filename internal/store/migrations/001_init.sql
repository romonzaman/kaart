-- +goose Up
CREATE TABLE decks (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at  TIMESTAMP NOT NULL,
  updated_at  TIMESTAMP NOT NULL,
  archived_at TIMESTAMP
);

CREATE TABLE cards (
  id           TEXT PRIMARY KEY,
  deck_id      TEXT NOT NULL REFERENCES decks(id) ON DELETE CASCADE,
  front        TEXT NOT NULL,
  back         TEXT NOT NULL,
  hint         TEXT NOT NULL DEFAULT '',
  tags         TEXT NOT NULL DEFAULT '[]',   -- JSON array
  created_at   TIMESTAMP NOT NULL,
  updated_at   TIMESTAMP NOT NULL,
  suspended_at TIMESTAMP
);
CREATE INDEX idx_cards_deck ON cards(deck_id);

CREATE TABLE card_states (
  card_id        TEXT PRIMARY KEY REFERENCES cards(id) ON DELETE CASCADE,
  due            TIMESTAMP NOT NULL,
  stability      REAL NOT NULL DEFAULT 0,
  difficulty     REAL NOT NULL DEFAULT 0,
  elapsed_days   INTEGER NOT NULL DEFAULT 0,
  scheduled_days INTEGER NOT NULL DEFAULT 0,
  reps           INTEGER NOT NULL DEFAULT 0,
  lapses         INTEGER NOT NULL DEFAULT 0,
  state          INTEGER NOT NULL DEFAULT 0,
  last_review    TIMESTAMP
);
CREATE INDEX idx_card_states_due ON card_states(due);

CREATE TABLE reviews (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  card_id           TEXT NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
  rating            INTEGER NOT NULL,
  state             INTEGER NOT NULL,
  due               TIMESTAMP NOT NULL,
  stability         REAL NOT NULL,
  difficulty        REAL NOT NULL,
  elapsed_days      INTEGER NOT NULL,
  last_elapsed_days INTEGER NOT NULL,
  scheduled_days    INTEGER NOT NULL,
  reviewed_at       TIMESTAMP NOT NULL,
  duration_ms       INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_reviews_card ON reviews(card_id);
CREATE INDEX idx_reviews_time ON reviews(reviewed_at);

-- +goose Down
DROP TABLE reviews;
DROP TABLE card_states;
DROP TABLE cards;
DROP TABLE decks;
