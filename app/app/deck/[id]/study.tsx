import { useCallback, useEffect, useMemo, useReducer, useRef, useState } from 'react';
import { Pressable, ScrollView, Text, View } from 'react-native';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { useQuery, useQueryClient } from '@tanstack/react-query';

import {
  Button,
  Card as Surface,
  EmptyState,
  Screen,
  ScreenError,
  ScreenLoading,
  Stat,
  StatRow,
  useTheme,
} from '@/components';
import { ApiError, api } from '@/lib/api';
import type { Preview, Queue, QueueItem, RatingValue } from '@/lib/api';
import { keys } from '@/lib/queries';
import { isTypingTarget, useKeyboardShortcuts } from '@/lib/keyboard';
import { createReviewQueue } from '@/lib/reviewQueue';
import type { ReviewQueueState } from '@/lib/reviewQueue';
import {
  currentItem,
  formatDuration,
  formatRelativeFuture,
  ratingBreakdown,
  remainingCounts,
  sessionDurationMs,
  sessionReducer,
  startSession,
} from '@/lib/session';

const EMPTY_SESSION = startSession([], 0);

export default function StudyScreen() {
  const theme = useTheme();
  const router = useRouter();
  const client = useQueryClient();
  const { id } = useLocalSearchParams<{ id: string }>();
  const deckId = id ?? '';

  // The queue is fetched once and then owned by local state. Refetching
  // mid-session would reshuffle the cards under the user's hands.
  const queue = useQuery<Queue, ApiError>({
    queryKey: keys.queue(deckId),
    queryFn: ({ signal }) => api.getQueue(deckId, 50, signal),
    enabled: deckId !== '',
    staleTime: Infinity,
    refetchOnMount: 'always',
  });

  const stats = useQuery({
    queryKey: keys.deckStats(deckId),
    queryFn: ({ signal }) => api.getStats(deckId, undefined, signal),
    enabled: deckId !== '',
  });

  const [session, dispatch] = useReducer(sessionReducer, EMPTY_SESSION);
  const [started, setStarted] = useState(false);
  const [confirmingExit, setConfirmingExit] = useState(false);
  const [outbox, setOutbox] = useState<ReviewQueueState>({ pending: 0, failed: [], busy: false });

  const reviewQueue = useMemo(
    () =>
      createReviewQueue({
        submit: (review) =>
          api.review(review.cardId, { rating: review.rating, duration_ms: review.durationMs }),
        onChange: setOutbox,
      }),
    [],
  );

  // Seed the session the first time the queue arrives.
  useEffect(() => {
    if (queue.data === undefined || started) return;
    dispatch({ type: 'start', items: queue.data.items, at: Date.now() });
    setStarted(true);
  }, [queue.data, started]);

  // Once the session ends, let the rest of the app know the deck moved on.
  useEffect(() => {
    if (session.phase !== 'summary' || !started) return;
    void client.invalidateQueries({ queryKey: keys.deckStats(deckId) });
    void client.invalidateQueries({ queryKey: keys.decks });
  }, [client, deckId, session.phase, started]);

  const item = currentItem(session);
  const remaining = remainingCounts(session);

  const reveal = useCallback(() => dispatch({ type: 'reveal', at: Date.now() }), []);

  const rate = useCallback(
    (rating: RatingValue) => {
      const at = Date.now();
      const card = currentItem(session);
      if (card === undefined || session.phase !== 'answer') return;

      const durationMs = session.revealedAt === null ? 0 : at - session.revealedAt;

      // Advance first, submit second. The user is already looking at the next
      // card by the time the request leaves.
      dispatch({ type: 'rate', rating, at });
      reviewQueue.enqueue({
        cardId: card.card.id,
        rating,
        durationMs: Math.max(0, Math.round(durationMs)),
      });
    },
    [reviewQueue, session],
  );

  const exit = useCallback(() => {
    if (session.phase !== 'summary' && session.results.length > 0 && !confirmingExit) {
      setConfirmingExit(true);
      return;
    }
    router.back();
  }, [confirmingExit, router, session.phase, session.results.length]);

  const restart = useCallback(() => {
    setStarted(false);
    setConfirmingExit(false);
    void queue.refetch();
  }, [queue]);

  useKeyboardShortcuts(
    useCallback(
      (event: KeyboardEvent) => {
        if (isTypingTarget(event)) return;

        if (event.key === 'Escape') {
          event.preventDefault();
          exit();
          return;
        }
        if (session.phase === 'question' && (event.key === ' ' || event.key === 'Enter')) {
          event.preventDefault();
          reveal();
          return;
        }
        if (session.phase === 'answer' && ['1', '2', '3', '4'].includes(event.key)) {
          event.preventDefault();
          rate(Number.parseInt(event.key, 10) as RatingValue);
        }
      },
      [exit, rate, reveal, session.phase],
    ),
  );

  if (queue.isPending) {
    return (
      <Screen>
        <ScreenLoading label="Building your queue…" />
      </Screen>
    );
  }

  if (queue.isError) {
    return (
      <Screen>
        <ScreenError message={queue.error.message} onRetry={() => void queue.refetch()} />
      </Screen>
    );
  }

  if (session.phase === 'summary') {
    return (
      <SummaryView
        session={session}
        outbox={outbox}
        nextDue={stats.data?.next_due ?? null}
        onRetryFailed={() => reviewQueue.retryFailed()}
        onStudyAgain={restart}
        onDone={() => router.back()}
      />
    );
  }

  if (item === undefined) {
    return (
      <Screen>
        <ScreenLoading />
      </Screen>
    );
  }

  return (
    <Screen scroll={false}>
      <View style={{ flex: 1, gap: theme.spacing.lg }}>
        <ProgressBar
          remaining={remaining}
          outbox={outbox}
          onExit={exit}
          confirming={confirmingExit}
          onConfirmExit={() => router.back()}
          onCancelExit={() => setConfirmingExit(false)}
        />

        <Pressable
          onPress={session.phase === 'question' ? reveal : undefined}
          accessibilityRole={session.phase === 'question' ? 'button' : undefined}
          accessibilityLabel={session.phase === 'question' ? 'Show answer' : undefined}
          style={{ flex: 1 }}
          testID="card-face"
        >
          <Surface style={{ flex: 1, justifyContent: 'center', alignItems: 'center' }}>
            <ScrollView
              contentContainerStyle={{
                flexGrow: 1,
                justifyContent: 'center',
                alignItems: 'center',
                gap: theme.spacing.xl,
                padding: theme.spacing.lg,
              }}
            >
              <Text
                style={[
                  theme.typography.display,
                  { color: theme.colors.text, textAlign: 'center' },
                ]}
                testID="card-front"
              >
                {item.card.front}
              </Text>

              {session.phase === 'answer' ? (
                <>
                  <View
                    style={{
                      height: 1,
                      alignSelf: 'stretch',
                      backgroundColor: theme.colors.border,
                    }}
                  />
                  <Text
                    style={[
                      theme.typography.title,
                      { color: theme.colors.text, textAlign: 'center' },
                    ]}
                    testID="card-back"
                  >
                    {item.card.back}
                  </Text>
                  {item.card.hint === '' ? null : (
                    <Text
                      style={[
                        theme.typography.body,
                        { color: theme.colors.textMuted, textAlign: 'center' },
                      ]}
                    >
                      {item.card.hint}
                    </Text>
                  )}
                </>
              ) : (
                <Text style={[theme.typography.caption, { color: theme.colors.textFaint }]}>
                  Tap the card or press Space to show the answer
                </Text>
              )}
            </ScrollView>
          </Surface>
        </Pressable>

        {session.phase === 'question' ? (
          <Button label="Show answer" onPress={reveal} fullWidth testID="show-answer" />
        ) : (
          <RatingButtons previews={item.previews} onRate={rate} />
        )}
      </View>
    </Screen>
  );
}

