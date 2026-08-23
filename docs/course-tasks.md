# Kaart — course management tasks

Prompts for running Kaart's deck and card work through Cowork: building a course
deck for a new language, cleaning up one that has gone messy, reorganising a
deck that outgrew itself, and reading what the review history is telling you.

These are *content* tasks, not code tasks. Nothing here changes the repository —
they operate on a running `kaartd` over its HTTP API.

---

## How to use this document

Paste the **context block** first, then exactly one task. Each task has
acceptance criteria; check them before moving on.

Start `kaartd` before you start the task:

```bash
make run    # 127.0.0.1:8080
```

If the server is not running, every request fails with a connection error and
the agent has nothing to work against.

---

# CONTEXT BLOCK

*Prepend this to every task below.*

````
You are managing content in **Kaart**, a local-first flashcard app with FSRS
spaced repetition. It runs as a local Go server on `http://localhost:8080`.
There are no accounts and no authentication.

## Verify the server first

    curl -sS localhost:8080/healthz

Expect `{"status":"ok",...}`. If this fails, stop and tell the user to run
`make run` — do not continue and do not invent an alternative data path.

## The API you have

    GET    /api/v1/decks                      list decks
    POST   /api/v1/decks                      create a deck
    GET    /api/v1/decks/{deckID}             one deck
    PATCH  /api/v1/decks/{deckID}             update name/description/settings/archived
    DELETE /api/v1/decks/{deckID}             delete deck AND its cards and review history

    GET    /api/v1/decks/{deckID}/cards       ?limit=(1-500,def 50)&offset=&q=
    POST   /api/v1/decks/{deckID}/cards       create a card
    GET    /api/v1/decks/{deckID}/stats       ?days=(1-365,def 30)
    GET    /api/v1/decks/{deckID}/queue       ?limit=  what is due right now

    GET    /api/v1/cards/{cardID}
    PATCH  /api/v1/cards/{cardID}             front/back/hint/tags only
    DELETE /api/v1/cards/{cardID}             deletes the card AND its review history
    POST   /api/v1/cards/{cardID}/suspend     take it out of the queue, keep everything
    POST   /api/v1/cards/{cardID}/unsuspend

Full reference with examples: `docs/api.md`.

## Four things that are not in the API

Do not plan around capabilities that do not exist. As of phases 1–5:

1. **There is no bulk import.** Every card is one POST. Loop; do not look for a
   CSV endpoint.
2. **There is no move-between-decks.** `PATCH /cards/{id}` accepts front, back,
   hint, and tags — nothing else. Relocating a card means create-then-delete,
   which destroys its scheduling state and its review history.
3. **There is no per-card state endpoint.** Scheduling state (due, reps, lapses)
   is only visible through the queue, which returns just what is due now. For
   anything historical, read the database directly — see Task D.
4. **Nothing deduplicates.** Posting the same front twice creates two cards.
   Check before you write.

## Rules you must not break

- **Never call the review endpoint.** `POST /cards/{id}/review` is how a human
  records that they recalled something. Calling it fabricates a memory the user
  never had, and the reviews table is append-only — there is no undo, and the
  false row will be fed to FSRS parameter optimisation later.
- **Never write to the database directly.** Read-only queries are fine (Task D).
  Writes bypass validation, transactions, and the store layer's invariants.
- **Prefer suspend over delete.** Deleting a card cascades to its review rows.
  A card the user has reviewed forty times carries forty rows of evidence about
  how they learn; a bad *card* is not a reason to destroy that. Suspend removes
  it from the queue and keeps everything.
- **Never delete a deck without explicit confirmation** naming that deck. It
  takes every card and every review with it.
- **Editing content does not reset scheduling.** Fixing a typo, adding a hint,
  or retagging is free — the card keeps its interval. Say so when you propose
  edits; users often assume otherwise and leave errors in place because of it.

## Validation limits

Requests violating these come back `400 invalid_request` with a message naming
the field. Unknown JSON fields are rejected too — a typo'd key is a 400, not a
silent no-op.

