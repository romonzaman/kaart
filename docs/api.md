# HTTP API

Base URL: `http://localhost:8080`. Everything is JSON. There is no
authentication — the server binds to loopback and serves one local user.

## Conventions

- Request bodies must be `application/json`. Unknown fields are **rejected**, so
  a typo'd field name fails loudly rather than silently doing nothing.
- Timestamps are RFC3339 in UTC on the wire.
- IDs are UUIDv7 strings, generated server-side.
- `PATCH` bodies are sparse: an omitted field is left alone, and a field present
  with a zero value is applied. That is why every `PATCH` field is nullable.

### Errors

Every error has the same shape and never contains SQL:

```json
{ "error": { "code": "invalid_request", "message": "front must not be empty" } }
```

| Code | Status | Meaning |
|---|---|---|
| `invalid_request` | 400 | Malformed body, unknown field, or failed validation |
| `not_found` | 404 | No such deck, card, or endpoint |
| `conflict` | 409 | The operation is not allowed in the resource's current state |
| `internal` | 500 | Something broke; the detail is in the server log, not the response |

### Request IDs

Every response carries `X-Request-Id`. Send your own to have it echoed back and
used in the server logs — useful for tracing a retried review submission.

---

## `GET /healthz`

```bash
curl -s localhost:8080/healthz
```

```json
{ "status": "ok", "version": "dev", "time": "2026-05-04T09:00:00Z" }
```

---

## Decks

### `GET /api/v1/decks`

Query: `include_archived=true` to include archived decks (default: excluded).

```bash
curl -s localhost:8080/api/v1/decks
```

```json
{
  "decks": [
    {
      "id": "0195f3a1-...",
      "name": "Estonian A1",
      "description": "beginner vocabulary",
      "new_cards_per_day": 20,
      "max_reviews_per_day": 200,
      "desired_retention": 0.9,
      "fsrs_weights": null,
      "created_at": "2026-05-04T09:00:00Z",
      "updated_at": "2026-05-04T09:00:00Z",
      "archived_at": null
    }
  ]
}
```

### `POST /api/v1/decks`

`name` is required (1–200 characters after trimming). Everything else is optional
and falls back to the defaults.

```bash
curl -s -X POST localhost:8080/api/v1/decks \
  -H 'Content-Type: application/json' \
  -d '{"name":"Estonian A1","new_cards_per_day":10,"desired_retention":0.9}'
```

`201 Created` with the deck body above.

| Field | Rules |
|---|---|
| `name` | required, 1–200 characters trimmed |
| `description` | ≤ 2000 characters |
| `new_cards_per_day` | 0–1000 |
| `max_reviews_per_day` | 0–10000 |
| `desired_retention` | 0.70–0.99 |
| `fsrs_weights` | exactly 21 numbers, or omitted for library defaults |

### `GET /api/v1/decks/{deckID}`

`200` with the deck, `404` if it does not exist.

### `PATCH /api/v1/decks/{deckID}`

Any subset of the create fields, plus `archived` (boolean).

```bash
curl -s -X PATCH localhost:8080/api/v1/decks/$DECK \
  -H 'Content-Type: application/json' \
  -d '{"new_cards_per_day":5,"archived":true}'
```

Sending `"fsrs_weights": []` clears an override back to the library defaults.

### `DELETE /api/v1/decks/{deckID}`

`204 No Content`. Removes the deck's cards, their states, and their reviews.
This is not reversible — archiving is the recoverable option.

---

## Cards

### `GET /api/v1/decks/{deckID}/cards`

Query: `limit` (1–500, default 50), `offset` (default 0), `q` (case-insensitive
substring over front, back, and hint).

```bash
curl -s "localhost:8080/api/v1/decks/$DECK/cards?q=koer&limit=20"
```

```json
{
  "cards": [
    {
      "id": "0195f3a2-...",
      "deck_id": "0195f3a1-...",
      "front": "koer",
      "back": "dog",
      "hint": "KOH-er",
      "tags": ["animals", "a1"],
      "created_at": "2026-05-04T09:01:00Z",
      "updated_at": "2026-05-04T09:01:00Z",
      "suspended_at": null
    }
  ],
  "total": 1,
  "limit": 20,
  "offset": 0
}
```

`total` is the count matching the filter, not the size of this page.

### `POST /api/v1/decks/{deckID}/cards`

Creates the card and its scheduling state (New, due now) in one transaction.

```bash
curl -s -X POST localhost:8080/api/v1/decks/$DECK/cards \
  -H 'Content-Type: application/json' \
  -d '{"front":"koer","back":"dog","hint":"KOH-er","tags":["animals","a1"]}'
```

`201 Created` with the card. `front` and `back` must be non-empty after
trimming; at most 20 tags of at most 32 characters each (blanks are dropped).

