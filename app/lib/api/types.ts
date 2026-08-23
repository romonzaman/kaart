/**
 * Wire types. Every field here mirrors a Go response struct in
 * internal/api/types.go — when one changes, both change.
 *
 * There is no `any` in this directory, deliberately: the API surface is the one
 * place where a wrong assumption is invisible until runtime.
 */

export type RatingValue = 1 | 2 | 3 | 4;

export const RATINGS: readonly RatingValue[] = [1, 2, 3, 4];

export type RatingName = 'again' | 'hard' | 'good' | 'easy';

export const RATING_NAMES: Record<RatingValue, RatingName> = {
  1: 'again',
  2: 'hard',
  3: 'good',
  4: 'easy',
};

export type CardStateName = 'new' | 'learning' | 'review' | 'relearning';

export type Deck = {
  id: string;
  name: string;
  description: string;
  new_cards_per_day: number;
  max_reviews_per_day: number;
  desired_retention: number;
  fsrs_weights: number[] | null;
  created_at: string;
  updated_at: string;
  archived_at: string | null;
};

export type DeckList = {
  decks: Deck[];
};

export type Card = {
  id: string;
  deck_id: string;
  front: string;
  back: string;
  hint: string;
  tags: string[];
  created_at: string;
  updated_at: string;
  suspended_at: string | null;
};

export type CardList = {
  cards: Card[];
  total: number;
  limit: number;
  offset: number;
};

export type CardState = {
  card_id: string;
  due: string;
  stability: number;
  difficulty: number;
  elapsed_days: number;
  scheduled_days: number;
  reps: number;
  lapses: number;
  state: CardStateName;
  state_code: number;
  last_review: string | null;
};

export type Preview = {
  rating: RatingValue;
  rating_name: RatingName;
  due: string;
  interval_seconds: number;
  label: string;
  state: CardStateName;
};

export type QueueItem = {
  card: Card;
  state: CardState;
  previews: Preview[];
};

export type QueueCounts = {
  total: number;
  new: number;
  learning: number;
  review: number;
};

export type Queue = {
  deck_id: string;
  now: string;
  counts: QueueCounts;
  items: QueueItem[];
};

export type ReviewResult = {
  card_id: string;
  rating: RatingValue;
  rating_name: RatingName;
  reviewed_at: string;
  next_due: string;
  state: CardState;
};

export type DayCount = {
  date: string;
  count: number;
};

export type DeckStats = {
  deck_id: string;
  now: string;
  due_now: number;
  new: number;
  learning: number;
  suspended: number;
  total_cards: number;
  reviews_today: number;
  new_cards_today: number;
  remaining_new_today: number;
  remaining_reviews_today: number;
  /** When the next card comes up, or null when nothing is scheduled ahead. */
  next_due: string | null;
  histogram: DayCount[];
};

export type Health = {
  status: string;
  version: string;
  time: string;
};

// --- request payloads ---

export type CreateDeckInput = {
  name: string;
  description?: string;
  new_cards_per_day?: number;
  max_reviews_per_day?: number;
  desired_retention?: number;
};

export type UpdateDeckInput = {
  name?: string;
  description?: string;
  new_cards_per_day?: number;
  max_reviews_per_day?: number;
  desired_retention?: number;
  archived?: boolean;
};

export type CreateCardInput = {
  front: string;
  back: string;
  hint?: string;
  tags?: string[];
};

export type UpdateCardInput = {
  front?: string;
  back?: string;
  hint?: string;
  tags?: string[];
};

export type ReviewInput = {
  rating: RatingValue;
  duration_ms?: number;
};

export type ListCardsParams = {
  q?: string;
  limit?: number;
  offset?: number;
};
