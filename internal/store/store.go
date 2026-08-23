// Package store defines the persistence boundary for Kaart. Every read and
// write of application data goes through the Store interface; no SQL exists
// outside this package's implementations. That constraint is what makes a
// second backend (Postgres, Supabase) an additive change rather than a rewrite.
package store

import (
	"context"
	"errors"
	"time"

	"github.com/romonzaman/kaart/internal/domain"
)

// Sentinel errors returned by every Store implementation.
var (
	// ErrNotFound means the requested row does not exist.
	ErrNotFound = errors.New("store: not found")
	// ErrConflict means the write violated an invariant the caller can fix,
	// such as reviewing a suspended card.
	ErrConflict = errors.New("store: conflict")
)

// DeckFilter narrows ListDecks.
type DeckFilter struct {
	// IncludeArchived returns archived decks as well as active ones.
	IncludeArchived bool
}

// CardFilter narrows and pages ListCards.
type CardFilter struct {
	// Query is a case-insensitive substring match over front, back, and hint.
	Query string
	// Limit caps the number of rows returned. Zero means the default (50).
	Limit int
	// Offset skips rows for paging.
	Offset int
}

// QueueParams controls how a study queue is assembled.
type QueueParams struct {
	// Now is the instant the queue is built for; cards due after it are excluded.
	Now time.Time
	// Limit caps the total number of items returned.
	Limit int
	// NewLimit caps how many state-New cards may appear (the deck's remaining
	// new-card allowance for today). Zero means no new cards.
	NewLimit int
	// ReviewLimit caps how many learning, relearning, and review cards may
	// appear (the deck's remaining review allowance for today).
	ReviewLimit int
}

// QueueItem pairs a card with its scheduling state.
type QueueItem struct {
	Card  domain.Card
	State domain.CardState
}

// ReviewTotals counts what has already been studied within a time window.
type ReviewTotals struct {
	// Reviews is every review in the window, whatever the card's prior state.
	Reviews int
	// NewCards is the subset whose pre-review state was New — i.e. cards
	// introduced for the first time in the window.
	NewCards int
}

// Store is the complete persistence surface of Kaart.
//
// Every method takes a context and returns errors wrapped with enough context
// to identify the operation. Callers distinguish cases with errors.Is against
// ErrNotFound and ErrConflict; no other error class is part of the contract.
type Store interface {
	// --- decks ---

	// CreateDeck inserts d. The caller supplies ID and timestamps.
	CreateDeck(ctx context.Context, d *domain.Deck) error
	// GetDeck returns the deck, or ErrNotFound.
	GetDeck(ctx context.Context, id string) (*domain.Deck, error)
	// ListDecks returns decks ordered by name.
	ListDecks(ctx context.Context, f DeckFilter) ([]*domain.Deck, error)
	// UpdateDeck overwrites every mutable column of the deck, or ErrNotFound.
	UpdateDeck(ctx context.Context, d *domain.Deck) error
	// DeleteDeck removes the deck and, by cascade, its cards, their states,
	// and their reviews. Returns ErrNotFound if the deck does not exist.
	DeleteDeck(ctx context.Context, id string) error

	// --- cards ---

	// CreateCard inserts the card and its initial scheduling state in one
	// transaction. Returns ErrNotFound if the deck does not exist.
	CreateCard(ctx context.Context, c *domain.Card, st *domain.CardState) error
	// GetCard returns the card, or ErrNotFound.
	GetCard(ctx context.Context, id string) (*domain.Card, error)
	// ListCards returns a page of the deck's cards ordered by creation time,
	// along with the total number of cards matching the filter.
	ListCards(ctx context.Context, deckID string, f CardFilter) ([]*domain.Card, int, error)
	// UpdateCard overwrites the card's content columns, or ErrNotFound.
	UpdateCard(ctx context.Context, c *domain.Card) error
	// DeleteCard removes the card, its state, and its reviews.
	DeleteCard(ctx context.Context, id string) error
	// SetCardSuspended sets or clears the card's suspended_at timestamp.
	// Passing nil unsuspends. Returns ErrNotFound if the card does not exist.
	SetCardSuspended(ctx context.Context, id string, at *time.Time) error

	// --- scheduling state ---

	// GetCardState returns the card's scheduling state, or ErrNotFound.
	GetCardState(ctx context.Context, cardID string) (*domain.CardState, error)
	// UpsertCardState writes the state, inserting it if absent.
	UpsertCardState(ctx context.Context, st *domain.CardState) error

	// --- reviews ---

	// AppendReview adds a row to the append-only review log and sets rv.ID.
	AppendReview(ctx context.Context, rv *domain.Review) error
	// ApplyReview writes the new card state and appends the review row in one
	// transaction. rv records the state as it was before the review.
	// Returns ErrNotFound for an unknown card and ErrConflict for a suspended one.
	ApplyReview(ctx context.Context, st *domain.CardState, rv *domain.Review) error

	// --- queue and statistics ---

	// DueQueue returns the deck's study queue: learning and relearning cards
	// first, then review cards by due date ascending, then new cards.
	DueQueue(ctx context.Context, deckID string, p QueueParams) ([]QueueItem, error)
	// DeckCounts summarises the deck's queue composition at now.
	DeckCounts(ctx context.Context, deckID string, now time.Time) (domain.DeckCounts, error)
	// NextDue returns the earliest due time strictly after `after` among the
	// deck's unsuspended cards, or nil when nothing is scheduled beyond it.
	// The study screen uses it to say "next card in 3 hours" rather than
	// showing an empty session with no explanation.
	NextDue(ctx context.Context, deckID string, after time.Time) (*time.Time, error)
	// ReviewTotals counts reviews in the deck over [from, to).
	ReviewTotals(ctx context.Context, deckID string, from, to time.Time) (ReviewTotals, error)
	// ReviewHistogram returns one entry per UTC day in [from, to) that has at
	// least one review, ordered by date ascending.
	ReviewHistogram(ctx context.Context, deckID string, from, to time.Time) ([]domain.DayCount, error)

	// Close releases the underlying resources.
	Close() error
}
