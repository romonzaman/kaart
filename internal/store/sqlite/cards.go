package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/romonzaman/kaart/internal/domain"
	"github.com/romonzaman/kaart/internal/store"
)

const cardColumns = `id, deck_id, front, back, hint, tags, created_at, updated_at, suspended_at`

// CreateCard inserts the card and its initial scheduling state atomically, so a
// card can never exist without a row in card_states.
func (s *Store) CreateCard(ctx context.Context, c *domain.Card, st *domain.CardState) error {
	tags, err := encodeTags(c.Tags)
	if err != nil {
		return fmt.Errorf("creating card in deck %s: %w", c.DeckID, err)
	}

	return s.withTx(ctx, func(tx *sql.Tx) error {
		var exists int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM decks WHERE id = ?`, c.DeckID).Scan(&exists)
		if err != nil {
			return notFound(err, fmt.Sprintf("creating card in deck %s", c.DeckID))
		}

		const insertCard = `INSERT INTO cards (` + cardColumns + `)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
		if _, err := tx.ExecContext(ctx, insertCard,
			c.ID, c.DeckID, c.Front, c.Back, c.Hint, tags,
			ts(c.CreatedAt), ts(c.UpdatedAt), tsPtr(c.SuspendedAt),
		); err != nil {
			return fmt.Errorf("creating card %s: %w", c.ID, err)
		}

		if err := upsertStateTx(ctx, tx, st); err != nil {
			return err
		}
		return nil
	})
}

// GetCard returns one card by ID.
func (s *Store) GetCard(ctx context.Context, id string) (*domain.Card, error) {
	const q = `SELECT ` + cardColumns + ` FROM cards WHERE id = ?`

	c, err := scanCard(s.db.QueryRowContext(ctx, q, id))
	if err != nil {
		return nil, notFound(err, fmt.Sprintf("getting card %s", id))
	}
	return c, nil
}

// ListCards returns a page of a deck's cards plus the total matching count.
func (s *Store) ListCards(ctx context.Context, deckID string, f store.CardFilter) ([]*domain.Card, int, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = defaultCardLimit
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	where := `WHERE deck_id = ?`
	args := []any{deckID}
	if q := strings.TrimSpace(f.Query); q != "" {
		where += ` AND (front LIKE ? ESCAPE '\' OR back LIKE ? ESCAPE '\' OR hint LIKE ? ESCAPE '\')`
		pat := "%" + escapeLike(q) + "%"
		args = append(args, pat, pat, pat)
	}

	var total int
	countQ := `SELECT COUNT(*) FROM cards ` + where
	if err := s.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting cards in deck %s: %w", deckID, err)
	}

	listQ := `SELECT ` + cardColumns + ` FROM cards ` + where +
		` ORDER BY created_at ASC, id ASC LIMIT ? OFFSET ?`
	rows, err := s.db.QueryContext(ctx, listQ, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing cards in deck %s: %w", deckID, err)
	}
	defer rows.Close()

	cards := make([]*domain.Card, 0, limit)
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("listing cards in deck %s: %w", deckID, err)
		}
		cards = append(cards, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("listing cards in deck %s: %w", deckID, err)
	}
	return cards, total, nil
}

// UpdateCard overwrites the card's content columns. Its scheduling state is
// untouched: editing a typo on the back of a card must not reset its interval.
func (s *Store) UpdateCard(ctx context.Context, c *domain.Card) error {
	tags, err := encodeTags(c.Tags)
	if err != nil {
		return fmt.Errorf("updating card %s: %w", c.ID, err)
	}

	const q = `UPDATE cards SET front = ?, back = ?, hint = ?, tags = ?, updated_at = ?
		WHERE id = ?`

	res, err := s.db.ExecContext(ctx, q, c.Front, c.Back, c.Hint, tags, ts(c.UpdatedAt), c.ID)
	if err != nil {
		return fmt.Errorf("updating card %s: %w", c.ID, err)
	}
	return affected(res, fmt.Sprintf("updating card %s", c.ID))
}

// DeleteCard removes a card. Its state and reviews cascade.
func (s *Store) DeleteCard(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM cards WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting card %s: %w", id, err)
	}
	return affected(res, fmt.Sprintf("deleting card %s", id))
}

// SetCardSuspended sets suspended_at, or clears it when at is nil.
func (s *Store) SetCardSuspended(ctx context.Context, id string, at *time.Time) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE cards SET suspended_at = ? WHERE id = ?`, tsPtr(at), id)
	if err != nil {
		return fmt.Errorf("setting suspension on card %s: %w", id, err)
	}
	return affected(res, fmt.Sprintf("setting suspension on card %s", id))
}

func scanCard(sc rowScanner) (*domain.Card, error) {
	var (
		c         domain.Card
		tags      string
		created   string
		updated   string
		suspended sql.NullString
	)

	if err := sc.Scan(
		&c.ID, &c.DeckID, &c.Front, &c.Back, &c.Hint, &tags,
		&created, &updated, &suspended,
	); err != nil {
		return nil, err
	}

	var err error
	if c.Tags, err = decodeTags(tags); err != nil {
		return nil, err
	}
	if c.CreatedAt, err = parseTS(created); err != nil {
		return nil, err
	}
	if c.UpdatedAt, err = parseTS(updated); err != nil {
		return nil, err
	}
	if c.SuspendedAt, err = parseTSNull(suspended); err != nil {
		return nil, err
	}
	return &c, nil
}

// escapeLike neutralises LIKE wildcards in user-supplied search text.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func escapeLike(s string) string { return likeEscaper.Replace(s) }
