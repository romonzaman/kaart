package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/romonzaman/kaart/internal/domain"
)

const stateColumns = `card_id, due, stability, difficulty, elapsed_days, scheduled_days,
	reps, lapses, state, remaining_steps, last_review`

// GetCardState returns a card's scheduling state.
func (s *Store) GetCardState(ctx context.Context, cardID string) (*domain.CardState, error) {
	const q = `SELECT ` + stateColumns + ` FROM card_states WHERE card_id = ?`

	st, err := scanState(s.db.QueryRowContext(ctx, q, cardID))
	if err != nil {
		return nil, notFound(err, fmt.Sprintf("getting state for card %s", cardID))
	}
	return st, nil
}

// UpsertCardState writes a card's scheduling state, inserting it if absent.
func (s *Store) UpsertCardState(ctx context.Context, st *domain.CardState) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		return upsertStateTx(ctx, tx, st)
	})
}

func upsertStateTx(ctx context.Context, tx *sql.Tx, st *domain.CardState) error {
	const q = `INSERT INTO card_states (` + stateColumns + `)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(card_id) DO UPDATE SET
			due            = excluded.due,
			stability      = excluded.stability,
			difficulty     = excluded.difficulty,
			elapsed_days   = excluded.elapsed_days,
			scheduled_days = excluded.scheduled_days,
			reps           = excluded.reps,
			lapses         = excluded.lapses,
			state          = excluded.state,
			remaining_steps = excluded.remaining_steps,
			last_review    = excluded.last_review`

	if _, err := tx.ExecContext(ctx, q,
		st.CardID, ts(st.Due), st.Stability, st.Difficulty,
		st.ElapsedDays, st.ScheduledDays, st.Reps, st.Lapses,
		int(st.State), st.RemainingSteps, tsPtr(st.LastReview),
	); err != nil {
		return fmt.Errorf("upserting state for card %s: %w", st.CardID, err)
	}
	return nil
}

func scanState(sc rowScanner) (*domain.CardState, error) {
	var (
		st         domain.CardState
		due        string
		state      int
		lastReview sql.NullString
	)

	if err := sc.Scan(
		&st.CardID, &due, &st.Stability, &st.Difficulty,
		&st.ElapsedDays, &st.ScheduledDays, &st.Reps, &st.Lapses,
		&state, &st.RemainingSteps, &lastReview,
	); err != nil {
		return nil, err
	}

	st.State = domain.State(state)

	var err error
	if st.Due, err = parseTS(due); err != nil {
		return nil, err
	}
	if st.LastReview, err = parseTSNull(lastReview); err != nil {
		return nil, err
	}
	return &st, nil
}
