package api

import (
	"fmt"
	"time"

	"github.com/romonzaman/kaart/internal/domain"
)

// formatTime renders a timestamp for the wire: RFC3339 in UTC, always.
func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := formatTime(*t)
	return &s
}

// --- decks ---

type deckResponse struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	NewCardsPerDay   int       `json:"new_cards_per_day"`
	MaxReviewsPerDay int       `json:"max_reviews_per_day"`
	DesiredRetention float64   `json:"desired_retention"`
	FSRSWeights      []float64 `json:"fsrs_weights"`
	CreatedAt        string    `json:"created_at"`
	UpdatedAt        string    `json:"updated_at"`
	ArchivedAt       *string   `json:"archived_at"`
}

func newDeckResponse(d *domain.Deck) deckResponse {
	return deckResponse{
		ID:               d.ID,
		Name:             d.Name,
		Description:      d.Description,
		NewCardsPerDay:   d.NewCardsPerDay,
		MaxReviewsPerDay: d.MaxReviewsPerDay,
		DesiredRetention: d.DesiredRetention,
		FSRSWeights:      d.FSRSWeights,
		CreatedAt:        formatTime(d.CreatedAt),
		UpdatedAt:        formatTime(d.UpdatedAt),
		ArchivedAt:       formatTimePtr(d.ArchivedAt),
	}
}

type deckListResponse struct {
	Decks []deckResponse `json:"decks"`
}

// --- cards ---

type cardResponse struct {
	ID          string   `json:"id"`
	DeckID      string   `json:"deck_id"`
	Front       string   `json:"front"`
	Back        string   `json:"back"`
	Hint        string   `json:"hint"`
	Tags        []string `json:"tags"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	SuspendedAt *string  `json:"suspended_at"`
}

func newCardResponse(c *domain.Card) cardResponse {
	tags := c.Tags
	if tags == nil {
		tags = []string{}
	}
	return cardResponse{
		ID:          c.ID,
		DeckID:      c.DeckID,
		Front:       c.Front,
		Back:        c.Back,
		Hint:        c.Hint,
		Tags:        tags,
		CreatedAt:   formatTime(c.CreatedAt),
		UpdatedAt:   formatTime(c.UpdatedAt),
		SuspendedAt: formatTimePtr(c.SuspendedAt),
	}
}

type cardListResponse struct {
	Cards  []cardResponse `json:"cards"`
	Total  int            `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

// --- scheduling state ---

type cardStateResponse struct {
	CardID        string  `json:"card_id"`
	Due           string  `json:"due"`
	Stability     float64 `json:"stability"`
	Difficulty    float64 `json:"difficulty"`
	ElapsedDays   int     `json:"elapsed_days"`
	ScheduledDays int     `json:"scheduled_days"`
	Reps          int     `json:"reps"`
	Lapses        int     `json:"lapses"`
	State         string  `json:"state"`
	StateCode     int     `json:"state_code"`
	LastReview    *string `json:"last_review"`
}

func newCardStateResponse(st *domain.CardState) cardStateResponse {
	return cardStateResponse{
		CardID:        st.CardID,
		Due:           formatTime(st.Due),
		Stability:     st.Stability,
		Difficulty:    st.Difficulty,
		ElapsedDays:   st.ElapsedDays,
		ScheduledDays: st.ScheduledDays,
		Reps:          st.Reps,
		Lapses:        st.Lapses,
		State:         st.State.String(),
		StateCode:     int(st.State),
		LastReview:    formatTimePtr(st.LastReview),
	}
}

// --- queue ---

// previewResponse is one rating button's projected outcome. Label is the string
// the UI puts on the button, so "1m / 10m / 4d / 9d" needs no client-side
// interval maths and no second round trip.
type previewResponse struct {
	Rating          int    `json:"rating"`
	RatingName      string `json:"rating_name"`
	Due             string `json:"due"`
	IntervalSeconds int64  `json:"interval_seconds"`
	Label           string `json:"label"`
	State           string `json:"state"`
}

type queueItemResponse struct {
	Card     cardResponse      `json:"card"`
	State    cardStateResponse `json:"state"`
	Previews []previewResponse `json:"previews"`
}

type queueResponse struct {
	DeckID string              `json:"deck_id"`
	Now    string              `json:"now"`
	Counts queueCounts         `json:"counts"`
	Items  []queueItemResponse `json:"items"`
}

type queueCounts struct {
	Total    int `json:"total"`
	New      int `json:"new"`
	Learning int `json:"learning"`
	Review   int `json:"review"`
}

// ratingOrder fixes the order previews appear in, so the UI can render the four
// buttons without sorting.
var ratingOrder = []domain.Rating{
	domain.RatingAgain, domain.RatingHard, domain.RatingGood, domain.RatingEasy,
}

// formatInterval renders a duration the way a flashcard app does: coarse,
// single-unit, and never longer than it needs to be.
func formatInterval(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Round(time.Minute)/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Round(time.Hour)/time.Hour))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%.1fmo", d.Hours()/(24*30))
	default:
		return fmt.Sprintf("%.1fy", d.Hours()/(24*365))
	}
}

// --- review ---

type reviewResponse struct {
	CardID     string            `json:"card_id"`
	Rating     int               `json:"rating"`
	RatingName string            `json:"rating_name"`
	ReviewedAt string            `json:"reviewed_at"`
	NextDue    string            `json:"next_due"`
	State      cardStateResponse `json:"state"`
}

// --- stats ---

type dayCountResponse struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type statsResponse struct {
	DeckID                string `json:"deck_id"`
	Now                   string `json:"now"`
	DueNow                int    `json:"due_now"`
	New                   int    `json:"new"`
	Learning              int    `json:"learning"`
	Suspended             int    `json:"suspended"`
	TotalCards            int    `json:"total_cards"`
	ReviewsToday          int    `json:"reviews_today"`
	NewCardsToday         int    `json:"new_cards_today"`
	RemainingNewToday     int    `json:"remaining_new_today"`
	RemainingReviewsToday int    `json:"remaining_reviews_today"`
	// NextDue is when the deck's next card comes up, or null when nothing is
	// scheduled beyond now. The study screen turns it into "next card in 3
	// hours" instead of an unexplained empty session.
	NextDue   *string            `json:"next_due"`
	Histogram []dayCountResponse `json:"histogram"`
}
