// Package domain holds Kaart's core types. It has no dependencies on storage,
// HTTP, or the scheduling library, so every other package can import it freely.
package domain

import "time"

// Rating is the user's self-assessment of recall for a single review.
type Rating int

// Rating values. These match the FSRS grades and must not be renumbered:
// they are persisted in the append-only reviews log.
const (
	RatingAgain Rating = 1
	RatingHard  Rating = 2
	RatingGood  Rating = 3
	RatingEasy  Rating = 4
)

// String implements fmt.Stringer.
func (r Rating) String() string {
	switch r {
	case RatingAgain:
		return "again"
	case RatingHard:
		return "hard"
	case RatingGood:
		return "good"
	case RatingEasy:
		return "easy"
	default:
		return "unknown"
	}
}

// Valid reports whether r is one of the four defined ratings.
func (r Rating) Valid() bool {
	return r >= RatingAgain && r <= RatingEasy
}

// State is where a card sits in the learning lifecycle.
type State int

// State values. Persisted; do not renumber.
const (
	StateNew        State = 0
	StateLearning   State = 1
	StateReview     State = 2
	StateRelearning State = 3
)

// String implements fmt.Stringer.
func (s State) String() string {
	switch s {
	case StateNew:
		return "new"
	case StateLearning:
		return "learning"
	case StateReview:
		return "review"
	case StateRelearning:
		return "relearning"
	default:
		return "unknown"
	}
}

// Valid reports whether s is one of the four defined states.
func (s State) Valid() bool {
	return s >= StateNew && s <= StateRelearning
}

// Deck scheduling defaults, applied when a deck is created without overrides.
const (
	DefaultNewCardsPerDay   = 20
	DefaultMaxReviewsPerDay = 200
	DefaultDesiredRetention = 0.9
)

// FSRSWeightCount is the number of weights go-fsrs/v4 expects. A deck either
// supplies exactly this many or supplies none and gets the library defaults.
const FSRSWeightCount = 21

// Deck is a named collection of cards with its own scheduling settings.
type Deck struct {
	ID          string
	Name        string
	Description string

	// Scheduling settings (migration 002).
	NewCardsPerDay   int
	MaxReviewsPerDay int
	DesiredRetention float64
	// FSRSWeights is nil when the deck uses the library's default weights.
	FSRSWeights []float64

	CreatedAt  time.Time
	UpdatedAt  time.Time
	ArchivedAt *time.Time
}

// Archived reports whether the deck has been archived.
func (d *Deck) Archived() bool { return d.ArchivedAt != nil }

// Card is a single question/answer pair belonging to a deck.
type Card struct {
	ID          string
	DeckID      string
	Front       string
	Back        string
	Hint        string
	Tags        []string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	SuspendedAt *time.Time
}

// Suspended reports whether the card is excluded from review queues.
func (c *Card) Suspended() bool { return c.SuspendedAt != nil }

// CardState is the scheduler's memory of one card. Exactly one row exists per
// card; it is overwritten on every review. The history lives in Review.
type CardState struct {
	CardID        string
	Due           time.Time
	Stability     float64
	Difficulty    float64
	ElapsedDays   int
	ScheduledDays int
	Reps          int
	Lapses        int
	State         State
	// RemainingSteps is FSRS's position within the learning/relearning step
	// ladder. It is scheduler-owned bookkeeping, not user-facing.
	RemainingSteps int
	// LastReview is nil for a card that has never been reviewed.
	LastReview *time.Time
}

// NewCardState returns the state a freshly created card starts in: due
// immediately, no memory, state New.
func NewCardState(cardID string, now time.Time) CardState {
	return CardState{
		CardID: cardID,
		Due:    now.UTC(),
		State:  StateNew,
	}
}

// Review is one immutable row in the append-only review log. Every numeric
// field records the state as it was *before* the review was applied, so the
// log can be replayed to re-optimise FSRS parameters later.
type Review struct {
	ID              int64
	CardID          string
	Rating          Rating
	State           State
	Due             time.Time
	Stability       float64
	Difficulty      float64
	ElapsedDays     int
	LastElapsedDays int
	ScheduledDays   int
	ReviewedAt      time.Time
	DurationMS      int
}

// DeckCounts is a snapshot of a deck's queue composition at a point in time.
type DeckCounts struct {
	Total     int
	Due       int
	New       int
	Learning  int
	Suspended int
}

// DayCount is one bar of the review histogram.
type DayCount struct {
	Date  string // YYYY-MM-DD, UTC
	Count int
}
