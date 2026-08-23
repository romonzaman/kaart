# Database schema

Kaart stores everything in one SQLite file. There is no user table and no
`user_id` column anywhere: v1 is single-user and local-first, and adding those
columns before there is a second user would be inventing structure to serve a
hypothetical.

Migrations live in `internal/store/migrations`, are embedded into the binary
with `embed.FS`, and run automatically when the database is opened. Applied
migrations are tracked by goose in its own `goose_db_version` table.

The connection is opened with three pragmas:

| Pragma | Why |
|---|---|
| `journal_mode(WAL)` | Readers do not block the writer, so a stats query mid-session never stalls a review |
| `busy_timeout(5000)` | A contended write waits five seconds instead of failing instantly |
| `foreign_keys(1)` | SQLite disables foreign keys by default; the cascade deletes below depend on them |

Timestamps are stored as fixed-width RFC3339 strings in UTC
(`2006-01-02T15:04:05.000000000Z`). Fixed width matters: SQLite compares these
as text, so every value needs the same number of fractional digits for
lexicographic order to match chronological order. `time.RFC3339Nano` trims
trailing zeros and would quietly break every `due <= now` comparison.

---

## `decks`

A named collection of cards, plus the scheduling settings that apply to it.

| Column | Type | Notes |
|---|---|---|
| `id` | TEXT PK | UUIDv7, generated server-side |
| `name` | TEXT | 1–200 characters after trimming |
| `description` | TEXT | May be empty, never NULL |
| `new_cards_per_day` | INTEGER | Default 20 |
| `max_reviews_per_day` | INTEGER | Default 200 |
| `desired_retention` | REAL | Default 0.9 — the recall probability the schedule targets |
| `fsrs_weights` | TEXT NULL | JSON array of 21 floats, or NULL for the library defaults |
| `created_at`, `updated_at` | TIMESTAMP | |
| `archived_at` | TIMESTAMP NULL | Non-NULL hides the deck from the default list without deleting it |

Settings live on the deck rather than in a global config file because they are
genuinely per-deck: someone doing twenty new Estonian words a day may want two
new pharmacology cards a day, and one number cannot serve both.

## `cards`

The content. A card is front, back, an optional hint, and tags.

| Column | Type | Notes |
|---|---|---|
| `id` | TEXT PK | UUIDv7 |
| `deck_id` | TEXT | `REFERENCES decks(id) ON DELETE CASCADE` |
| `front`, `back` | TEXT | Non-empty after trimming |
| `hint` | TEXT | May be empty |
| `tags` | TEXT | JSON array; `[]` when there are none, never NULL |
| `created_at`, `updated_at` | TIMESTAMP | |
| `suspended_at` | TIMESTAMP NULL | Non-NULL excludes the card from every queue |

Index: `idx_cards_deck (deck_id)` — every card listing is scoped to a deck.

Tags are a JSON array rather than a join table. A card has at most twenty short
tags, they are never queried across decks in v1, and a `card_tags` table would
be three more queries for no capability anyone has asked for. If tag-based study
filters arrive later, that is the moment to normalise them.

Note that editing a card's content leaves its `card_states` row untouched. Fixing
a typo on the back of a card you have known for six months must not reset the
interval to ten minutes.

## `card_states`

Exactly one row per card: the scheduler's current memory of it. This table is
overwritten on every review — the history lives in `reviews`.

| Column | Type | Notes |
|---|---|---|
| `card_id` | TEXT PK | `REFERENCES cards(id) ON DELETE CASCADE` |
| `due` | TIMESTAMP | When the card next appears |
| `stability` | REAL | FSRS: days until recall probability decays to the retention target |
| `difficulty` | REAL | FSRS: how hard this card is for this user, roughly 1–10 |
| `elapsed_days` | INTEGER | Days between the previous two reviews |
| `scheduled_days` | INTEGER | The interval FSRS asked for at the last review |
| `reps` | INTEGER | Total reviews |
| `lapses` | INTEGER | Times the card was forgotten from Review state |
| `state` | INTEGER | 0 new, 1 learning, 2 review, 3 relearning |
| `remaining_steps` | INTEGER | Position in the learning/relearning step ladder |
| `last_review` | TIMESTAMP NULL | NULL for a card never reviewed |

Index: `idx_card_states_due (due)` — the due query runs on every queue build.

`remaining_steps` was added in migration 003 and is not in the original plan.
`go-fsrs/v4` keeps a card's position in the short-term step ladder on the card
itself; without persisting it, every review of a learning card restarts the
ladder and the ten-minute step never advances to the next one. It is scheduler
bookkeeping, not a user-facing setting.

A card and its state are created in a single transaction, so a card can never
exist without a schedule.

## `reviews`

The append-only log. **Nothing in Kaart updates or deletes a row in this table.**

| Column | Type | Notes |
|---|---|---|
| `id` | INTEGER PK AUTOINCREMENT | Insertion order |
| `card_id` | TEXT | `REFERENCES cards(id) ON DELETE CASCADE` |
| `rating` | INTEGER | 1 again, 2 hard, 3 good, 4 easy |
| `state` | INTEGER | The card's state **before** this review |
| `due` | TIMESTAMP | What the card's due date was before this review |
| `stability`, `difficulty` | REAL | The memory model **before** this review |
| `elapsed_days` | INTEGER | Days since the previous review |
| `last_elapsed_days` | INTEGER | The previous review's `elapsed_days` |
| `scheduled_days` | INTEGER | The interval that had been scheduled |
| `reviewed_at` | TIMESTAMP | |
| `duration_ms` | INTEGER | Reveal-to-rating time, capped at one hour |

Indexes: `idx_reviews_card (card_id)`, `idx_reviews_time (reviewed_at)`.

### Why every value is the *pre*-review state

FSRS parameter optimisation works by replaying real history: for each review it
needs the memory state the algorithm was predicting *from*, paired with what the
user actually did. A row recording the state after the review has thrown that
pairing away — the prediction and its outcome are no longer separable.

### Why it is append-only

The log is the only record that cannot be reconstructed. `card_states` is
derivable by replaying `reviews`; `reviews` is derivable from nothing. Editing a
row to "fix" a mis-tap silently corrupts the training data for every future
parameter optimisation, and does so invisibly. A wrong review is data; a
retconned review is a lie.

Optimisation itself is deliberately deferred — it needs a few thousand real
reviews before it improves on the defaults. The table accumulates them in the
meantime. See `docs/scheduling.md`.

---

## Cascade behaviour

With `foreign_keys(1)` on:

- Deleting a **deck** removes its cards, their states, and their reviews.
- Deleting a **card** removes its state and its reviews, and leaves the deck alone.

Both paths are covered by tests in `internal/store/sqlite/sqlite_test.go`, which
run against a real temp-file database rather than a mock — the cascades being
asserted are SQLite's behaviour, and a mock would only assert our beliefs about it.
