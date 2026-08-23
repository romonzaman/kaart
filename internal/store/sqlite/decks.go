package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/romonzaman/kaart/internal/domain"
	"github.com/romonzaman/kaart/internal/store"
)

const deckColumns = `id, name, description, new_cards_per_day, max_reviews_per_day,
	desired_retention, fsrs_weights, created_at, updated_at, archived_at`

// CreateDeck inserts a deck.
func (s *Store) CreateDeck(ctx context.Context, d *domain.Deck) error {
	weights, err := encodeWeights(d.FSRSWeights)
	if err != nil {
		return fmt.Errorf("creating deck: %w", err)
	}

	const q = `INSERT INTO decks (` + deckColumns + `)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	if _, err := s.db.ExecContext(ctx, q,
		d.ID, d.Name, d.Description,
		d.NewCardsPerDay, d.MaxReviewsPerDay, d.DesiredRetention, weights,
		ts(d.CreatedAt), ts(d.UpdatedAt), tsPtr(d.ArchivedAt),
	); err != nil {
		return fmt.Errorf("creating deck %s: %w", d.ID, err)
	}
	return nil
}

// GetDeck returns one deck by ID.
func (s *Store) GetDeck(ctx context.Context, id string) (*domain.Deck, error) {
	const q = `SELECT ` + deckColumns + ` FROM decks WHERE id = ?`

	d, err := scanDeck(s.db.QueryRowContext(ctx, q, id))
	if err != nil {
		return nil, notFound(err, fmt.Sprintf("getting deck %s", id))
	}
	return d, nil
}

// ListDecks returns decks ordered by name.
func (s *Store) ListDecks(ctx context.Context, f store.DeckFilter) ([]*domain.Deck, error) {
	q := `SELECT ` + deckColumns + ` FROM decks`
	if !f.IncludeArchived {
		q += ` WHERE archived_at IS NULL`
	}
	q += ` ORDER BY name COLLATE NOCASE ASC, id ASC`

	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("listing decks: %w", err)
	}
	defer rows.Close()

	decks := make([]*domain.Deck, 0, 8)
	for rows.Next() {
		d, err := scanDeck(rows)
		if err != nil {
			return nil, fmt.Errorf("listing decks: %w", err)
		}
		decks = append(decks, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing decks: %w", err)
	}
	return decks, nil
}

// UpdateDeck overwrites every mutable column.
func (s *Store) UpdateDeck(ctx context.Context, d *domain.Deck) error {
	weights, err := encodeWeights(d.FSRSWeights)
	if err != nil {
		return fmt.Errorf("updating deck %s: %w", d.ID, err)
	}

	const q = `UPDATE decks SET
		name = ?, description = ?,
		new_cards_per_day = ?, max_reviews_per_day = ?,
		desired_retention = ?, fsrs_weights = ?,
		updated_at = ?, archived_at = ?
		WHERE id = ?`

	res, err := s.db.ExecContext(ctx, q,
		d.Name, d.Description,
		d.NewCardsPerDay, d.MaxReviewsPerDay, d.DesiredRetention, weights,
		ts(d.UpdatedAt), tsPtr(d.ArchivedAt),
		d.ID,
	)
	if err != nil {
		return fmt.Errorf("updating deck %s: %w", d.ID, err)
	}
	return affected(res, fmt.Sprintf("updating deck %s", d.ID))
}

// DeleteDeck removes a deck. Cards, card states, and reviews cascade.
func (s *Store) DeleteDeck(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM decks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting deck %s: %w", id, err)
	}
	return affected(res, fmt.Sprintf("deleting deck %s", id))
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanDeck(sc rowScanner) (*domain.Deck, error) {
	var (
		d        domain.Deck
		weights  sql.NullString
		created  string
		updated  string
		archived sql.NullString
	)

	if err := sc.Scan(
		&d.ID, &d.Name, &d.Description,
		&d.NewCardsPerDay, &d.MaxReviewsPerDay, &d.DesiredRetention, &weights,
		&created, &updated, &archived,
	); err != nil {
		return nil, err
	}

	var err error
	if d.FSRSWeights, err = decodeWeights(weights); err != nil {
		return nil, err
	}
	if d.CreatedAt, err = parseTS(created); err != nil {
		return nil, err
	}
	if d.UpdatedAt, err = parseTS(updated); err != nil {
		return nil, err
	}
	if d.ArchivedAt, err = parseTSNull(archived); err != nil {
		return nil, err
	}
	return &d, nil
}
