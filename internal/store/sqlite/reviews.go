package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/romonzaman/kaart/internal/domain"
	"github.com/romonzaman/kaart/internal/store"
)

// AppendReview adds one row to the append-only review log.
//
// Nothing in Kaart updates or deletes rows in this table. It is the raw material
// for re-optimising FSRS parameters against real history later, and a review
// that has been altered is worse than one that was never recorded.
func (s *Store) AppendReview(ctx context.Context, rv *domain.Review) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		return appendReviewTx(ctx, tx, rv)
	})
}

func appendReviewTx(ctx context.Context, tx *sql.Tx, rv *domain.Review) error {
	const q = `INSERT INTO reviews
		(card_id, rating, state, due, stability, difficulty,
		 elapsed_days, last_elapsed_days, scheduled_days, reviewed_at, duration_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	res, err := tx.ExecContext(ctx, q,
		rv.CardID, int(rv.Rating), int(rv.State), ts(rv.Due),
		rv.Stability, rv.Difficulty,
		rv.ElapsedDays, rv.LastElapsedDays, rv.ScheduledDays,
		ts(rv.ReviewedAt), rv.DurationMS,
	)
	if err != nil {
		return fmt.Errorf("appending review for card %s: %w", rv.CardID, err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("reading review id for card %s: %w", rv.CardID, err)
	}
	rv.ID = id
	return nil
}

// ApplyReview writes the post-review state and appends the log row in one
// transaction. rv carries the state as it was *before* the review.
func (s *Store) ApplyReview(ctx context.Context, st *domain.CardState, rv *domain.Review) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		var suspended sql.NullString
		err := tx.QueryRowContext(ctx,
			`SELECT suspended_at FROM cards WHERE id = ?`, rv.CardID).Scan(&suspended)
		if err != nil {
			return notFound(err, fmt.Sprintf("applying review to card %s", rv.CardID))
		}
		if suspended.Valid {
			return fmt.Errorf("applying review to suspended card %s: %w", rv.CardID, store.ErrConflict)
		}

		if err := upsertStateTx(ctx, tx, st); err != nil {
			return err
		}
		return appendReviewTx(ctx, tx, rv)
	})
}

// ReviewTotals counts reviews for a deck over [from, to).
func (s *Store) ReviewTotals(ctx context.Context, deckID string, from, to time.Time) (store.ReviewTotals, error) {
	const q = `SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN r.state = 0 THEN 1 ELSE 0 END), 0)
		FROM reviews r
		JOIN cards c ON c.id = r.card_id
		WHERE c.deck_id = ? AND r.reviewed_at >= ? AND r.reviewed_at < ?`

	var totals store.ReviewTotals
	if err := s.db.QueryRowContext(ctx, q, deckID, ts(from), ts(to)).
		Scan(&totals.Reviews, &totals.NewCards); err != nil {
		return store.ReviewTotals{}, fmt.Errorf("counting reviews for deck %s: %w", deckID, err)
	}
	return totals, nil
}

// ReviewHistogram returns per-UTC-day review counts over [from, to), skipping
// days with no reviews.
func (s *Store) ReviewHistogram(ctx context.Context, deckID string, from, to time.Time) ([]domain.DayCount, error) {
	const q = `SELECT substr(r.reviewed_at, 1, 10) AS day, COUNT(*)
		FROM reviews r
		JOIN cards c ON c.id = r.card_id
		WHERE c.deck_id = ? AND r.reviewed_at >= ? AND r.reviewed_at < ?
		GROUP BY day
		ORDER BY day ASC`

	rows, err := s.db.QueryContext(ctx, q, deckID, ts(from), ts(to))
	if err != nil {
		return nil, fmt.Errorf("building review histogram for deck %s: %w", deckID, err)
	}
	defer rows.Close()

	out := make([]domain.DayCount, 0, 30)
	for rows.Next() {
		var dc domain.DayCount
		if err := rows.Scan(&dc.Date, &dc.Count); err != nil {
			return nil, fmt.Errorf("building review histogram for deck %s: %w", deckID, err)
		}
		out = append(out, dc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("building review histogram for deck %s: %w", deckID, err)
	}
	return out, nil
}
