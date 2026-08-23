package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/romonzaman/kaart/internal/domain"
	"github.com/romonzaman/kaart/internal/store"
)

// queueSelect joins a card to its state. Column order must match scanQueueItem.
const queueSelect = `SELECT
	c.id, c.deck_id, c.front, c.back, c.hint, c.tags, c.created_at, c.updated_at, c.suspended_at,
	s.card_id, s.due, s.stability, s.difficulty, s.elapsed_days, s.scheduled_days,
	s.reps, s.lapses, s.state, s.remaining_steps, s.last_review
	FROM cards c
	JOIN card_states s ON s.card_id = c.id
	WHERE c.deck_id = ? AND c.suspended_at IS NULL`

// DueQueue assembles the study queue in three passes so the ordering rule is
// readable rather than encoded in a CASE expression:
//
//  1. learning and relearning cards that are due, soonest first
//  2. review cards that are due, soonest first
//  3. new cards, oldest first, up to the deck's remaining daily new allowance
//
// Learning cards come first because their intervals are minutes: making the user
// wait behind a long review backlog is how short-term steps get missed.
func (s *Store) DueQueue(ctx context.Context, deckID string, p store.QueueParams) ([]store.QueueItem, error) {
	limit := p.Limit
	if limit <= 0 {
		limit = 50
	}

	reviewBudget := p.ReviewLimit
	if reviewBudget < 0 {
		reviewBudget = 0
	}
	newBudget := p.NewLimit
	if newBudget < 0 {
		newBudget = 0
	}

	items := make([]store.QueueItem, 0, limit)
	now := ts(p.Now)

	// 1. learning + relearning
	if n := min(reviewBudget, limit-len(items)); n > 0 {
		const q = queueSelect + ` AND s.state IN (1, 3) AND s.due <= ?
			ORDER BY s.due ASC, c.created_at ASC, c.id ASC LIMIT ?`
		batch, err := s.queryQueue(ctx, deckID, q, deckID, now, n)
		if err != nil {
			return nil, err
		}
		items = append(items, batch...)
		reviewBudget -= len(batch)
	}

	// 2. review
	if n := min(reviewBudget, limit-len(items)); n > 0 {
		const q = queueSelect + ` AND s.state = 2 AND s.due <= ?
			ORDER BY s.due ASC, c.created_at ASC, c.id ASC LIMIT ?`
		batch, err := s.queryQueue(ctx, deckID, q, deckID, now, n)
		if err != nil {
			return nil, err
		}
		items = append(items, batch...)
	}

	// 3. new
	if n := min(newBudget, limit-len(items)); n > 0 {
		const q = queueSelect + ` AND s.state = 0 AND s.due <= ?
			ORDER BY c.created_at ASC, c.id ASC LIMIT ?`
		batch, err := s.queryQueue(ctx, deckID, q, deckID, now, n)
		if err != nil {
			return nil, err
		}
		items = append(items, batch...)
	}

	return items, nil
}

func (s *Store) queryQueue(ctx context.Context, deckID, query string, args ...any) ([]store.QueueItem, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("building queue for deck %s: %w", deckID, err)
	}
	defer rows.Close()

	var out []store.QueueItem
	for rows.Next() {
		item, err := scanQueueItem(rows)
		if err != nil {
			return nil, fmt.Errorf("building queue for deck %s: %w", deckID, err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("building queue for deck %s: %w", deckID, err)
	}
	return out, nil
}

func scanQueueItem(rows rowScanner) (store.QueueItem, error) {
	var (
		c         domain.Card
		st        domain.CardState
		tags      string
		created   string
		updated   string
		suspended sql.NullString
		due       string
		state     int
		last      sql.NullString
	)

	if err := rows.Scan(
		&c.ID, &c.DeckID, &c.Front, &c.Back, &c.Hint, &tags, &created, &updated, &suspended,
		&st.CardID, &due, &st.Stability, &st.Difficulty, &st.ElapsedDays, &st.ScheduledDays,
		&st.Reps, &st.Lapses, &state, &st.RemainingSteps, &last,
	); err != nil {
		return store.QueueItem{}, err
	}

	var err error
	if c.Tags, err = decodeTags(tags); err != nil {
		return store.QueueItem{}, err
	}
	if c.CreatedAt, err = parseTS(created); err != nil {
		return store.QueueItem{}, err
	}
	if c.UpdatedAt, err = parseTS(updated); err != nil {
		return store.QueueItem{}, err
	}
	if c.SuspendedAt, err = parseTSNull(suspended); err != nil {
		return store.QueueItem{}, err
	}

	st.State = domain.State(state)
	if st.Due, err = parseTS(due); err != nil {
		return store.QueueItem{}, err
	}
	if st.LastReview, err = parseTSNull(last); err != nil {
		return store.QueueItem{}, err
	}

	return store.QueueItem{Card: c, State: st}, nil
}

// DeckCounts summarises a deck's queue composition at now.
func (s *Store) DeckCounts(ctx context.Context, deckID string, now time.Time) (domain.DeckCounts, error) {
	const q = `SELECT
		COUNT(*),
		COALESCE(SUM(CASE WHEN c.suspended_at IS NULL AND s.due <= ? THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN c.suspended_at IS NULL AND s.state = 0 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN c.suspended_at IS NULL AND s.state IN (1, 3) THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN c.suspended_at IS NOT NULL THEN 1 ELSE 0 END), 0)
		FROM cards c
		JOIN card_states s ON s.card_id = c.id
		WHERE c.deck_id = ?`

	var counts domain.DeckCounts
	if err := s.db.QueryRowContext(ctx, q, ts(now), deckID).Scan(
		&counts.Total, &counts.Due, &counts.New, &counts.Learning, &counts.Suspended,
	); err != nil {
		return domain.DeckCounts{}, fmt.Errorf("counting cards in deck %s: %w", deckID, err)
	}
	return counts, nil
}

// NextDue returns when the deck's next card comes up after the given instant.
func (s *Store) NextDue(ctx context.Context, deckID string, after time.Time) (*time.Time, error) {
	const q = `SELECT MIN(s.due)
		FROM cards c
		JOIN card_states s ON s.card_id = c.id
		WHERE c.deck_id = ? AND c.suspended_at IS NULL AND s.due > ?`

	var due sql.NullString
	if err := s.db.QueryRowContext(ctx, q, deckID, ts(after)).Scan(&due); err != nil {
		return nil, fmt.Errorf("finding next due card in deck %s: %w", deckID, err)
	}

	next, err := parseTSNull(due)
	if err != nil {
		return nil, fmt.Errorf("finding next due card in deck %s: %w", deckID, err)
	}
	return next, nil
}
