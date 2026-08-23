import { ApiError } from '@/lib/api';
import { createReviewQueue } from '@/lib/reviewQueue';
import type { PendingReview, ReviewQueueState } from '@/lib/reviewQueue';

/** Collects scheduled retries so tests can run them on demand. */
function makeScheduler() {
  const jobs: Array<{ fn: () => void; ms: number }> = [];
  return {
    schedule: (fn: () => void, ms: number) => {
      jobs.push({ fn, ms });
    },
    /** Runs every job queued so far. */
    runAll: () => {
      const pending = jobs.splice(0, jobs.length);
      for (const job of pending) job.fn();
    },
    delays: () => jobs.map((j) => j.ms),
    count: () => jobs.length,
  };
}

const review = (cardId: string) => ({ cardId, rating: 3 as const, durationMs: 1000 });

/**
 * Lets the outbox's promise chain settle. A single `await Promise.resolve()` is
 * not enough: a failed submit has to travel through the rejected promise, the
 * catch handler, and the backoff scheduling before the effect is observable.
 */
const tick = () => new Promise<void>((resolve) => setTimeout(resolve, 0));

describe('review outbox', () => {
  it('submits a review and drains', async () => {
    const submitted: PendingReview[] = [];
    const queue = createReviewQueue({
      submit: async (r) => {
        submitted.push({ ...r });
      },
    });

    queue.enqueue(review('a'));
    await queue.drain();

    expect(submitted.map((r) => r.cardId)).toEqual(['a']);
    expect(queue.getState().pending).toBe(0);
  });

  it('submits in the order the user rated, not in parallel', async () => {
    const order: string[] = [];
    let inFlight = 0;

    const queue = createReviewQueue({
      submit: async (r) => {
        inFlight += 1;
        expect(inFlight).toBe(1);
        await tick();
        order.push(r.cardId);
        inFlight -= 1;
      },
    });

    queue.enqueue(review('a'));
    queue.enqueue(review('b'));
    queue.enqueue(review('c'));
    await queue.drain();

    expect(order).toEqual(['a', 'b', 'c']);
  });

  /**
   * The acceptance criterion: killing the API mid-session must not lose a
   * review. Here the server is "down" for the first two attempts and the
   * reviews still land, in order, once it answers.
   */
  it('retries a network failure until the server comes back, losing nothing', async () => {
    const scheduler = makeScheduler();
    const accepted: string[] = [];
    let serverDown = true;

    const queue = createReviewQueue({
      schedule: scheduler.schedule,
      submit: async (r) => {
        if (serverDown) {
          throw new ApiError('network', 'Could not reach Kaart');
        }
        accepted.push(r.cardId);
      },
    });

    queue.enqueue(review('a'));
    queue.enqueue(review('b'));
    await tick();

    expect(accepted).toHaveLength(0);
    expect(queue.getState().pending).toBe(2);

    // Still down: the retry is rescheduled rather than dropped.
    scheduler.runAll();
    await tick();
    expect(accepted).toHaveLength(0);
    expect(queue.getState().pending).toBe(2);

    serverDown = false;
    scheduler.runAll();
    await queue.drain();

    expect(accepted).toEqual(['a', 'b']);
    expect(queue.getState().pending).toBe(0);
    expect(queue.getState().failed).toHaveLength(0);
  });

  it('backs off further on each successive failure', async () => {
    const scheduler = makeScheduler();
    const queue = createReviewQueue({
      schedule: scheduler.schedule,
      submit: async () => {
        throw new ApiError('network', 'down');
      },
    });

    queue.enqueue(review('a'));
    await tick();
    const first = scheduler.delays()[0];

    scheduler.runAll();
    await tick();
    const second = scheduler.delays()[0];

    expect(first).toBe(1000);
    expect(second).toBe(2000);
  });

  /**
   * A 409 for a suspended card will never succeed. Retrying it forever would
   * block every review queued behind it, so it steps aside and is surfaced.
   */
  it('sets aside a permanently rejected review and keeps going', async () => {
    const accepted: string[] = [];
    const queue = createReviewQueue({
      submit: async (r) => {
        if (r.cardId === 'bad') {
          throw new ApiError('conflict', 'card is suspended and cannot be reviewed', 409);
        }
        accepted.push(r.cardId);
      },
    });

    queue.enqueue(review('bad'));
    queue.enqueue(review('good'));
    await queue.drain();

    expect(accepted).toEqual(['good']);
    expect(queue.getState().pending).toBe(0);

    const failed = queue.getState().failed;
    expect(failed).toHaveLength(1);
    expect(failed[0]?.cardId).toBe('bad');
    expect(failed[0]?.reason).toContain('suspended');
  });

  it('can retry the reviews that were set aside', async () => {
    const accepted: string[] = [];
    let rejecting = true;

    const queue = createReviewQueue({
      submit: async (r) => {
        if (rejecting) throw new ApiError('conflict', 'card is suspended', 409);
        accepted.push(r.cardId);
      },
    });

    queue.enqueue(review('a'));
    await queue.drain();
    expect(queue.getState().failed).toHaveLength(1);

    rejecting = false;
    queue.retryFailed();
    await queue.drain();

    expect(accepted).toEqual(['a']);
    expect(queue.getState().failed).toHaveLength(0);
  });

  it('reports its state as it changes', async () => {
    const seen: ReviewQueueState[] = [];
    const queue = createReviewQueue({
      submit: async () => {},
      onChange: (state) => seen.push(state),
    });

    queue.enqueue(review('a'));
    await queue.drain();

    expect(seen.length).toBeGreaterThan(0);
    expect(seen.some((s) => s.pending === 1)).toBe(true);
    expect(seen[seen.length - 1]?.pending).toBe(0);
  });

  it('drains immediately when nothing is queued', async () => {
    const queue = createReviewQueue({ submit: async () => {} });
    await expect(queue.drain()).resolves.toBeUndefined();
  });
});
