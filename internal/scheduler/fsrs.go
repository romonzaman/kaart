package scheduler

import (
	"fmt"
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v4"

	"github.com/romonzaman/kaart/internal/domain"
)

// FSRS is the production Scheduler, backed by go-fsrs/v4.
//
// This file is the single place where domain types meet the library's types.
// Nothing else in Kaart imports go-fsrs, so replacing the algorithm means
// rewriting this file and nothing else.
type FSRS struct {
	engine *fsrs.FSRS
}

var _ Scheduler = (*FSRS)(nil)

// New builds an FSRS scheduler for the given deck settings.
//
// Fuzz is disabled. FSRS normally jitters intervals by a few percent to stop
// cards introduced together from clumping forever, but a non-deterministic
// scheduler cannot be regression-tested, and the Preview intervals shown on the
// rating buttons would not match what Apply then writes. Re-enable it behind a
// deck setting if clumping turns out to matter in practice.
func New(s Settings) (*FSRS, error) {
	params := fsrs.DefaultParam()

	if s.DesiredRetention > 0 && s.DesiredRetention < 1 {
		params.RequestRetention = s.DesiredRetention
	}
	if len(s.Weights) > 0 {
		if len(s.Weights) != domain.FSRSWeightCount {
			return nil, fmt.Errorf("building scheduler: got %d weights: %w",
				len(s.Weights), ErrInvalidWeights)
		}
		var w fsrs.Weights
		copy(w[:], s.Weights)
		params.W = w
	}
	params.EnableFuzz = false

	return &FSRS{engine: fsrs.NewFSRS(params)}, nil
}

// NewFactory returns a Factory producing production schedulers.
func NewFactory() Factory {
	return func(s Settings) (Scheduler, error) { return New(s) }
}

// Preview projects the next state for each rating without committing.
func (f *FSRS) Preview(state domain.CardState, now time.Time) (map[domain.Rating]domain.CardState, error) {
	log, err := f.engine.Repeat(toFSRS(state), now.UTC())
	if err != nil {
		return nil, fmt.Errorf("previewing card %s: %w", state.CardID, err)
	}

	out := make(map[domain.Rating]domain.CardState, 4)
	for rating, grade := range gradeByRating {
		info, ok := log[grade]
		if !ok {
			return nil, fmt.Errorf("previewing card %s: fsrs returned no projection for %s",
				state.CardID, rating)
		}
		out[rating] = fromFSRS(state, info.Card, now)
	}
	return out, nil
}

// Apply returns the state the card moves to for the rating actually given.
func (f *FSRS) Apply(state domain.CardState, rating domain.Rating, now time.Time) (domain.CardState, error) {
	grade, ok := gradeByRating[rating]
	if !ok {
		return domain.CardState{}, fmt.Errorf("applying review to card %s: invalid rating %d",
			state.CardID, int(rating))
	}

	info, err := f.engine.Next(toFSRS(state), now.UTC(), grade)
	if err != nil {
		return domain.CardState{}, fmt.Errorf("applying review to card %s: %w", state.CardID, err)
	}
	return fromFSRS(state, info.Card, now), nil
}

// gradeByRating maps Kaart's ratings onto the library's. The numeric values
// agree today, but going through an explicit table means a future library
// renumbering fails loudly here instead of silently mis-scheduling every card.
var gradeByRating = map[domain.Rating]fsrs.Rating{
	domain.RatingAgain: fsrs.Again,
	domain.RatingHard:  fsrs.Hard,
	domain.RatingGood:  fsrs.Good,
	domain.RatingEasy:  fsrs.Easy,
}

// toFSRS converts Kaart's card state into the library's card type.
func toFSRS(st domain.CardState) fsrs.Card {
	c := fsrs.Card{
		Due:            st.Due.UTC(),
		Stability:      st.Stability,
		Difficulty:     st.Difficulty,
		ScheduledDays:  uint64(max(st.ScheduledDays, 0)),
		Reps:           uint64(max(st.Reps, 0)),
		Lapses:         uint64(max(st.Lapses, 0)),
		State:          fsrs.State(st.State),
		RemainingSteps: st.RemainingSteps,
	}
	if st.LastReview != nil {
		c.LastReview = st.LastReview.UTC()
	}
	return c
}

// fromFSRS converts the library's post-review card back into Kaart's state.
//
// ElapsedDays is computed here rather than read from the library: go-fsrs/v4's
// Card carries no elapsed-days field, but Kaart persists it because the review
// log needs each row's elapsed interval to be re-optimisable later.
func fromFSRS(before domain.CardState, c fsrs.Card, now time.Time) domain.CardState {
	reviewedAt := now.UTC()
	if !c.LastReview.IsZero() {
		reviewedAt = c.LastReview.UTC()
	}

	return domain.CardState{
		CardID:         before.CardID,
		Due:            c.Due.UTC(),
		Stability:      c.Stability,
		Difficulty:     c.Difficulty,
		ElapsedDays:    ElapsedDays(before.LastReview, now),
		ScheduledDays:  int(c.ScheduledDays),
		Reps:           int(c.Reps),
		Lapses:         int(c.Lapses),
		State:          domain.State(c.State),
		RemainingSteps: c.RemainingSteps,
		LastReview:     &reviewedAt,
	}
}