function ProgressBar({
  remaining,
  outbox,
  onExit,
  confirming,
  onConfirmExit,
  onCancelExit,
}: {
  remaining: ReturnType<typeof remainingCounts>;
  outbox: ReviewQueueState;
  onExit: () => void;
  confirming: boolean;
  onConfirmExit: () => void;
  onCancelExit: () => void;
}) {
  const theme = useTheme();

  if (confirming) {
    return (
      <Surface style={{ backgroundColor: theme.colors.surfaceAlt }}>
        <Text style={[theme.typography.heading, { color: theme.colors.text }]}>
          Leave this session?
        </Text>
        <Text style={[theme.typography.body, { color: theme.colors.textMuted }]}>
          The cards you have already rated are saved. The rest stay due.
        </Text>
        <View style={{ flexDirection: 'row', gap: theme.spacing.md, marginTop: theme.spacing.sm }}>
          <Button label="Leave" variant="danger" onPress={onConfirmExit} />
          <Button label="Keep studying" onPress={onCancelExit} />
        </View>
      </Surface>
    );
  }

  return (
    <View
      style={{
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: theme.spacing.md,
      }}
    >
      <View style={{ flexDirection: 'row', gap: theme.spacing.lg, alignItems: 'baseline' }}>
        <Text style={[theme.typography.title, { color: theme.colors.text }]} testID="remaining">
          {remaining.total} left
        </Text>
        <Text style={[theme.typography.caption, { color: theme.colors.textMuted }]}>
          {remaining.new} new · {remaining.learning} learning · {remaining.review} review
        </Text>
      </View>

      <View style={{ alignItems: 'flex-end' }}>
        {outbox.pending > 0 ? (
          <Text style={[theme.typography.caption, { color: theme.colors.textFaint }]}>
            saving {outbox.pending}…
          </Text>
        ) : null}
        <Button label="Exit" variant="ghost" onPress={onExit} />
      </View>
    </View>
  );
}

