import type { CardStateName, QueueItem, RatingValue } from '@/lib/api';
import {
  MAX_DURATION_MS,
  currentItem,
  formatDuration,
  formatRelativeFuture,
  ratingBreakdown,
  remainingCounts,
  sessionDurationMs,
  sessionReducer,
  startSession,
} from '@/lib/session';

function makeItem(id: string, state: CardStateName = 'new'): QueueItem {
  return {
    card: {
      id,
      deck_id: 'deck-1',
      front: `front-${id}`,
      back: `back-${id}`,
      hint: '',
      tags: [],
      created_at: '2026-05-04T09:00:00Z',
      updated_at: '2026-05-04T09:00:00Z',
      suspended_at: null,
    },
    state: {
      card_id: id,
      due: '2026-05-04T09:00:00Z',
      stability: 0,
      difficulty: 0,
      elapsed_days: 0,
      scheduled_days: 0,
      reps: 0,
      lapses: 0,
      state,
      state_code: 0,
      last_review: null,
    },
    previews: [
      { rating: 1, rating_name: 'again', due: '', interval_seconds: 60, label: '<1m', state: 'learning' },
      { rating: 2, rating_name: 'hard', due: '', interval_seconds: 360, label: '6m', state: 'learning' },
      { rating: 3, rating_name: 'good', due: '', interval_seconds: 600, label: '10m', state: 'learning' },
      { rating: 4, rating_name: 'easy', due: '', interval_seconds: 345600, label: '4d', state: 'review' },
    ],
  };
}

const T0 = 1_000_000;

