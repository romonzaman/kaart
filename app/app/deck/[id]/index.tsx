import { Text, View } from 'react-native';
import { useLocalSearchParams, useRouter } from 'expo-router';

import {
  Button,
  Card,
  EmptyState,
  Screen,
  ScreenError,
  ScreenLoading,
  Stat,
  StatRow,
  useTheme,
} from '@/components';
import { useDeck, useDeckStats } from '@/lib/queries';

export default function DeckDetailScreen() {
  const theme = useTheme();
  const router = useRouter();
  const { id } = useLocalSearchParams<{ id: string }>();
  const deckId = id ?? '';

  const deck = useDeck(deckId);
  const stats = useDeckStats(deckId);

  if (deck.isPending) {
    return (
      <Screen>
        <ScreenLoading />
      </Screen>
    );
  }

  if (deck.isError) {
    return (
      <Screen>
        <ScreenError message={deck.error.message} onRetry={() => void deck.refetch()} />
      </Screen>
    );
  }

  const counts = stats.data;
  const dueNow = counts?.due_now ?? 0;
  const hasCards = (counts?.total_cards ?? 0) > 0;

  return (
    <Screen
      title={deck.data.name}
      subtitle={deck.data.description === '' ? undefined : deck.data.description}
    >
      <View style={{ gap: theme.spacing.xl }}>
        {stats.isError ? (
          <ScreenError message={stats.error.message} onRetry={() => void stats.refetch()} />
        ) : (
          <Card>
            <StatRow>
              <Stat label="Due now" value={dueNow} tone={theme.colors.stateReview} testID="due-now" />
              <Stat label="New" value={counts?.new ?? 0} tone={theme.colors.stateNew} />
              <Stat
                label="Learning"
                value={counts?.learning ?? 0}
                tone={theme.colors.stateLearning}
              />
              <Stat label="Total" value={counts?.total_cards ?? 0} />
            </StatRow>

            <Text
              style={[
                theme.typography.caption,
                { color: theme.colors.textFaint, marginTop: theme.spacing.md },
              ]}
            >
              {counts === undefined
                ? ' '
                : `${counts.reviews_today} reviewed today · ${counts.remaining_new_today} new cards left in today's allowance`}
            </Text>
          </Card>
        )}

        {!hasCards ? (
          <EmptyState
            title="This deck is empty"
            body="Add a few cards and they'll be waiting for you in the queue. The fastest way in is Add cards — it keeps the form open so you can type several in a row."
            actionLabel="Add cards"
            onAction={() => router.push(`/deck/${deckId}/add`)}
          />
        ) : (
          <View style={{ gap: theme.spacing.md }}>
            <Button
              label={dueNow > 0 ? `Study ${dueNow} card${dueNow === 1 ? '' : 's'}` : 'Study'}
              onPress={() => router.push(`/deck/${deckId}/study`)}
              fullWidth
              testID="study"
            />
            <View style={{ flexDirection: 'row', gap: theme.spacing.md, flexWrap: 'wrap' }}>
              <Button
                label="Add cards"
                variant="secondary"
                onPress={() => router.push(`/deck/${deckId}/add`)}
              />
              <Button
                label="Browse cards"
                variant="secondary"
                onPress={() => router.push(`/deck/${deckId}/cards`)}
              />
            </View>
          </View>
        )}

        <Card>
          <Text style={[theme.typography.heading, { color: theme.colors.text }]}>Settings</Text>
          <Text style={[theme.typography.body, { color: theme.colors.textMuted }]}>
            {deck.data.new_cards_per_day} new cards/day · {deck.data.max_reviews_per_day} reviews/day
          </Text>
          <Text style={[theme.typography.body, { color: theme.colors.textMuted }]}>
            Target retention {Math.round(deck.data.desired_retention * 100)}% — higher means shorter
            intervals and more reviews.
          </Text>
        </Card>
      </View>
    </Screen>
  );
}
