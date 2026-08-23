# Scheduling

Kaart schedules with **FSRS** (Free Spaced Repetition Scheduler), via
[`go-fsrs/v4`](https://pkg.go.dev/github.com/open-spaced-repetition/go-fsrs/v4).

## The idea in one paragraph

Every card carries two numbers. **Stability** is roughly how many days you can
leave a card before your chance of recalling it drops to the retention target.
**Difficulty** is how hard this particular card is for you, on a scale of about
1 to 10. When you rate a card, FSRS updates both, then picks the next interval so
that when the card comes back, your predicted chance of remembering it is
`desired_retention` — 0.9 by default. Rate Good and stability grows, so the next
gap is longer. Rate Again and stability collapses, difficulty rises, and the card
drops back into short steps.

The four ratings are not a satisfaction survey. They mean:

| Rating | What it means |
|---|---|
| **Again** | You did not recall it. The card lapses and comes back in minutes. |
| **Hard** | You recalled it, but slowly or with effort. Shorter interval than Good. |
| **Good** | You recalled it with normal effort. This is the default answer. |
| **Easy** | It was instant and effortless. Longer interval than Good. |

Answer honestly. Rating Easy on cards you barely recalled inflates the intervals
and you stop seeing the cards you actually need.

## Card states

```
new ──Good──▶ learning ──(steps done)──▶ review
                  ▲                        │
                  │                      Again
                  └──── relearning ◀───────┘
```

- **New** — never reviewed.
- **Learning** — being introduced, on short steps measured in minutes.
- **Review** — graduated, on intervals measured in days.
- **Relearning** — a review card you forgot, back on short steps. Each lapse
  increments the card's `lapses` count and lowers its stability.

## Deck settings

| Setting | Default | Effect |
|---|---|---|
| `new_cards_per_day` | 20 | How many unseen cards enter the queue each day |
| `max_reviews_per_day` | 200 | Cap on total reviews per day, so a backlog cannot swamp a session |
| `desired_retention` | 0.9 | Target recall probability. Higher = shorter intervals = more reviews |
| `fsrs_weights` | library defaults | 21-value FSRS parameter vector |

`desired_retention` is the one worth understanding: raising it from 0.90 to 0.95
does not make you learn better, it makes you review considerably more often for a
modest gain in recall. 0.9 is the value the FSRS authors recommend, and Kaart
does not encourage moving it.

The daily caps are counted against your **local** day, not UTC — the server runs
on your machine, so "today" turns over at your midnight.

## What's inside `internal/scheduler`

The package is pure. It imports nothing from `store` or `api`, does no I/O, and
takes the current time as a parameter rather than calling `time.Now()`. That is
what makes scheduling testable: the tests drive a fake clock forward to each
card's due date and assert on the resulting intervals.

It exposes two operations:

- **`Preview`** — the projected next state for all four ratings, committing
  nothing. The queue endpoint uses it to label the rating buttons with real
  intervals ("<1m / 6m / 10m / 4d").
- **`Apply`** — the new state for the rating actually given.

Both return an error. A pure function ideally would not, but the underlying
library validates its inputs, and swallowing a validation failure would mean
writing a silently wrong due date.

### Two deliberate choices

**Fuzz is off.** FSRS normally jitters intervals by a few percent so cards
introduced together do not stay clumped forever. Kaart disables it, for two
reasons: a non-deterministic scheduler cannot be regression-tested, and the
intervals shown on the rating buttons would not match what gets written when the
button is pressed. If clumping turns out to matter in daily use, re-enable it
behind a deck setting — and accept that `Preview` becomes advisory.

**`remaining_steps` is persisted.** `go-fsrs/v4` tracks a card's position in the
learning-step ladder on the card itself. Migration `003` adds a column for it.
Without it, every review of a learning card would restart the ladder and short
steps would never advance.

## Parameter optimisation is deferred

FSRS can fit its 21 weights to your own review history, and that is genuinely
better than the defaults — but only once there are a few thousand reviews to fit
against. Before that it overfits noise.

So the `reviews` table is accumulating them now. Every row records the card's
memory state as it was **before** the review, paired with the rating you gave.
That pairing — prediction and outcome — is exactly what an optimiser consumes.
The table is append-only and nothing in Kaart ever rewrites a row; a retconned
review would corrupt that training data invisibly.

When the optimiser lands, it will read `reviews`, produce a 21-value weight
vector, and write it to the deck's `fsrs_weights`. Nothing else has to change.

## Testing scheduling changes

`internal/scheduler/scheduler_test.go` contains `TestFixedSequenceIsStable`,
which replays a fixed sequence of twenty ratings and asserts the final state. It
exists so that a library bump or a refactor of the domain/library conversion
cannot silently move everyone's schedule.

Its golden stability and difficulty values start at zero, which disables those
two assertions. Pin them once against a build you trust:

```
go test ./internal/scheduler -run TestFixedSequenceIsStable -v
```

and copy the logged values into the `goldenStability` and `goldenDifficulty`
constants. After that, a failure means scheduling behaviour changed — decide
whether that was intended before you update the numbers.
