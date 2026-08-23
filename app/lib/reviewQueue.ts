import { ApiError } from '@/lib/api';
import type { RatingValue } from '@/lib/api';

/**
 * A durable-ish outbox for review submissions.
 *
 * The rule this exists to enforce: a dropped network must never lose a review
 * or block the user mid-session. Ratings go into this queue and the UI advances
 * immediately; the queue drains in the background and retries with backoff
 * until the server comes back.
 *
 * Reviews are submitted strictly in order. FSRS is a state machine over a
 * card's history, and while two reviews of *different* cards commute, sending
 * them out of order gives the server review timestamps that disagree with the
 * order the user actually produced them in.
 */

export type PendingReview = {
  cardId: string;
  rating: RatingValue;
  durationMs: number;
  attempts: number;
};

export type FailedReview = PendingReview & { reason: string };

export type ReviewQueueState = {
  /** Submissions still waiting to be accepted. */
  pending: number;
  /** Submissions the server rejected in a way retrying cannot fix. */
  failed: FailedReview[];
  /** True while a submission is in flight or a retry is scheduled. */
  busy: boolean;
};

export type ReviewQueueOptions = {
  submit: (review: PendingReview) => Promise<unknown>;
  onChange?: (state: ReviewQueueState) => void;
  /** Injected in tests so retries do not depend on real time. */
  schedule?: (fn: () => void, ms: number) => void;
  /** Backoff for attempt n (1-based). Defaults to 1s, 2s, 4s… capped at 30s. */
  backoffMs?: (attempt: number) => number;
};

const defaultBackoff = (attempt: number): number =>
  Math.min(30_000, 1000 * 2 ** Math.max(0, attempt - 1));

export type ReviewQueue = {
  enqueue: (review: Omit<PendingReview, 'attempts'>) => void;
  /** Resolves when the queue is empty — the summary screen waits on this. */
  drain: () => Promise<void>;
  getState: () => ReviewQueueState;
  /** Retry everything that failed permanently, e.g. from a "try again" button. */
  retryFailed: () => void;
};

export function createReviewQueue(options: ReviewQueueOptions): ReviewQueue {
  const schedule = options.schedule ?? ((fn, ms) => void setTimeout(fn, ms));
  const backoff = options.backoffMs ?? defaultBackoff;

  const queue: PendingReview[] = [];
  const failed: FailedReview[] = [];
  let busy = false;
  let waiters: Array<() => void> = [];

  const snapshot = (): ReviewQueueState => ({
    pending: queue.length,
    failed: [...failed],
    busy,
  });

  const notify = () => {
    options.onChange?.(snapshot());
    if (queue.length === 0 && !busy) {
      const pendingWaiters = waiters;
      waiters = [];
      for (const resolve of pendingWaiters) resolve();
    }
  };

  /**
   * A rejection the server will give again no matter how long we wait — a
   * suspended card, a deleted card, a malformed body. Keeping it at the head of
   * the queue would block every later review behind it forever, so it is moved
   * aside and surfaced instead.
   */
  const isPermanent = (error: unknown): boolean =>
    error instanceof ApiError && !error.retryable;

  const flush = (): void => {
    if (busy || queue.length === 0) {
      notify();
      return;
    }

    const review = queue[0];
    if (review === undefined) return;

    busy = true;
    notify();

    void options
      .submit(review)
      .then(() => {
        queue.shift();
        busy = false;
        notify();
        flush();
      })
      .catch((error: unknown) => {
        busy = false;

        if (isPermanent(error)) {
          queue.shift();
          failed.push({
            ...review,
            reason: error instanceof Error ? error.message : String(error),
          });
          notify();
          flush();
          return;
        }

        review.attempts += 1;
        const delay = backoff(review.attempts);
        busy = true;
        notify();

        schedule(() => {
          busy = false;
          flush();
        }, delay);
      });
  };

  return {
    enqueue(review) {
      queue.push({ ...review, attempts: 0 });
      notify();
      flush();
    },

    drain() {
      if (queue.length === 0 && !busy) return Promise.resolve();
      return new Promise<void>((resolve) => {
        waiters.push(resolve);
      });
    },

    getState: snapshot,

    retryFailed() {
      if (failed.length === 0) return;
      const retrying = failed.splice(0, failed.length);
      for (const review of retrying) {
        queue.push({
          cardId: review.cardId,
          rating: review.rating,
          durationMs: review.durationMs,
          attempts: 0,
        });
      }
      notify();
      flush();
    },
  };
}
