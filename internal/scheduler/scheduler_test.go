package scheduler_test

import (
	"errors"
	"testing"
	"time"

	"github.com/romonzaman/kaart/internal/clock"
	"github.com/romonzaman/kaart/internal/domain"
	"github.com/romonzaman/kaart/internal/scheduler"
)

func newScheduler(t *testing.T) *scheduler.FSRS {
	t.Helper()
	s, err := scheduler.New(scheduler.Settings{DesiredRetention: 0.9})
	if err != nil {
		t.Fatalf("building scheduler: %v", err)
	}
	return s
}

func newState(cardID string, now time.Time) domain.CardState {
	return domain.NewCardState(cardID, now)
}

func TestNewRejectsWrongWeightCount(t *testing.T) {
	_, err := scheduler.New(scheduler.Settings{
		DesiredRetention: 0.9,
		Weights:          []float64{1, 2, 3},
	})
	if !errors.Is(err, scheduler.ErrInvalidWeights) {
		t.Fatalf("err = %v, want ErrInvalidWeights", err)
	}
}

func TestNewAcceptsFullWeightVector(t *testing.T) {
	w := make([]float64, domain.FSRSWeightCount)
	for i := range w {
		w[i] = 0.5
	}
	if _, err := scheduler.New(scheduler.Settings{DesiredRetention: 0.9, Weights: w}); err != nil {
		t.Fatalf("New with %d weights: %v", domain.FSRSWeightCount, err)
	}
}

func TestNewCardRatedGoodEntersLearning(t *testing.T) {
	c := clock.NewFake(time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC))
	s := newScheduler(t)

	st := newState("card-1", c.Now())
	if st.State != domain.StateNew {
		t.Fatalf("precondition: state = %v", st.State)
	}

	got, err := s.Apply(st, domain.RatingGood, c.Now())
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if got.State != domain.StateLearning {
		t.Fatalf("state after Good on a new card = %v, want learning", got.State)
	}
	if got.Reps != 1 {
		t.Fatalf("Reps = %d, want 1", got.Reps)
	}
	if got.Lapses != 0 {
		t.Fatalf("Lapses = %d, want 0", got.Lapses)
	}
	if !got.Due.After(c.Now()) {
		t.Fatalf("Due = %v, should be after now = %v", got.Due, c.Now())
	}
	if got.LastReview == nil || !got.LastReview.Equal(c.Now()) {
		t.Fatalf("LastReview = %v, want %v", got.LastReview, c.Now())
	}
	if got.Stability <= 0 {
		t.Fatalf("Stability = %v, want > 0 after a review", got.Stability)
	}
}

func TestAgainFromReviewEntersRelearningAndIncrementsLapses(t *testing.T) {
	c := clock.NewFake(time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC))
	s := newScheduler(t)

	st := graduate(t, s, c, "card-1")
	if st.State != domain.StateReview {
		t.Fatalf("precondition: state = %v, want review", st.State)
	}
	lapsesBefore := st.Lapses

	c.Set(st.Due)
	got, err := s.Apply(st, domain.RatingAgain, c.Now())
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if got.State != domain.StateRelearning {
		t.Fatalf("state after Again from review = %v, want relearning", got.State)
	}
	if got.Lapses != lapsesBefore+1 {
		t.Fatalf("Lapses = %d, want %d", got.Lapses, lapsesBefore+1)
	}
}

func TestIntervalsGrowAcrossRepeatedGood(t *testing.T) {
	c := clock.NewFake(time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC))
	s := newScheduler(t)

	st := graduate(t, s, c, "card-1")

	var prev time.Duration
	for i := 0; i < 6; i++ {
		c.Set(st.Due)
		next, err := s.Apply(st, domain.RatingGood, c.Now())
		if err != nil {
			t.Fatalf("Apply #%d: %v", i, err)
		}
		interval := next.Due.Sub(c.Now())
		if interval <= 0 {
			t.Fatalf("review #%d produced a non-positive interval %v", i, interval)
		}
		if i > 0 && interval <= prev {
			t.Fatalf("review #%d interval %v did not grow past %v", i, interval, prev)
		}
		prev = interval
		st = next
	}

	if st.Stability <= 0 {
		t.Fatalf("Stability = %v after six Good ratings", st.Stability)
	}
}

