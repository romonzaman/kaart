// Package api is Kaart's HTTP layer: routing, request decoding, validation,
// and response shaping. It holds no SQL and no scheduling maths — those live in
// store and scheduler respectively.
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/romonzaman/kaart/internal/clock"
	"github.com/romonzaman/kaart/internal/domain"
	"github.com/romonzaman/kaart/internal/scheduler"
	"github.com/romonzaman/kaart/internal/store"
)

// Config assembles a Server.
type Config struct {
	Store store.Store
	Clock clock.Clock
	// Scheduler builds a scheduler for a deck's settings. Nil means the
	// production FSRS factory.
	Scheduler scheduler.Factory
	Logger    *slog.Logger
	// CORSOrigins are the browser origins allowed to call the API. Empty
	// disables CORS.
	CORSOrigins []string
	// Version is reported by /healthz.
	Version string
}

// Server is the HTTP handler for the whole API.
type Server struct {
	store       store.Store
	clock       clock.Clock
	newSchedule scheduler.Factory
	logger      *slog.Logger
	version     string
	handler     http.Handler
}

// New builds a Server. It never returns nil.
func New(cfg Config) *Server {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	clk := cfg.Clock
	if clk == nil {
		clk = clock.New()
	}
	factory := cfg.Scheduler
	if factory == nil {
		factory = scheduler.NewFactory()
	}
	version := cfg.Version
	if version == "" {
		version = "dev"
	}

	s := &Server{
		store:       cfg.Store,
		clock:       clk,
		newSchedule: factory,
		logger:      logger,
		version:     version,
	}
	s.handler = chain(s.routes(),
		withRequestID,
		withLogging(logger),
		withRecovery(logger),
		withCORS(cfg.CORSOrigins),
	)
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /healthz", s.h(s.handleHealth))

	mux.Handle("GET /api/v1/decks", s.h(s.handleListDecks))
	mux.Handle("POST /api/v1/decks", s.h(s.handleCreateDeck))
	mux.Handle("GET /api/v1/decks/{deckID}", s.h(s.handleGetDeck))
	mux.Handle("PATCH /api/v1/decks/{deckID}", s.h(s.handleUpdateDeck))
	mux.Handle("DELETE /api/v1/decks/{deckID}", s.h(s.handleDeleteDeck))

	mux.Handle("GET /api/v1/decks/{deckID}/cards", s.h(s.handleListCards))
	mux.Handle("POST /api/v1/decks/{deckID}/cards", s.h(s.handleCreateCard))
	mux.Handle("GET /api/v1/decks/{deckID}/queue", s.h(s.handleQueue))
	mux.Handle("GET /api/v1/decks/{deckID}/stats", s.h(s.handleStats))

	mux.Handle("GET /api/v1/cards/{cardID}", s.h(s.handleGetCard))
	mux.Handle("PATCH /api/v1/cards/{cardID}", s.h(s.handleUpdateCard))
	mux.Handle("DELETE /api/v1/cards/{cardID}", s.h(s.handleDeleteCard))
	mux.Handle("POST /api/v1/cards/{cardID}/suspend", s.h(s.handleSuspend))
	mux.Handle("POST /api/v1/cards/{cardID}/unsuspend", s.h(s.handleUnsuspend))
	mux.Handle("POST /api/v1/cards/{cardID}/review", s.h(s.handleReview))

	// Anything unmatched gets the standard error body rather than net/http's
	// plain-text 404.
	mux.Handle("/", s.h(func(http.ResponseWriter, *http.Request) error {
		return notFoundError("endpoint")
	}))

	return mux
}

// handlerFunc is a handler that can fail. Returning an error is what keeps
// every handler free of repeated error-writing boilerplate.
type handlerFunc func(w http.ResponseWriter, r *http.Request) error

// h adapts a handlerFunc into an http.Handler.
func (s *Server) h(fn handlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			writeError(w, r, s.logger, err)
		}
	})
}

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Time    string `json:"time"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) error {
	writeJSON(w, r, s.logger, http.StatusOK, healthResponse{
		Status:  "ok",
		Version: s.version,
		Time:    formatTime(s.clock.Now()),
	})
	return nil
}

// schedulerFor builds a scheduler configured from the deck's settings.
func (s *Server) schedulerFor(d *domain.Deck) (scheduler.Scheduler, error) {
	sch, err := s.newSchedule(scheduler.SettingsFor(d))
	if err != nil {
		return nil, internalError(err)
	}
	return sch, nil
}

// dayBounds returns the half-open [start, end) of the local day containing now.
//
// Local, not UTC: kaartd runs on the user's own machine, so "today" should
// change when their day does. A user in UTC+6 would otherwise see their daily
// new-card allowance reset at six in the morning.
func dayBounds(now time.Time) (time.Time, time.Time) {
	local := now.Local()
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	return start.UTC(), start.AddDate(0, 0, 1).UTC()
}
