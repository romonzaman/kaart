import type { QueueItem, RatingName, RatingValue } from '@/lib/api';
import { RATING_NAMES } from '@/lib/api';

/**
 * The review session's state machine, as a pure reducer.
 *
 * It lives outside the component so it can be tested without rendering
 * anything: the reveal/rate cycle is the part of the app most likely to break
 * subtly (a double keypress rating two cards, a duration measured from the
 * wrong moment) and least likely to be caught by looking at it.
 */

export type SessionPhase = 'question' | 'answer' | 'summary';

export type SessionResult = {
  cardId: string;
  rating: RatingValue;
  durationMs: number;
};

export type SessionState = {
  items: QueueItem[];
  index: number;
  phase: SessionPhase;
  /** Timestamp of the reveal, or null while the front is showing. */
  revealedAt: number | null;
  results: SessionResult[];
  startedAt: number;
  endedAt: number | null;
};

export type SessionAction =
  | { type: 'start'; items: QueueItem[]; at: number }
  | { type: 'reveal'; at: number }
  | { type: 'rate'; rating: RatingValue; at: number };

/** One hour, matching the server's cap on duration_ms. */
export const MAX_DURATION_MS = 60 * 60 * 1000;

export function startSession(items: QueueItem[], at: number): SessionState {
  return {
    items,
    index: 0,
    phase: items.length === 0 ? 'summary' : 'question',
    revealedAt: null,
    results: [],
    startedAt: at,
    endedAt: items.length === 0 ? at : null,
  };
}

export function sessionReducer(state: SessionState, action: SessionAction): SessionState {
  switch (action.type) {
    case 'start':
      return startSession(action.items, action.at);

    case 'reveal':
      // Only meaningful while a question is showing. A second Space press is a
      // no-op rather than a second, later reveal timestamp.
      if (state.phase !== 'question') return state;
      return { ...state, phase: 'answer', revealedAt: action.at };

    case 'rate': {
      // Rating is only possible after a reveal. This is what stops a fast
      // double keypress from rating the next card as well as this one.
      if (state.phase !== 'answer') return state;

      const current = state.items[state.index];
      if (current === undefined) return state;

      const elapsed = state.revealedAt === null ? 0 : action.at - state.revealedAt;
      const durationMs = Math.min(Math.max(0, Math.round(elapsed)), MAX_DURATION_MS);

      const results = [
        ...state.results,
        { cardId: current.card.id, rating: action.rating, durationMs },
      ];
      const nextIndex = state.index + 1;
      const done = nextIndex >= state.items.length;

      return {
        ...state,
        index: nextIndex,
        phase: done ? 'summary' : 'question',
        revealedAt: null,
        results,
        endedAt: done ? action.at : null,
      };
    }

    default:
      return state;
  }
}

export function currentItem(state: SessionState): QueueItem | undefined {
  return state.items[state.index];
}

export type RemainingCounts = {
  total: number;
  new: number;
  learning: number;
  review: number;
};

/** What is still ahead, including the card on screen. */
export function remainingCounts(state: SessionState): RemainingCounts {
  const counts: RemainingCounts = { total: 0, new: 0, learning: 0, review: 0 };

  for (let i = state.index; i < state.items.length; i += 1) {
    const item = state.items[i];
    if (item === undefined) continue;
    counts.total += 1;
    switch (item.state.state) {
      case 'new':
        counts.new += 1;
        break;
      case 'learning':
      case 'relearning':
        counts.learning += 1;
        break;
      case 'review':
        counts.review += 1;
        break;
      default:
        break;
    }
  }

  return counts;
}

export function ratingBreakdown(state: SessionState): Record<RatingName, number> {
  const breakdown: Record<RatingName, number> = { again: 0, hard: 0, good: 0, easy: 0 };
  for (const result of state.results) {
    const name = RATING_NAMES[result.rating];
    breakdown[name] += 1;
  }
  return breakdown;
}

/** Wall-clock length of the session so far. */
export function sessionDurationMs(state: SessionState, now: number): number {
  return Math.max(0, (state.endedAt ?? now) - state.startedAt);
}

/** "4m 12s" — session length, not card intervals. */
export function formatDuration(ms: number): string {
  const totalSeconds = Math.round(ms / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  if (minutes === 0) return `${seconds}s`;
  return `${minutes}m ${seconds}s`;
}

/**
 * "in 3 hours" / "in 12 minutes" — used by the nothing-due state so it can say
 * when to come back rather than showing a blank screen.
 */
export function formatRelativeFuture(iso: string, now: number): string {
  const target = Date.parse(iso);
  if (Number.isNaN(target)) return 'soon';

  const ms = target - now;
  if (ms <= 60_000) return 'in under a minute';

  const minutes = Math.round(ms / 60_000);
  if (minutes < 60) return `in ${minutes} minute${minutes === 1 ? '' : 's'}`;

  const hours = Math.round(minutes / 60);
  if (hours < 24) return `in ${hours} hour${hours === 1 ? '' : 's'}`;

  const days = Math.round(hours / 24);
  return `in ${days} day${days === 1 ? '' : 's'}`;
}