function RatingButtons({
  previews,
  onRate,
}: {
  previews: Preview[];
  onRate: (rating: RatingValue) => void;
}) {
  const theme = useTheme();
  const tones: Record<RatingValue, string> = {
    1: theme.colors.again,
    2: theme.colors.hard,
    3: theme.colors.good,
    4: theme.colors.easy,
  };
  const labels: Record<RatingValue, string> = {
    1: 'Again',
    2: 'Hard',
    3: 'Good',
    4: 'Easy',
  };

  return (
    <View style={{ flexDirection: 'row', gap: theme.spacing.sm }}>
      {previews.map((preview) => (
        <Button
          key={preview.rating}
          label={labels[preview.rating]}
          // The interval comes from the server's own scheduler, never a
          // hardcoded guess — it is what will actually be written.
          sublabel={preview.label}
          tone={tones[preview.rating]}
          onPress={() => onRate(preview.rating)}
          style={{ flex: 1 }}
          testID={`rate-${preview.rating}`}
          accessibilityLabel={`${labels[preview.rating]}, next in ${preview.label}`}
        />
      ))}
    </View>
  );
}

function SummaryView({
  session,
  outbox,
  nextDue,
  onRetryFailed,
  onStudyAgain,
  onDone,
}: {
  session: ReturnType<typeof startSession>;
  outbox: ReviewQueueState;
  nextDue: string | null;
  onRetryFailed: () => void;
  onStudyAgain: () => void;
  onDone: () => void;
}) {
  const theme = useTheme();
  const breakdown = ratingBreakdown(session);
  const reviewed = session.results.length;

  if (reviewed === 0) {
    return (
      <Screen title="Nothing due">
        <View style={{ gap: theme.spacing.lg }}>
          <EmptyState
            title="You're caught up"
            body={
              nextDue === null
                ? 'No cards are waiting for you right now. Add some new ones, or come back after your next interval comes around.'
                : `No cards are waiting right now. The next one comes up ${formatRelativeFuture(nextDue, Date.now())}.`
            }
            actionLabel="Back to deck"
            onAction={onDone}
          />
        </View>
      </Screen>
    );
  }

  return (
    <Screen title="Session complete">
      <View style={{ gap: theme.spacing.xl }}>
        <Surface>
          <StatRow>
            <Stat label="Reviewed" value={reviewed} testID="reviewed-count" />
            <Stat
              label="Time"
              value={formatDuration(sessionDurationMs(session, Date.now()))}
            />
          </StatRow>
        </Surface>

        <Surface>
          <Text
            style={[
              theme.typography.heading,
              { color: theme.colors.text, marginBottom: theme.spacing.md },
            ]}
          >
            How it went
          </Text>
          <StatRow>
            <Stat label="Again" value={breakdown.again} tone={theme.colors.again} />
            <Stat label="Hard" value={breakdown.hard} tone={theme.colors.hard} />
            <Stat label="Good" value={breakdown.good} tone={theme.colors.good} />
            <Stat label="Easy" value={breakdown.easy} tone={theme.colors.easy} />
          </StatRow>
        </Surface>

        {outbox.pending > 0 ? (
          <Surface style={{ backgroundColor: theme.colors.accentSoft }}>
            <Text style={[theme.typography.body, { color: theme.colors.text }]}>
              Saving {outbox.pending} review{outbox.pending === 1 ? '' : 's'}… They'll go through as
              soon as the server answers. You can leave this screen.
            </Text>
          </Surface>
        ) : null}

        {outbox.failed.length > 0 ? (
          <Surface style={{ backgroundColor: theme.colors.dangerSoft }}>
            <Text style={[theme.typography.heading, { color: theme.colors.danger }]}>
              {outbox.failed.length} review{outbox.failed.length === 1 ? '' : 's'} were rejected
            </Text>
            <Text style={[theme.typography.body, { color: theme.colors.text }]}>
              {outbox.failed[0]?.reason ?? ''}
            </Text>
            <Button
              label="Try again"
              variant="secondary"
              onPress={onRetryFailed}
              style={{ marginTop: theme.spacing.sm }}
            />
          </Surface>
        ) : null}

        <View style={{ flexDirection: 'row', gap: theme.spacing.md, flexWrap: 'wrap' }}>
          <Button label="Study again" onPress={onStudyAgain} testID="study-again" />
          <Button label="Back to deck" variant="secondary" onPress={onDone} />
        </View>
      </View>
    </Screen>
  );
}

export type { QueueItem };