| Field | Limit |
|---|---|
| deck `name` | 1–200 characters after trimming |
| deck `description` | ≤ 2000 |
| card `front`, `back` | non-empty after trimming, ≤ 10000 |
| card `hint` | ≤ 10000 |
| `tags` | ≤ 20 entries, each ≤ 32 characters; blanks are dropped |
| `new_cards_per_day` | 0–1000 |
| `max_reviews_per_day` | 0–10000 |
| `desired_retention` | 0.70–0.99 |

Errors are always `{"error":{"code","message"}}` with code `invalid_request`,
`not_found`, `conflict`, or `internal`.

## How to write a language card

The person using this studies 20–30 minutes a day and will see these cards for
years. Card quality compounds; a sloppy card costs a few seconds every time it
comes up, forever.

- **One card, one fact.** A card that asks for a word *and* its gender *and* its
  plural cannot be rated honestly — the user recalls two of three and has no way
  to say so. Split it.
- **The front must have exactly one right answer.** "run" is a bad front: which
  sense? Disambiguate on the *front* — "run (a race)" — not by listing options
  on the back.
- **Recognition first.** Default to L2 → L1 (foreign word on the front). A
  production card (L1 → L2) is a *separate card* — Kaart has front/back only,
  no reverse card type — and adding both doubles the daily load. Only add
  production cards when the user asks.
- **Hints are for pronunciation, not for the answer.** A hint that narrows the
  answer turns every review into a cued recall and inflates the ratings. For
  languages where spelling predicts pronunciation, leave the hint empty rather
  than filling it with noise.
- **Choose words by frequency.** The most common couple of thousand words carry
  the large majority of everyday speech; a beginner deck of picturesque
  vocabulary is a deck that does not pay rent.
- **Do not teach confusable pairs together.** Words differing by one letter or
  one vowel, introduced in the same batch, interfere with each other and both
  get learned slower. Separate them by a few dozen cards.
- **Add a context sentence to the back** for function words and anything whose
  meaning is carried by usage rather than translation.

### Tags

Use a small controlled vocabulary and reuse it exactly. Tags are free text, so
`a1`, `A1`, and `level-a1` will happily coexist and none of them will be useful.
A workable scheme: one level tag (`a1`), one part-of-speech tag (`noun`,
`verb`), one topic tag (`food`, `travel`). Three tags, twenty allowed — leave
room.

### How big should a deck be?

At the default 20 new cards a day, a 300-card deck is about fifteen days of
intake and several months of reviews. Do not build 2000 cards for someone
starting a language this week — the daily cap means the surplus just sits there,
and vocabulary chosen before you know how they study is vocabulary chosen badly.
Build a first tranche, let them study it, extend from what the data says.

## Working style

- Show the user a sample of 5–10 cards and get agreement **before** creating
  hundreds. A wrong convention caught at card 8 is a rewrite of 8 cards.
- Report counts honestly: created, skipped as duplicates, failed with the reason.
  Never report a card as created without a 201.
- When a decision is not specified here, pick the more conservative option and
  say which you picked.
````

---

# TASK A — Author a new course deck

````
Build a vocabulary deck for a language and level the user names.

## Deliverables

1. Ask for, or confirm, four things before writing anything:
   - target language and the user's native/reference language
   - level (A1, A2, …) or a topic scope
   - how many cards in this first tranche (suggest 150–300; explain the daily-cap
     arithmetic if they ask for thousands)
   - whether they want pronunciation hints (yes for languages where spelling
     does not predict sound)

2. Draft **10 sample cards** and show them as a table before creating anything.
   Ask the user to confirm the conventions: disambiguation style, hint format,
   tag scheme, whether articles/genders are included with nouns.

3. Create the deck:

   ```bash
   DECK=$(curl -sS -X POST localhost:8080/api/v1/decks \
     -H 'Content-Type: application/json' \
     -d '{"name":"German A1","description":"Core A1 vocabulary, recognition","new_cards_per_day":15}' \
     | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')
   echo "$DECK"
   ```

