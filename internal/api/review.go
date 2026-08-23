package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/romonzaman/kaart/internal/domain"
	"github.com/romonzaman/kaart/internal/scheduler"
	"github.com/romonzaman/kaart/internal/store"
)

type reviewRequest struct {
	Rating     int  `json:"rating"`
	DurationMS *int `json:"duration_ms"`
}

// maxDurationMS caps a self-reported think time at one hour. Anything longer is
// a browser tab left open overnight, not a review, and would poison any future
// analysis of the log.
const maxDurationMS = 60 * 60 * 1000

// handleQueue builds a study session.
//
// The response carries each card's four projected intervals alongside the card
// itself. That is deliberate: the review screen needs to label the rating
// buttons the instant it renders, and a second request per card would put a
// network round trip inside the tightest loop in the product.
func (s *Server) handleQueue(w http.ResponseWriter, r *http.Request) error {
	deckID, err := pathValue(r, "deckID")
	if err != nil {
		return err
	}
	limit, err := intParam(r, "limit", 50, 1, 500)
	if err != nil {
		return err
	}

	deck, err := s.store.GetDeck(r.Context(), deckID)
	if err != nil {
		return storeError(err, "deck")
	}

	now := s.clock.Now()
	dayStart, dayEnd := dayBounds(now)

	totals, err := s.store.ReviewTotals(r.Context(), deckID, dayStart, dayEnd)
	if err != nil {
		return internalError(err)
	}

	items, err := s.store.DueQueue(r.Context(), deckID, store.QueueParams{
		Now:         now,
		Limit:       limit,
		NewLimit:    remaining(deck.NewCardsPerDay, totals.NewCards),
		ReviewLimit: remaining(deck.MaxReviewsPerDay, totals.Reviews),
	})
	if err != nil {
		return internalError(err)
	}

	sched, err := s.schedulerFor(deck)
	if err != nil {
		return err
	}

	resp := queueResponse{
		DeckID: deckID,
		Now:    formatTime(now),
		Items:  make([]queueItemResponse, 0, len(items)),
	}

	for i := range items {
		item := items[i]

		previews, err := sched.Preview(item.State, now)
		if err != nil {
			return internalError(err)
		}

		out := queueItemResponse{
			Card:     newCardResponse(&item.Card),
			State:    newCardStateResponse(&item.State),
			Previews: make([]previewResponse, 0, len(ratingOrder)),
		}
		for _, rating := range ratingOrder {
			projected, ok := previews[rating]
			if !ok {
				return internalError(fmt.Errorf(
					"scheduler returned no %s projection for card %s", rating, item.Card.ID))
			}
			out.Previews = append(out.Previews, previewResponse{
				Rating:          int(rating),
				RatingName:      rating.String(),
				Due:             formatTime(projected.Due),
				IntervalSeconds: int64(projected.Due.Sub(now) / time.Second),
				Label:           formatInterval(projected.Due.Sub(now)),
				State:           projected.State.String(),
			})
		}

		switch item.State.State {
		case domain.StateNew:
			resp.Counts.New++
		case domain.StateLearning, domain.StateRelearning:
			resp.Counts.Learning++
		case domain.StateReview:
			resp.Counts.Review++
		}
		resp.Counts.Total++

		resp.Items = append(resp.Items, out)
	}

	writeJSON(w, r, s.logger, http.StatusOK, resp)
	return nil
}

// handleReview records one rating.
//
// The new state and the log row are written in a single transaction. The log
// row captures the state as it was *before* the rating, because that is what a
// future FSRS optimiser needs: the memory the algorithm predicted from, paired
// with what the user actually did.
func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) error {
	cardID, err := pathValue(r, "cardID")
	if err != nil {
		return err
	}

	var req reviewRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}

	rating := domain.Rating(req.Rating)
	if !rating.Valid() {
		return badRequest("rating must be 1 (again), 2 (hard), 3 (good), or 4 (easy)")
	}

	durationMS := 0
	if req.DurationMS != nil {
		durationMS = *req.DurationMS
		if durationMS < 0 || durationMS > maxDurationMS {
			return badRequest("duration_ms must be between 0 and %d", maxDurationMS)
		}
	}

	card, err := s.store.GetCard(r.Context(), cardID)
	if err != nil {
		return storeError(err, "card")
	}
	if card.Suspended() {
		return conflictError("card is suspended and cannot be reviewed")
	}

	deck, err := s.store.GetDeck(r.Context(), card.DeckID)
	if err != nil {
		return storeError(err, "deck")
	}

	before, err := s.store.GetCardState(r.Context(), cardID)
	if err != nil {
		return storeError(err, "card state")
	}

	sched, err := s.schedulerFor(deck)
	if err != nil {
		return err
	}

	now := s.clock.Now()
	after, err := sched.Apply(*before, rating, now)
	if err != nil {
		return internalError(err)
	}

	logRow := scheduler.ReviewFrom(*before, rating, now, durationMS)

	if err := s.store.ApplyReview(r.Context(), &after, &logRow); err != nil {
		return storeError(err, "card")
	}

	writeJSON(w, r, s.logger, http.StatusOK, reviewResponse{
		CardID:     cardID,
		Rating:     int(rating),
		RatingName: rating.String(),
		ReviewedAt: formatTime(now),
		NextDue:    formatTime(after.Due),
		State:      newCardStateResponse(&after),
	})
	return nil
}

// handleStats reports what is due and what has been done.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) error {
	deckID, err := pathValue(r, "deckID")
	if err != nil {
		return err
	}
	days, err := intParam(r, "days", 30, 1, 365)
	if err != nil {
		return err
	}

	deck, err := s.store.GetDeck(r.Context(), deckID)
	if err != nil {
		return storeError(err, "deck")
	}

	now := s.clock.Now()
	dayStart, dayEnd := dayBounds(now)

	counts, err := s.store.DeckCounts(r.Context(), deckID, now)
	if err != nil {
		return internalError(err)
	}
	totals, err := s.store.ReviewTotals(r.Context(), deckID, dayStart, dayEnd)
	if err != nil {
		return internalError(err)
	}
	hist, err := s.store.ReviewHistogram(r.Context(), deckID, dayEnd.AddDate(0, 0, -days), dayEnd)
	if err != nil {
		return internalError(err)
	}
	nextDue, err := s.store.NextDue(r.Context(), deckID, now)
	if err != nil {
		return internalError(err)
	}

	buckets := make([]dayCountResponse, 0, len(hist))
	for _, h := range hist {
		buckets = append(buckets, dayCountResponse{Date: h.Date, Count: h.Count})
	}

	writeJSON(w, r, s.logger, http.StatusOK, statsResponse{
		DeckID:                deckID,
		Now:                   formatTime(now),
		DueNow:                counts.Due,
		New:                   counts.New,
		Learning:              counts.Learning,
		Suspended:             counts.Suspended,
		TotalCards:            counts.Total,
		ReviewsToday:          totals.Reviews,
		NewCardsToday:         totals.NewCards,
		RemainingNewToday:     remaining(deck.NewCardsPerDay, totals.NewCards),
		RemainingReviewsToday: remaining(deck.MaxReviewsPerDay, totals.Reviews),
		NextDue:               formatTimePtr(nextDue),
		Histogram:             buckets,
	})
	return nil
}

// remaining is a daily allowance minus what has been used, floored at zero.
func remaining(allowance, used int) int {
	if n := allowance - used; n > 0 {
		return n
	}
	return 0
}
