package domain_test

import (
	"testing"
	"time"

	"github.com/romonzaman/kaart/internal/domain"
)

func TestRatingString(t *testing.T) {
	tests := []struct {
		name string
		in   domain.Rating
		want string
	}{
		{"again", domain.RatingAgain, "again"},
		{"hard", domain.RatingHard, "hard"},
		{"good", domain.RatingGood, "good"},
		{"easy", domain.RatingEasy, "easy"},
		{"zero", domain.Rating(0), "unknown"},
		{"five", domain.Rating(5), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRatingValid(t *testing.T) {
	tests := []struct {
		in   domain.Rating
		want bool
	}{
		{0, false}, {1, true}, {2, true}, {3, true}, {4, true}, {5, false}, {-1, false},
	}
	for _, tt := range tests {
		if got := tt.in.Valid(); got != tt.want {
			t.Fatalf("Rating(%d).Valid() = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestStateString(t *testing.T) {
	tests := []struct {
		in   domain.State
		want string
	}{
		{domain.StateNew, "new"},
		{domain.StateLearning, "learning"},
		{domain.StateReview, "review"},
		{domain.StateRelearning, "relearning"},
		{domain.State(9), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.in.String(); got != tt.want {
			t.Fatalf("State(%d).String() = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestStateValid(t *testing.T) {
	for s := domain.State(0); s <= 3; s++ {
		if !s.Valid() {
			t.Fatalf("State(%d) should be valid", s)
		}
	}
	if domain.State(4).Valid() || domain.State(-1).Valid() {
		t.Fatal("out-of-range states should be invalid")
	}
}

func TestNewCardState(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	st := domain.NewCardState("card-1", now)

	if st.CardID != "card-1" {
		t.Fatalf("CardID = %q", st.CardID)
	}
	if !st.Due.Equal(now) {
		t.Fatalf("Due = %v, want %v", st.Due, now)
	}
	if st.State != domain.StateNew {
		t.Fatalf("State = %v, want new", st.State)
	}
	if st.LastReview != nil {
		t.Fatal("LastReview should be nil for a new card")
	}
	if st.Reps != 0 || st.Lapses != 0 || st.Stability != 0 || st.Difficulty != 0 {
		t.Fatal("a new card should have no accumulated memory")
	}
}

func TestArchivedAndSuspended(t *testing.T) {
	now := time.Now().UTC()

	d := &domain.Deck{}
	if d.Archived() {
		t.Fatal("deck with nil ArchivedAt should not be archived")
	}
	d.ArchivedAt = &now
	if !d.Archived() {
		t.Fatal("deck with ArchivedAt should be archived")
	}

	c := &domain.Card{}
	if c.Suspended() {
		t.Fatal("card with nil SuspendedAt should not be suspended")
	}
	c.SuspendedAt = &now
	if !c.Suspended() {
		t.Fatal("card with SuspendedAt should be suspended")
	}
}
