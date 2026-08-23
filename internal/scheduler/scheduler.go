// Package scheduler decides when a card is next due.
//
// It is pure: it performs no I/O, imports nothing from store or api, and is
// fully determined by its inputs. Everything time-dependent arrives as a
// parameter, so scheduling behaviour is testable without a database or a clock.
package scheduler

import (
	"errors"
	"time"

	"github.com/romonzaman/kaart/internal/domain"
)

// ErrInvalidWeights is returned when a deck supplies an FSRS weight vector of
// the wrong length.
var ErrInvalidWeights = errors.New("scheduler: fsrs weights must have exactly 21 entries")

// Settings are the per-deck knobs the scheduler honours.
type Settings struct {
	// DesiredRetention is the probability of recall the schedule targets,
	// typically 0.9. Higher means shorter intervals and more reviews.
	DesiredRetention float64
	// Weights is an optional 21-entry FSRS parameter vector. Nil or empty
	// means the library's defaults.
	Weights []float64
}

// Scheduler maps a card's memory state and a rating onto a new memory state.
//
// Both methods return an error, unlike a pure function would: the underlying
// FSRS implementation validates its parameters and can reject a malformed
// state, and swallowing that would mean silently writing a wrong due date.
type Scheduler interface {
	// Preview returns the projected next state for each of the four ratings
	// without committing anything. The UI uses it to label the rating buttons
	// with real intervals.
	Preview(state domain.CardState, now time.Time) (map[domain.Rating]domain.CardState, error)

	// Apply returns the new state for the rating the user actually gave.
	Apply(state domain.CardState, rating domain.Rating, now time.Time) (domain.CardState, error)
}

// Factory builds a Scheduler for a deck's settings. Handlers take a Factory so
// tests can substitute a deterministic scheduler.
type Factory func(Settings) (Scheduler, error)

// SettingsFor extracts scheduler settings from a deck.
func SettingsFor(d *domain.Deck) Settings {
	retention := d.DesiredRetention
	if retention <= 0 || retention >= 1 {
		retention = domain.DefaultDesiredRetention
	}
	return Settings{
		DesiredRetention: retention,
		Weights:          d.FSRSWeights,
	}
}

// ElapsedDays is the whole number of days between a card's last review and now.
// A card never reviewed, or reviewed in the future because of a clock change,
// counts as zero.
func ElapsedDays(lastReview *time.Time, now time.Time) int {
	if lastReview == nil || lastReview.IsZero() {
		return 0
	}
	d := int(now.Sub(*lastReview) / (24 * time.Hour))
	if d < 0 {
		return 0
	}
	return d
}

// ReviewFrom builds the append-only log row for a review, capturing the card's
// state as it was *before* the rating was applied. The caller fills in ID.
func ReviewFrom(before domain.CardState, rating domain.Rating, now time.Time, durationMS int) domain.Review {
	return domain.Review{
		CardID:          before.CardID,
		Rating:          rating,
		State:           before.State,
		Due:             before.Due,
		Stability:       before.Stability,
		Difficulty:      before.Difficulty,
		ElapsedDays:     ElapsedDays(before.LastReview, now),
		LastElapsedDays: before.ElapsedDays,
		ScheduledDays:   before.ScheduledDays,
		ReviewedAt:      now.UTC(),
		DurationMS:      durationMS,
	}
}
