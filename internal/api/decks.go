package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/romonzaman/kaart/internal/domain"
	"github.com/romonzaman/kaart/internal/store"
)

type createDeckRequest struct {
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	NewCardsPerDay   *int      `json:"new_cards_per_day"`
	MaxReviewsPerDay *int      `json:"max_reviews_per_day"`
	DesiredRetention *float64  `json:"desired_retention"`
	FSRSWeights      []float64 `json:"fsrs_weights"`
}

// updateDeckRequest uses pointers throughout so an omitted field is
// distinguishable from a field explicitly set to its zero value.
type updateDeckRequest struct {
	Name             *string   `json:"name"`
	Description      *string   `json:"description"`
	NewCardsPerDay   *int      `json:"new_cards_per_day"`
	MaxReviewsPerDay *int      `json:"max_reviews_per_day"`
	DesiredRetention *float64  `json:"desired_retention"`
	FSRSWeights      []float64 `json:"fsrs_weights"`
	Archived         *bool     `json:"archived"`
}

func (s *Server) handleListDecks(w http.ResponseWriter, r *http.Request) error {
	includeArchived := strings.EqualFold(r.URL.Query().Get("include_archived"), "true")

	decks, err := s.store.ListDecks(r.Context(), store.DeckFilter{IncludeArchived: includeArchived})
	if err != nil {
		return internalError(err)
	}

	out := make([]deckResponse, 0, len(decks))
	for _, d := range decks {
		out = append(out, newDeckResponse(d))
	}
	writeJSON(w, r, s.logger, http.StatusOK, deckListResponse{Decks: out})
	return nil
}

func (s *Server) handleCreateDeck(w http.ResponseWriter, r *http.Request) error {
	var req createDeckRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}

	name, err := validateDeckName(req.Name)
	if err != nil {
		return err
	}
	desc, err := validateDeckDescription(req.Description)
	if err != nil {
		return err
	}

	now := s.clock.Now()
	d := &domain.Deck{
		Name:             name,
		Description:      desc,
		NewCardsPerDay:   domain.DefaultNewCardsPerDay,
		MaxReviewsPerDay: domain.DefaultMaxReviewsPerDay,
		DesiredRetention: domain.DefaultDesiredRetention,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	if req.NewCardsPerDay != nil {
		if err := validateNewCardsPerDay(*req.NewCardsPerDay); err != nil {
			return err
		}
		d.NewCardsPerDay = *req.NewCardsPerDay
	}
	if req.MaxReviewsPerDay != nil {
		if err := validateMaxReviewsPerDay(*req.MaxReviewsPerDay); err != nil {
			return err
		}
		d.MaxReviewsPerDay = *req.MaxReviewsPerDay
	}
	if req.DesiredRetention != nil {
		if err := validateDesiredRetention(*req.DesiredRetention); err != nil {
			return err
		}
		d.DesiredRetention = *req.DesiredRetention
	}
	if req.FSRSWeights != nil {
		if err := validateWeights(req.FSRSWeights); err != nil {
			return err
		}
		d.FSRSWeights = req.FSRSWeights
	}

	id, err := newID()
	if err != nil {
		return internalError(err)
	}
	d.ID = id

	if err := s.store.CreateDeck(r.Context(), d); err != nil {
		return storeError(err, "deck")
	}
	writeJSON(w, r, s.logger, http.StatusCreated, newDeckResponse(d))
	return nil
}

func (s *Server) handleGetDeck(w http.ResponseWriter, r *http.Request) error {
	id, err := pathValue(r, "deckID")
	if err != nil {
		return err
	}
	d, err := s.store.GetDeck(r.Context(), id)
	if err != nil {
		return storeError(err, "deck")
	}
	writeJSON(w, r, s.logger, http.StatusOK, newDeckResponse(d))
	return nil
}

func (s *Server) handleUpdateDeck(w http.ResponseWriter, r *http.Request) error {
	id, err := pathValue(r, "deckID")
	if err != nil {
		return err
	}

	var req updateDeckRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return err
	}

	d, err := s.store.GetDeck(r.Context(), id)
	if err != nil {
		return storeError(err, "deck")
	}

	if req.Name != nil {
		name, err := validateDeckName(*req.Name)
		if err != nil {
			return err
		}
		d.Name = name
	}
	if req.Description != nil {
		desc, err := validateDeckDescription(*req.Description)
		if err != nil {
			return err
		}
		d.Description = desc
	}
	if req.NewCardsPerDay != nil {
		if err := validateNewCardsPerDay(*req.NewCardsPerDay); err != nil {
			return err
		}
		d.NewCardsPerDay = *req.NewCardsPerDay
	}
	if req.MaxReviewsPerDay != nil {
		if err := validateMaxReviewsPerDay(*req.MaxReviewsPerDay); err != nil {
			return err
		}
		d.MaxReviewsPerDay = *req.MaxReviewsPerDay
	}
	if req.DesiredRetention != nil {
		if err := validateDesiredRetention(*req.DesiredRetention); err != nil {
			return err
		}
		d.DesiredRetention = *req.DesiredRetention
	}
	if req.FSRSWeights != nil {
		if err := validateWeights(req.FSRSWeights); err != nil {
			return err
		}
		// An explicit empty array clears the override back to library defaults.
		if len(req.FSRSWeights) == 0 {
			d.FSRSWeights = nil
		} else {
			d.FSRSWeights = req.FSRSWeights
		}
	}

	now := s.clock.Now()
	if req.Archived != nil {
		if *req.Archived {
			if d.ArchivedAt == nil {
				at := now
				d.ArchivedAt = &at
			}
		} else {
			d.ArchivedAt = nil
		}
	}
	d.UpdatedAt = now

	if err := s.store.UpdateDeck(r.Context(), d); err != nil {
		return storeError(err, "deck")
	}
	writeJSON(w, r, s.logger, http.StatusOK, newDeckResponse(d))
	return nil
}

func (s *Server) handleDeleteDeck(w http.ResponseWriter, r *http.Request) error {
	id, err := pathValue(r, "deckID")
	if err != nil {
		return err
	}
	if err := s.store.DeleteDeck(r.Context(), id); err != nil {
		return storeError(err, "deck")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// newID returns a UUIDv7. Time-ordered IDs keep SQLite's B-tree inserts
// appending at the right edge instead of scattering across the index.
func newID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generating id: %w", err)
	}
	return id.String(), nil
}