4. Write the cards to a TSV file first — `front <TAB> back <TAB> hint <TAB> tags`
   — so they are reviewable and re-runnable, then POST them with a script.
   Build the JSON in Python, not by interpolating into a shell string: card text
   contains apostrophes, quotes, and non-ASCII characters that will corrupt a
   hand-quoted `-d` argument.

   ```python
   # post_cards.py — reads TSV on stdin, skips fronts already in the deck
   import csv, json, sys, urllib.request

   BASE, deck = "http://localhost:8080", sys.argv[1]

   def call(method, path, payload=None):
       body = None if payload is None else json.dumps(payload).encode()
       req = urllib.request.Request(BASE + path, data=body, method=method)
       if body: req.add_header("Content-Type", "application/json")
       try:
           with urllib.request.urlopen(req) as r:
               return r.status, json.loads(r.read() or "null")
       except urllib.error.HTTPError as e:
           return e.code, json.loads(e.read() or "null")

   # existing fronts, paged
   existing, offset = set(), 0
   while True:
       _, page = call("GET", f"/api/v1/decks/{deck}/cards?limit=500&offset={offset}")
       for c in page["cards"]: existing.add(c["front"].strip())
       offset += 500
       if offset >= page["total"]: break

   created = skipped = 0
   errors = []
   for i, row in enumerate(csv.reader(sys.stdin, delimiter="\t"), start=1):
       if not row or not row[0].strip(): continue
       front, back = row[0].strip(), row[1].strip()
       hint = row[2].strip() if len(row) > 2 else ""
       tags = [t.strip() for t in row[3].split(",")] if len(row) > 3 else []
       if front in existing:
           skipped += 1; continue
       status, body = call("POST", f"/api/v1/decks/{deck}/cards",
                           {"front": front, "back": back, "hint": hint,
                            "tags": [t for t in tags if t]})
       if status == 201:
           created += 1; existing.add(front)
       else:
           errors.append((i, front, body.get("error", {}).get("message", status)))

   print(json.dumps({"created": created, "skipped": skipped,
                     "errors": errors}, ensure_ascii=False, indent=2))
   ```

5. Report created / skipped / failed with the line number and message for each
   failure. Do not summarise failures as "some cards had issues".

6. Set the deck's `new_cards_per_day` to something the user will actually
   sustain. The default 20 is right for a committed daily learner; propose 10
   for someone fitting study around a job.

## Acceptance criteria

- `GET /api/v1/decks/{deckID}/cards?limit=500` returns the expected count, and
  spot-checking five cards shows correct fronts, backs, hints, and tags.
- No two cards in the deck share a front. Verify, do not assume.
- Every front has exactly one defensible answer. Re-read the ten most ambiguous
  and fix them.
- The TSV file is left on disk so the deck can be rebuilt or reviewed.
- `GET /api/v1/decks/{deckID}/stats` shows `total_cards` matching, and
  `due_now` equal to the number of new cards (they are all due immediately).

## Out of scope

Do not create production (L1 → L2) cards unless asked. Do not touch other decks.
Do not review any card.
````

---

# TASK B — Curate an existing deck

````
Audit a deck the user names and fix what is wrong with it.

## Deliverables

1. Pull the whole deck, paging with `limit=500`, and analyse it locally. Report
   findings **before** changing anything, grouped by problem:

   - **Duplicate fronts** — exact matches, and near-matches differing only by
     case, punctuation, or an article.
   - **Ambiguous fronts** — a front with more than one correct answer, or one
     whose answer depends on context not present on the card.
   - **Two-facts-in-one** — cards whose back contains a list, a slash, or a
     parenthetical that is really a second question.
   - **Hints that give away the answer** — anything where the hint makes the
     recall trivial.
   - **Overlong cards** — a back that is a paragraph is a card that will be
     rated Again forever.
   - **Tag drift** — variants of the same tag (`a1` / `A1` / `level-a1`).
   - **Suspended cards** — list them and ask whether they should come back.

2. Propose a fix per finding and get approval before writing. For each, say
   which of these it is:
   - `PATCH` the content — the card stays, keeps its schedule and history
   - split into two cards — the original is edited, a new card is created
   - suspend — a card that is not worth studying but whose history is worth keeping
   - delete — only for a genuine duplicate with no review history

