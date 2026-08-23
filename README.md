# Kaart

An open-source, local-first flashcard app with FSRS spaced repetition.

Your cards live in a SQLite file on your own machine. There is no account, no
sync service, and no network call anywhere in the application.

**Status: phases 1–5.** The Go backend (schema, store, HTTP API, FSRS scheduler
and review loop) and the Expo frontend (deck list, card entry, card browser,
review session) are implemented. Import/export (phase 6) and release packaging
(phase 7) are not built yet.

---

## Quickstart

Requires Go 1.22+ and Node 20+. There is no cgo, so the server needs nothing else
installed.

```bash
go mod tidy          # resolve Go dependencies (first run only)
make app-deps        # install the Expo app's dependencies (first run only)

make run             # terminal 1 — API on 127.0.0.1:8080
make app             # terminal 2 — Expo web on http://localhost:8081
```

Open http://localhost:8081, create a deck, add a few cards, and press Study.

To check everything the way CI would:

```bash
make check           # go vet + go test + tsc --noEmit + jest
```

### Talking to the API directly

```bash
curl -s localhost:8080/healthz

DECK=$(curl -s -X POST localhost:8080/api/v1/decks \
  -H 'Content-Type: application/json' \
  -d '{"name":"Estonian A1"}' | grep -o '"id":"[^"]*"' | cut -d'"' -f4)

curl -s -X POST localhost:8080/api/v1/decks/$DECK/cards \
  -H 'Content-Type: application/json' \
  -d '{"front":"koer","back":"dog","hint":"KOH-er"}'

curl -s "localhost:8080/api/v1/decks/$DECK/queue"
```

### Server flags

| Flag | Default | |
|---|---|---|
| `--db` | `./kaart.db` | SQLite file; created and migrated on first run |
| `--addr` | `127.0.0.1:8080` | Listen address |
| `--cors-origin` | `http://localhost:8081` | Repeatable; allowed browser origins |
| `--log-level` | `info` | `debug`, `info`, `warn`, `error` |
| `--migrate-only` | | Apply migrations and exit |
| `--version` | | Print version and exit |

The app reads `EXPO_PUBLIC_API_URL` and defaults to `http://localhost:8080`. Set
it in `app/.env` to point a phone at a laptop running the server.

### Make targets

Backend: `deps` · `build` · `test` · `lint` · `fmt` · `vet` · `run` · `migrate` · `clean`
Frontend: `app-deps` · `app` · `app-typecheck` · `app-test` · `app-check`
Both: `check`

---

## Keyboard

The review loop is completable without touching the mouse.

| Key | |
|---|---|
| `Space` / `Enter` | Show the answer |
| `1` `2` `3` `4` | Again / Hard / Good / Easy |
| `Esc` | Leave the session (confirms if you're partway through) |
| `Cmd/Ctrl+Enter` | Save the card you're editing and clear the form |

---

## Project layout

```
cmd/kaartd/          server entrypoint, flags, graceful shutdown
internal/
  api/               HTTP handlers, routing, middleware, validation
  store/             Store interface
  store/sqlite/      the only implementation in v1
  store/migrations/  goose .sql files, embedded
  scheduler/         FSRS wrapper — pure, no I/O
  domain/            Deck, Card, CardState, Review
  clock/             Clock interface, real and fake
app/                 Expo app (React Native + React Native Web)
  app/               expo-router file routes
  components/        Button, TextField, Card, Screen, EmptyState, Stat, theme
  lib/api/           typed API client — no `any`
  lib/               query hooks, session state machine, review outbox
docs/                schema, API reference, scheduling
```

Six constraints hold the design together:

**All data access goes through `store.Store`.** No SQL exists outside
`internal/store`. That is what makes a Postgres or Supabase backend a new file
rather than a rewrite — and it is the constraint most easily lost by accident, so
enforce it in review.

**The scheduler is pure.** `internal/scheduler` takes a card state and a rating
and returns a new state. It does no I/O and imports nothing from `store` or
`api`, which is what lets scheduling be tested against a fake clock.

**Time is injected.** Nothing in business logic calls `time.Now()`; a `Clock`
is passed in.

**The review log is append-only.** Nothing updates or deletes a row in
`reviews`. See [docs/schema.md](docs/schema.md) for why.

**React Native primitives only.** No `div`, no web-only CSS, no Tailwind. The
same source has to build for iOS, and a single web-only primitive is enough to
break that silently on web-only development.

**The session state machine is pure too.** `app/lib/session.ts` is a reducer with
no React in it, so the reveal/rate cycle — the part most likely to break subtly
and least likely to be caught by looking at it — is unit-tested directly.

---

## How the review loop behaves

The rating buttons are labelled with intervals the **server** computed. The queue
response carries each card's four projected outcomes, so pressing Good writes
exactly the interval the button promised. No client-side interval maths exists,
and there is no second request per card.

Reviews submit optimistically through an outbox (`app/lib/reviewQueue.ts`). The
next card appears immediately; the submission drains in the background and
retries with backoff if the server is unreachable. Killing `kaartd` mid-session
does not lose reviews — they land when it comes back. A rejection that retrying
cannot fix (a suspended card, a deleted card) steps aside rather than blocking
everything queued behind it, and is surfaced on the summary screen with a retry
button.

Known gap: there is no undo. It is a v2 candidate — reversing a review needs a
path back through the scheduler, not just a deleted row.

---

## Docs

- [docs/schema.md](docs/schema.md) — every table, and why `reviews` is append-only
- [docs/api.md](docs/api.md) — every endpoint with curl examples
- [docs/scheduling.md](docs/scheduling.md) — FSRS in plain language, and what the four ratings mean
- [docs/course-tasks.md](docs/course-tasks.md) — prompts for building, curating, restructuring, and reporting on course decks through an agent

## Where the server runs

`kaartd` is a local process you start yourself, not a hosted service. That is the
point of local-first, but it is worth saying plainly: nobody is running this for
you, and closing the terminal stops it. Phase 7 of the plan embeds the web UI in
the binary so it becomes a single download.

## Contributing

The build is phased; see the plan in `kaartbuildprompts.md`. Before opening a PR:

```bash
make fmt && make check && make lint
```

Tests are written in the same pass as the code, not afterwards. Store tests run
against a real temp-file SQLite database rather than mocks.

## Licence

MIT. See [LICENSE](LICENSE).
