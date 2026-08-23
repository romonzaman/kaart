package api

import (
	"net/http"
	"time"

	"github.com/romonzaman/kaart/internal/domain"
	"github.com/romonzaman/kaart/internal/store"
)

type createCardRequest struct {
	Front string   `json:"front"`
	Back  string   `json:"back"`
	Hint  string   `json:"hint"`
	Tags  []string `json:"tags"`
}

type updateCardRequest struct {
	Front *string  `json:"front"`
	Back  *string  `json:"back"`
	Hint  *string  `json:"hint"`
	Tags  []string `json:"tags"`
}

func (s *Server) handleListCards(w http.ResponseWriter, r *http.Request) error {
	deckID, err := pathValue(r, "deckID")
	if err != nil {
		return err
	}
	limit, err := intParam(r, "limit", 50, 1, 500)
	if err != nil {
		return err
	}
	offset, err := intParam(r, "offset", 0, 0, 1<<30)
	if err != nil {
		return err
	}

	if _, err := s.store.GetDeck(r.Context(), deckID); err != nil {
		return storeError(err, "deck")
	}

	cards, total, err := s.store.ListCards(r.Context(), deckID, store.CardFilter{
		Query:  r.URL.Query().Get("q"),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return internalError(err)
	}

	out := make([]cardResponse, 0, len(cards))
	for _, c := range cards {
		out = append(out, newCardResponse(c))
	}
	writeJSON(w, r, s.logger, http.StatusOK, cardListResponse{
		Cards: out, Total: total, Limit: limit, Offset: offset,
	})
	return nil
}

// handleCreateCard creates the card and its initial New state in one
// transaction, so a card can never exist without a schedule.
func (s *Server) handleCreateCard(w http.ResponseWriter, r *http.Request) error {
	deckID, err := pathValue(r, "deckID")
	if err != nil {
		return err
	}

	var req createCardRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}

	front, err := validateCardText("front", req.Front)
	if err != nil {
		return err
	}
	back, err := validateCardText("back", req.Back)
	if err != nil {
		return err
	}
	hint, err := validateHint(req.Hint)
	if err != nil {
		return err
	}
	tags, err := validateTags(req.Tags)
	if err != nil {
		return err
	}

	id, err := newID()
	if err != nil {
		return internalError(err)
	}

	now := s.clock.Now()
	c := &domain.Card{
		ID:        id,
		DeckID:    deckID,
		Front:     front,
		Back:      back,
		Hint:      hint,
		Tags:      tags,
		CreatedAt: now,
		UpdatedAt: now,
	}
	st := domain.NewCardState(id, now)

	if err := s.store.CreateCard(r.Context(), c, &st); err != nil {
		return storeError(err, "deck")
	}
	writeJSON(w, r, s.logger, http.StatusCreated, newCardResponse(c))
	return nil
}

func (s *Server) handleGetCard(w http.ResponseWriter, r *http.Request) error {
	id, err := pathValue(r, "cardID")
	if err != nil {
		return err
	}
	c, err := s.store.GetCard(r.Context(), id)
	if err != nil {
		return storeError(err, "card")
	}
	writeJSON(w, r, s.logger, http.StatusOK, newCardResponse(c))
	return nil
}

func (s *Server) handleUpdateCard(w http.ResponseWriter, r *http.Request) error {
	id, err := pathValue(r, "cardID")
	if err != nil {
		return err
	}

	var req updateCardRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}

	c, err := s.store.GetCard(r.Context(), id)
	if err != nil {
		return storeError(err, "card")
	}

	if req.Front != nil {
		front, err := validateCardText("front", *req.Front)
		if err != nil {
			return err
		}
		c.Front = front
	}
	if req.Back != nil {
		back, err := validateCardText("back", *req.Back)
		if err != nil {
			return err
		}
		c.Back = back
	}
	if req.Hint != nil {
		hint, err := validateHint(*req.Hint)
		if err != nil {
			return err
		}
		c.Hint = hint
	}
	if req.Tags != nil {
		tags, err := validateTags(req.Tags)
		if err != nil {
			return err
		}
		c.Tags = tags
	}

	c.UpdatedAt = s.clock.Now()

	if err := s.store.UpdateCard(r.Context(), c); err != nil {
		return storeError(err, "card")
	}
	writeJSON(w, r, s.logger, http.StatusOK, newCardResponse(c))
	return nil
}

func (s *Server) handleDeleteCard(w http.ResponseWriter, r *http.Request) error {
	id, err := pathValue(r, "cardID")
	if err != nil {
		return err
	}
	if err := s.store.DeleteCard(r.Context(), id); err != nil {
		return storeError(err, "card")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *Server) handleSuspend(w http.ResponseWriter, r *http.Request) error {
	at := s.clock.Now()
	return s.setSuspended(w, r, &at)
}

func (s *Server) handleUnsuspend(w http.ResponseWriter, r *http.Request) error {
	return s.setSuspended(w, r, nil)
}

func (s *Server) setSuspended(w http.ResponseWriter, r *http.Request, at *time.Time) error {
	id, err := pathValue(r, "cardID")
	if err != nil {
		return err
	}
	if err := s.store.SetCardSuspended(r.Context(), id, at); err != nil {
		return storeError(err, "card")
	}
	c, err := s.store.GetCard(r.Context(), id)
	if err != nil {
		return storeError(err, "card")
	}
	writeJSON(w, r, s.logger, http.StatusOK, newCardResponse(c))
	return nil
}