3. Before deleting anything, check whether it has been reviewed. There is no
   API for this; read it (read-only) from the database:

   ```bash
   sqlite3 "file:kaart.db?mode=ro" \
     "SELECT c.front, s.reps, s.lapses
        FROM cards c JOIN card_states s ON s.card_id = c.id
       WHERE c.id = 'CARD_ID';"
   ```

   `reps > 0` means deleting destroys real history. Suspend instead, and say so.

4. Apply the approved changes. Report what was patched, split, suspended, and
   deleted, with counts.

## Acceptance criteria

- No exact-duplicate fronts remain.
- Every tag in the deck is from the agreed controlled vocabulary.
- No card was deleted that had `reps > 0`, unless the user explicitly approved
  that specific card knowing its history would go.
- `total_cards` before and after is reported and the difference is accounted
  for card by card.
- Spot-check five patched cards: the content changed and the schedule did not.

## Out of scope

Do not add new vocabulary — that is Task A. Do not change deck settings.
````

---

# TASK C — Restructure decks

````
Split, merge, or retag decks. Read the warning first — this task can destroy
learning history, and part of the job is telling the user that before they
agree to it.

## The warning you must deliver first

Kaart has **no move-between-decks operation**. A card's deck is fixed at
creation. So:

- **Splitting a deck** means creating copies in a new deck and deleting the
  originals. The copies are brand new cards: state New, due immediately,
  zero reps, zero lapses. Every interval the user earned on those cards is
  gone, and their review history is deleted by cascade.
- **Merging decks** has the same problem in the other direction.
- **Retagging in bulk** is safe — tags are a `PATCH`, and content edits never
  touch scheduling.

Tell the user this in plain terms before doing any split or merge, with the
actual numbers: how many cards, how many of them have been reviewed, and how
many total review rows would be destroyed:

```bash
sqlite3 "file:kaart.db?mode=ro" \
  "SELECT COUNT(*) AS cards,
          SUM(CASE WHEN s.reps > 0 THEN 1 ELSE 0 END) AS studied,
          (SELECT COUNT(*) FROM reviews r JOIN cards c2 ON c2.id = r.card_id
            WHERE c2.deck_id = 'DECK_ID') AS review_rows
     FROM cards c JOIN card_states s ON s.card_id = c.id
    WHERE c.deck_id = 'DECK_ID';"
```

Then offer the alternatives, which are usually better:

- **Tag instead of split.** If the goal is "separate the food words from the
  travel words", tags do that without touching a single schedule.
- **Split only the unstudied cards.** Cards with `reps = 0` have nothing to
  lose. Move those, leave the studied ones where they are.
- **Archive rather than merge.** `PATCH /decks/{id}` with `{"archived":true}`
  hides a deck from the list without destroying it.

Proceed with a destructive split or merge only if the user confirms after
hearing the numbers.

## Deliverables

1. Read the deck, propose the grouping (by tag, level, topic, or the user's own
   rule), and show how many cards land in each group.
2. Deliver the warning above with real numbers.
3. Recommend the least destructive option that meets the goal, and say why.
4. On approval, execute — creating the new deck(s), recreating cards, deleting
   originals only after confirming each new card returned 201.
5. Write the full mapping to a file before deleting anything: old card id, new
   card id, front. If something fails halfway, that file is the only way back.

## Acceptance criteria

- Every card in the new deck exists and its content matches the original
  exactly, including tags and hints.
- No original was deleted before its replacement was confirmed created.
- The mapping file is on disk.
- The user was told, before approving, how many review rows the operation
  destroys.
- If the user chose a non-destructive alternative, no card was deleted at all.

## Out of scope

Do not rewrite card content while restructuring — that is Task B, and mixing
the two makes the mapping file useless as an audit trail.
````

---

# TASK D — Report on how a course is going

````
Read a deck's history and tell the user what it says. Read-only: this task
creates, edits, and deletes nothing.

## Deliverables

1. Start with the API:

   ```bash
   curl -sS "localhost:8080/api/v1/decks/$DECK/stats?days=90"
   ```

   This gives due-now, new, learning, suspended, total, reviews today, the
   remaining daily allowances, `next_due`, and a per-day review histogram.