func TestEasyBeatsGoodFromTheSameState(t *testing.T) {
	c := clock.NewFake(time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC))
	s := newScheduler(t)

	st := graduate(t, s, c, "card-1")
	c.Set(st.Due)

	good, err := s.Apply(st, domain.RatingGood, c.Now())
	if err != nil {
		t.Fatalf("Apply(good): %v", err)
	}
	easy, err := s.Apply(st, domain.RatingEasy, c.Now())
	if err != nil {
		t.Fatalf("Apply(easy): %v", err)
	}
	hard, err := s.Apply(st, domain.RatingHard, c.Now())
	if err != nil {
		t.Fatalf("Apply(hard): %v", err)
	}

	if !easy.Due.After(good.Due) {
		t.Fatalf("easy due %v should be later than good due %v", easy.Due, good.Due)
	}
	if !good.Due.After(hard.Due) {
		t.Fatalf("good due %v should be later than hard due %v", good.Due, hard.Due)
	}
	if easy.Difficulty >= hard.Difficulty {
		t.Fatalf("easy difficulty %v should be below hard difficulty %v",
			easy.Difficulty, hard.Difficulty)
	}
}

func TestPreviewMatchesApplyForEveryRating(t *testing.T) {
	c := clock.NewFake(time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC))
	s := newScheduler(t)

	st := graduate(t, s, c, "card-1")
	c.Set(st.Due)

	preview, err := s.Preview(st, c.Now())
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if len(preview) != 4 {
		t.Fatalf("Preview returned %d entries, want 4", len(preview))
	}

	for _, rating := range []domain.Rating{
		domain.RatingAgain, domain.RatingHard, domain.RatingGood, domain.RatingEasy,
	} {
		projected, ok := preview[rating]
		if !ok {
			t.Fatalf("Preview missing %s", rating)
		}
		applied, err := s.Apply(st, rating, c.Now())
		if err != nil {
			t.Fatalf("Apply(%s): %v", rating, err)
		}
		if !projected.Due.Equal(applied.Due) {
			t.Fatalf("%s: preview due %v != applied due %v", rating, projected.Due, applied.Due)
		}
		if projected.State != applied.State {
			t.Fatalf("%s: preview state %v != applied state %v", rating, projected.State, applied.State)
		}
	}
}

func TestApplyRejectsInvalidRating(t *testing.T) {
	c := clock.NewFake(time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC))
	s := newScheduler(t)

	if _, err := s.Apply(newState("card-1", c.Now()), domain.Rating(0), c.Now()); err == nil {
		t.Fatal("Apply with rating 0 should fail")
	}
	if _, err := s.Apply(newState("card-1", c.Now()), domain.Rating(9), c.Now()); err == nil {
		t.Fatal("Apply with rating 9 should fail")
	}
}

// TestFixedSequenceIsStable replays a fixed rating sequence and asserts the
// exact final state. It exists so a future refactor — a library bump, a change
// to the domain/library conversion, a different default parameter set — cannot
// silently alter everyone's schedule. If this test fails, scheduling changed:
// decide whether that was intended before updating the expectations.
func TestFixedSequenceIsStable(t *testing.T) {
	c := clock.NewFake(time.Date(2026, 1, 1, 6, 0, 0, 0, time.UTC))
	s := newScheduler(t)

	ratings := []domain.Rating{
		domain.RatingGood, domain.RatingGood, domain.RatingGood, domain.RatingEasy,
		domain.RatingGood, domain.RatingHard, domain.RatingGood, domain.RatingAgain,
		domain.RatingGood, domain.RatingGood, domain.RatingEasy, domain.RatingGood,
		domain.RatingHard, domain.RatingGood, domain.RatingGood, domain.RatingAgain,
		domain.RatingHard, domain.RatingGood, domain.RatingGood, domain.RatingEasy,
	}

	st := newState("card-1", c.Now())
	for i, r := range ratings {
		next, err := s.Apply(st, r, c.Now())
		if err != nil {
			t.Fatalf("Apply #%d (%s): %v", i, r, err)
		}
		st = next
		c.Set(st.Due) // study exactly when the card comes due
	}

	// Always-true invariants of the sequence.
	if st.Reps != len(ratings) {
		t.Fatalf("Reps = %d, want %d", st.Reps, len(ratings))
	}
	if st.Lapses < 1 || st.Lapses > 2 {
		t.Fatalf("Lapses = %d, want 1 or 2 (the sequence contains two Again ratings)", st.Lapses)
	}
	if st.State != domain.StateReview {
		t.Fatalf("final state = %v, want review", st.State)
	}
	if st.Stability <= 0 || st.Difficulty <= 0 {
		t.Fatalf("final memory looks unset: stability=%v difficulty=%v", st.Stability, st.Difficulty)
	}

	// Golden values. goldenStability and goldenDifficulty are zero until you
	// pin them: run `go test ./internal/scheduler -run TestFixedSequenceIsStable -v`
	// once, copy the logged values in here, and this test becomes a hard lock on
	// scheduling behaviour. Regenerate deliberately, never reflexively — a
	// changed value means every existing user's schedule just moved.
	t.Logf("final state after the fixed sequence: stability=%.9f difficulty=%.9f due=%s",
		st.Stability, st.Difficulty, st.Due.Format(time.RFC3339))

	const (
		goldenStability  = 0.0
		goldenDifficulty = 0.0
	)
	if goldenStability != 0 && !approx(st.Stability, goldenStability) {
		t.Fatalf("Stability = %.9f, want %.9f", st.Stability, goldenStability)
	}
	if goldenDifficulty != 0 && !approx(st.Difficulty, goldenDifficulty) {
		t.Fatalf("Difficulty = %.9f, want %.9f", st.Difficulty, goldenDifficulty)
	}

	// Determinism: the identical replay must land on the identical state. This
	// holds regardless of whether the golden values above have been pinned.
	c2 := clock.NewFake(time.Date(2026, 1, 1, 6, 0, 0, 0, time.UTC))
	s2 := newScheduler(t)
	st2 := newState("card-1", c2.Now())
	for i, r := range ratings {
		next, err := s2.Apply(st2, r, c2.Now())
		if err != nil {
			t.Fatalf("replay Apply #%d: %v", i, err)
		}
		st2 = next
		c2.Set(st2.Due)
	}
	if !st2.Due.Equal(st.Due) || st2.Stability != st.Stability || st2.Difficulty != st.Difficulty {
		t.Fatalf("scheduling is not deterministic:\n first: %+v\nsecond: %+v", st, st2)
	}
}