### `GET /api/v1/cards/{cardID}` · `PATCH` · `DELETE`

`PATCH` accepts any subset of `front`, `back`, `hint`, `tags`. Editing content
never touches the card's schedule.

```bash
curl -s -X PATCH localhost:8080/api/v1/cards/$CARD \
  -H 'Content-Type: application/json' -d '{"back":"dog (noun)"}'
```

`DELETE` returns `204` and removes the card's state and reviews.

### `POST /api/v1/cards/{cardID}/suspend` · `/unsuspend`

No body. Returns `200` with the updated card. A suspended card is excluded from
every queue, and reviewing it returns `409 conflict`.

---

## Studying

### `GET /api/v1/decks/{deckID}/queue`

Query: `limit` (1–500, default 50).

Builds a study session. Ordering is learning and relearning cards first (soonest
due first), then review cards by due date, then new cards — capped by the deck's
remaining daily allowances. Suspended cards never appear.

```bash
curl -s "localhost:8080/api/v1/decks/$DECK/queue?limit=50"
```

```json
{
  "deck_id": "0195f3a1-...",
  "now": "2026-05-04T09:05:00Z",
  "counts": { "total": 1, "new": 1, "learning": 0, "review": 0 },
  "items": [
    {
      "card": { "id": "0195f3a2-...", "front": "koer", "back": "dog", "...": "..." },
      "state": {
        "card_id": "0195f3a2-...",
        "due": "2026-05-04T09:01:00Z",
        "stability": 0, "difficulty": 0,
        "elapsed_days": 0, "scheduled_days": 0,
        "reps": 0, "lapses": 0,
        "state": "new", "state_code": 0,
        "last_review": null
      },
      "previews": [
        { "rating": 1, "rating_name": "again", "label": "<1m",  "interval_seconds": 60,     "due": "...", "state": "learning" },
        { "rating": 2, "rating_name": "hard",  "label": "6m",   "interval_seconds": 360,    "due": "...", "state": "learning" },
        { "rating": 3, "rating_name": "good",  "label": "10m",  "interval_seconds": 600,    "due": "...", "state": "learning" },
        { "rating": 4, "rating_name": "easy",  "label": "4d",   "interval_seconds": 345600, "due": "...", "state": "review" }
      ]
    }
  ]
}
```

`previews` is always four entries in Again/Hard/Good/Easy order, computed by the
same scheduler that will run when the rating arrives. The `label` is what goes on
the button, so the review screen needs no interval maths and no second round trip
per card — that would put a network hop inside the tightest loop in the product.

The daily caps are counted against the **local** day, not UTC: the binary runs on
the user's machine, so "today" should turn over when their day does.

### `POST /api/v1/cards/{cardID}/review`

```bash
curl -s -X POST localhost:8080/api/v1/cards/$CARD/review \
  -H 'Content-Type: application/json' \
  -d '{"rating":3,"duration_ms":2400}'
```

```json
{
  "card_id": "0195f3a2-...",
  "rating": 3,
  "rating_name": "good",
  "reviewed_at": "2026-05-04T09:05:00Z",
  "next_due": "2026-05-04T09:15:00Z",
  "state": {
    "due": "2026-05-04T09:15:00Z",
    "stability": 3.17, "difficulty": 5.28,
    "reps": 1, "lapses": 0,
    "state": "learning", "state_code": 1,
    "last_review": "2026-05-04T09:05:00Z",
    "...": "..."
  }
}
```

`rating` is 1 (again), 2 (hard), 3 (good), 4 (easy). `duration_ms` is optional,
0–3600000, and is the reveal-to-rating time.

The new state and the log row are written in one transaction, and the log row
records the state as it was *before* the rating. Reviewing a suspended card
returns `409`.

### `GET /api/v1/decks/{deckID}/stats`

Query: `days` (1–365, default 30) — the histogram window.

```bash
curl -s "localhost:8080/api/v1/decks/$DECK/stats"
```

```json
{
  "deck_id": "0195f3a1-...",
  "now": "2026-05-04T09:05:00Z",
  "due_now": 12,
  "new": 40,
  "learning": 3,
  "suspended": 1,
  "total_cards": 120,
  "reviews_today": 34,
  "new_cards_today": 8,
  "remaining_new_today": 12,
  "remaining_reviews_today": 166,
  "next_due": "2026-05-04T12:05:00Z",
  "histogram": [
    { "date": "2026-05-03", "count": 41 },
    { "date": "2026-05-04", "count": 34 }
  ]
}
```

Days with no reviews are omitted from `histogram` rather than sent as zeroes;
the client fills the gaps when it draws the bars.

`next_due` is the earliest due time strictly after now among the deck's
unsuspended cards, or `null` when everything is already due (or the deck is
empty). It exists so the study screen can say "nothing due — next card in 3
hours" instead of showing an unexplained empty session.
