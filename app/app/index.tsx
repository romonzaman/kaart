import { View, Text } from 'react-native';
import { useRouter } from 'expo-router';

import {
  Button,
  Card,
  EmptyState,
  Screen,
  ScreenError,
  ScreenLoading,
  useTheme,
} from '@/components';
import { useDecks } from '@/lib/queries';
import type { Deck } from '@/lib/api';

export default function DeckListScreen() {
  const theme = useTheme();
  const router = useRouter();
  const decks = useDecks();

  return (
    <Screen
      title="Decks"
      subtitle="Pick a deck to study, or add a new one."
      action={<Button label="New deck" onPress={() => router.push('/deck/new')} />}
    >
      {decks.isPending ? (
        <ScreenLoading label="Loading your decks…" />
      ) : decks.isError ? (
        <ScreenError message={decks.error.message} onRetry={() => void decks.refetch()} />
      ) : decks.data.length === 0 ? (
        <EmptyState
          title="No decks yet"
          body="A deck is a set of cards you study together — one language, one subject, one exam. Create your first one and start adding cards."
          actionLabel="Create a deck"
          onAction={() => router.push('/deck/new')}
        />
      ) : (
        <View style={{ gap: theme.spacing.md }}>
          {decks.data.map((deck) => (
            <DeckRow key={deck.id} deck={deck} onPress={() => router.push(`/deck/${deck.id}`)} />
          ))}
        </View>
      )}
    </Screen>
  );
}

function DeckRow({ deck, onPress }: { deck: Deck; onPress: () => void }) {
  const theme = useTheme();

  return (
    <Card onPress={onPress} accessibilityLabel={`Open deck ${deck.name}`}>
      <Text style={[theme.typography.title, { color: theme.colors.text }]}>{deck.name}</Text>
      {deck.description === '' ? null : (
        <Text style={[theme.typography.body, { color: theme.colors.textMuted }]} numberOfLines={2}>
          {deck.description}
        </Text>
      )}
      <Text style={[theme.typography.caption, { color: theme.colors.textFaint }]}>
        {deck.new_cards_per_day} new/day · {deck.max_reviews_per_day} reviews/day · retention{' '}
        {Math.round(deck.desired_retention * 100)}%
      </Text>
    </Card>
  );
}