describe('session state machine', () => {
  it('starts on the first card showing the question', () => {
    const state = startSession([makeItem('a'), makeItem('b')], T0);

    expect(state.phase).toBe('question');
    expect(state.index).toBe(0);
    expect(state.revealedAt).toBeNull();
    expect(currentItem(state)?.card.id).toBe('a');
  });

  it('goes straight to the summary when the queue is empty', () => {
    const state = startSession([], T0);

    expect(state.phase).toBe('summary');
    expect(state.results).toHaveLength(0);
    expect(currentItem(state)).toBeUndefined();
  });

  it('reveals the answer and records when it happened', () => {
    let state = startSession([makeItem('a')], T0);
    state = sessionReducer(state, { type: 'reveal', at: T0 + 500 });

    expect(state.phase).toBe('answer');
    expect(state.revealedAt).toBe(T0 + 500);
  });

  it('ignores a second reveal so the duration is measured from the first', () => {
    let state = startSession([makeItem('a')], T0);
    state = sessionReducer(state, { type: 'reveal', at: T0 + 500 });
    const afterFirst = state;

    state = sessionReducer(state, { type: 'reveal', at: T0 + 9000 });

    expect(state).toBe(afterFirst);
    expect(state.revealedAt).toBe(T0 + 500);
  });

  it('refuses to rate a card that has not been revealed', () => {
    const state = startSession([makeItem('a')], T0);
    const next = sessionReducer(state, { type: 'rate', rating: 3, at: T0 + 100 });

    expect(next).toBe(state);
    expect(next.results).toHaveLength(0);
  });

  it('ignores a repeated rating so one keypress cannot rate two cards', () => {
    let state = startSession([makeItem('a'), makeItem('b')], T0);
    state = sessionReducer(state, { type: 'reveal', at: T0 + 100 });
    state = sessionReducer(state, { type: 'rate', rating: 3, at: T0 + 1100 });

    // Phase is now 'question' for card b — a second rate must not land.
    const afterFirstRating = state;
    state = sessionReducer(state, { type: 'rate', rating: 3, at: T0 + 1101 });

    expect(state).toBe(afterFirstRating);
    expect(state.results).toHaveLength(1);
    expect(currentItem(state)?.card.id).toBe('b');
  });

  it('measures duration from reveal to rating, not from the card appearing', () => {
    let state = startSession([makeItem('a')], T0);
    state = sessionReducer(state, { type: 'reveal', at: T0 + 5_000 });
    state = sessionReducer(state, { type: 'rate', rating: 2, at: T0 + 7_400 });

    expect(state.results[0]).toEqual({ cardId: 'a', rating: 2, durationMs: 2_400 });
  });

  it('clamps an absurd duration to the server-side maximum', () => {
    let state = startSession([makeItem('a')], T0);
    state = sessionReducer(state, { type: 'reveal', at: T0 });
    state = sessionReducer(state, { type: 'rate', rating: 3, at: T0 + MAX_DURATION_MS * 5 });

    expect(state.results[0]?.durationMs).toBe(MAX_DURATION_MS);
  });

  it('never records a negative duration if the clock jumps backwards', () => {
    let state = startSession([makeItem('a')], T0);
    state = sessionReducer(state, { type: 'reveal', at: T0 + 10_000 });
    state = sessionReducer(state, { type: 'rate', rating: 3, at: T0 + 2_000 });

    expect(state.results[0]?.durationMs).toBe(0);
  });

  it('advances through every card and then ends', () => {
    let state = startSession([makeItem('a'), makeItem('b'), makeItem('c')], T0);

    const ratings: RatingValue[] = [3, 1, 4];
    ratings.forEach((rating, i) => {
      state = sessionReducer(state, { type: 'reveal', at: T0 + i * 1000 });
      state = sessionReducer(state, { type: 'rate', rating, at: T0 + i * 1000 + 500 });
    });

    expect(state.phase).toBe('summary');
    expect(state.results.map((r) => r.cardId)).toEqual(['a', 'b', 'c']);
    expect(state.endedAt).not.toBeNull();
  });

  it('counts what is left by card type, including the card on screen', () => {
    let state = startSession(
      [makeItem('a', 'new'), makeItem('b', 'learning'), makeItem('c', 'review'), makeItem('d', 'relearning')],
      T0,
    );

    expect(remainingCounts(state)).toEqual({ total: 4, new: 1, learning: 2, review: 1 });

    state = sessionReducer(state, { type: 'reveal', at: T0 });
    state = sessionReducer(state, { type: 'rate', rating: 3, at: T0 + 100 });

    expect(remainingCounts(state)).toEqual({ total: 3, new: 0, learning: 2, review: 1 });
  });

  it('breaks results down by rating', () => {
    let state = startSession([makeItem('a'), makeItem('b'), makeItem('c')], T0);

    const ratings: RatingValue[] = [1, 3, 3];
    ratings.forEach((rating, i) => {
      state = sessionReducer(state, { type: 'reveal', at: T0 + i * 100 });
      state = sessionReducer(state, { type: 'rate', rating, at: T0 + i * 100 + 50 });
    });

    expect(ratingBreakdown(state)).toEqual({ again: 1, hard: 0, good: 2, easy: 0 });
  });

  it('reports session length from start to the last rating', () => {
    let state = startSession([makeItem('a')], T0);
    state = sessionReducer(state, { type: 'reveal', at: T0 + 1_000 });
    state = sessionReducer(state, { type: 'rate', rating: 3, at: T0 + 4_000 });

    // Once ended, the clock stops — a summary left open does not keep counting.
    expect(sessionDurationMs(state, T0 + 900_000)).toBe(4_000);
  });
});

describe('formatting', () => {
  it('formats session durations', () => {
    expect(formatDuration(4_000)).toBe('4s');
    expect(formatDuration(65_000)).toBe('1m 5s');
    expect(formatDuration(600_000)).toBe('10m 0s');
  });

  it('says when the next card comes up', () => {
    const now = Date.parse('2026-05-04T09:00:00Z');

    expect(formatRelativeFuture('2026-05-04T09:00:30Z', now)).toBe('in under a minute');
    expect(formatRelativeFuture('2026-05-04T09:12:00Z', now)).toBe('in 12 minutes');
    expect(formatRelativeFuture('2026-05-04T10:01:00Z', now)).toBe('in 1 hour');
    expect(formatRelativeFuture('2026-05-04T12:00:00Z', now)).toBe('in 3 hours');
    expect(formatRelativeFuture('2026-05-06T09:00:00Z', now)).toBe('in 2 days');
    expect(formatRelativeFuture('not a date', now)).toBe('soon');
  });
});