2. Anything per-card is not in the API. Query the database read-only. This works
   while `kaartd` is running because the database is in WAL mode; if SQLite
   complains about the shared-memory file, copy `kaart.db`, `kaart.db-wal`, and
   `kaart.db-shm` aside and query the copy.

   **Cards that keep lapsing** — the ones costing the most for the least return:

   ```sql
   SELECT c.front, c.back, s.reps, s.lapses,
          ROUND(CAST(s.lapses AS REAL) / NULLIF(s.reps, 0), 2) AS lapse_rate
     FROM cards c JOIN card_states s ON s.card_id = c.id
    WHERE c.deck_id = 'DECK_ID' AND s.reps >= 4
    ORDER BY lapse_rate DESC, s.lapses DESC
    LIMIT 20;
   ```

   **Rating mix over the last 30 days** — is the deck too hard or too easy?

   ```sql
   SELECT r.rating, COUNT(*) AS n
     FROM reviews r JOIN cards c ON c.id = r.card_id
    WHERE c.deck_id = 'DECK_ID'
      AND r.reviewed_at >= strftime('%Y-%m-%dT%H:%M:%f000Z', 'now', '-30 days')
    GROUP BY r.rating ORDER BY r.rating;
   ```

   Ratings are 1 Again, 2 Hard, 3 Good, 4 Easy.

   **Time spent per day**:

   ```sql
   SELECT substr(r.reviewed_at, 1, 10) AS day,
          COUNT(*) AS reviews,
          ROUND(SUM(r.duration_ms) / 60000.0, 1) AS minutes
     FROM reviews r JOIN cards c ON c.id = r.card_id
    WHERE c.deck_id = 'DECK_ID'
    GROUP BY day ORDER BY day DESC LIMIT 30;
   ```

   **Consistency** — how many of the last 30 days had any review at all. This
   usually matters more than anything else in the report.

3. Write a short report covering:
   - **Consistency**: days studied out of the last 30, and the longest gap.
   - **Load**: reviews and minutes per active day, and whether the daily new-card
     setting is producing a sustainable or a growing backlog.
   - **Difficulty**: the Again share. Very roughly, an Again rate far above
     ~15% suggests cards that are too hard or ambiguous; far below suggests the
     new-card rate could rise.
   - **Leeches**: the worst 10 cards by lapse rate, each with a concrete
     recommendation — rewrite it (usually the front is ambiguous), split it, or
     suspend it.
   - **What to add next**, if they asked.

4. Be careful about what the numbers can support. A rising Again rate after a
   week away is a break, not a bad deck. A single bad day is noise. Say when the
   history is too short to conclude anything — under a few hundred reviews, it
   usually is.

## Acceptance criteria

- Nothing was created, edited, deleted, or reviewed.
- Every number in the report traces to a query shown in it.
- The leech list names specific cards with specific fixes, not "review your
  difficult cards".
- Where the data does not support a conclusion, the report says so rather than
  reaching for one.

## Out of scope

Do not fix the leeches — recommend, then hand over to Task B.
````

---

## Deliberately not automated

| Task | Why it stays manual |
|---|---|
| Rating cards | Only the person studying knows whether they recalled something. An agent calling the review endpoint corrupts the record permanently. |
| Changing `desired_retention` | It trades review volume against recall in a way only the user can judge. Explain it; let them choose. |
| Deleting decks | One request, unbounded loss, no undo. |
| Choosing what to study next | Deck-level curriculum decisions belong to the learner. Report the data, offer options. |

## When the API gains capabilities

Three of the constraints above are phase-limited, not permanent:

- **Bulk import (phase 6)** replaces Task A's POST loop with a single CSV upload
  and gives duplicate detection server-side.
- **A move-between-decks endpoint** would make Task C non-destructive. Until it
  exists, the warning in Task C is load-bearing.
- **A per-card state endpoint** would remove Task D's need to read the database.

When any of these land, update this document in the same change — a prompt doc
that describes an API that has moved on is worse than no prompt doc.