func TestElapsedDays(t *testing.T) {
	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	past := now.Add(-50 * time.Hour)
	future := now.Add(3 * time.Hour)

	tests := []struct {
		name string
		last *time.Time
		want int
	}{
		{"never reviewed", nil, 0},
		{"two days and change", &past, 2},
		{"clock moved backwards", &future, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scheduler.ElapsedDays(tt.last, now); got != tt.want {
				t.Fatalf("ElapsedDays = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestReviewFromCapturesPreReviewState(t *testing.T) {
	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	last := now.Add(-72 * time.Hour)

	before := domain.CardState{
		CardID: "card-1", Due: now.Add(-time.Hour),
		Stability: 4.5, Difficulty: 6.25,
		ElapsedDays: 1, ScheduledDays: 3, Reps: 4, Lapses: 1,
		State: domain.StateReview, LastReview: &last,
	}

	rv := scheduler.ReviewFrom(before, domain.RatingHard, now, 3400)

	if rv.State != domain.StateReview || rv.Stability != 4.5 || rv.Difficulty != 6.25 {
		t.Fatalf("review did not capture the pre-review memory: %+v", rv)
	}
	if rv.ScheduledDays != 3 {
		t.Fatalf("ScheduledDays = %d, want the pre-review 3", rv.ScheduledDays)
	}
	if rv.ElapsedDays != 3 {
		t.Fatalf("ElapsedDays = %d, want 3 (72h since last review)", rv.ElapsedDays)
	}
	if rv.LastElapsedDays != 1 {
		t.Fatalf("LastElapsedDays = %d, want the pre-review ElapsedDays 1", rv.LastElapsedDays)
	}
	if rv.Rating != domain.RatingHard || rv.DurationMS != 3400 {
		t.Fatalf("rating/duration not recorded: %+v", rv)
	}
	if !rv.ReviewedAt.Equal(now) {
		t.Fatalf("ReviewedAt = %v, want %v", rv.ReviewedAt, now)
	}
}

func TestSettingsForFallsBackOnNonsenseRetention(t *testing.T) {
	tests := []struct {
		in   float64
		want float64
	}{
		{0.85, 0.85},
		{0, domain.DefaultDesiredRetention},
		{1, domain.DefaultDesiredRetention},
		{-3, domain.DefaultDesiredRetention},
	}
	for _, tt := range tests {
		got := scheduler.SettingsFor(&domain.Deck{DesiredRetention: tt.in})
		if got.DesiredRetention != tt.want {
			t.Fatalf("SettingsFor(%v).DesiredRetention = %v, want %v", tt.in, got.DesiredRetention, tt.want)
		}
	}
}

// graduate drives a fresh card into the Review state with Good ratings, so
// tests that care about review-state behaviour do not each repeat the setup.
func graduate(t *testing.T, s *scheduler.FSRS, c *clock.Fake, cardID string) domain.CardState {
	t.Helper()

	st := domain.NewCardState(cardID, c.Now())
	for i := 0; i < 12; i++ {
		next, err := s.Apply(st, domain.RatingGood, c.Now())
		if err != nil {
			t.Fatalf("graduating card, Apply #%d: %v", i, err)
		}
		st = next
		if st.State == domain.StateReview {
			return st
		}
		c.Set(st.Due)
	}
	t.Fatalf("card did not reach review state after 12 Good ratings (state=%v)", st.State)
	return st
}

func approx(a, b float64) bool {
	const eps = 1e-6
	d := a - b
	return d < eps && d > -eps
}
