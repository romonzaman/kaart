import { useMemo, useState } from 'react';
import { Text, View } from 'react-native';
import { useLocalSearchParams, useRouter } from 'expo-router';

import {
  Button,
  Card as Surface,
  EmptyState,
  Screen,
  ScreenError,
  ScreenLoading,
  TextField,
  useTheme,
} from '@/components';
import { useCards, useSetCardSuspended } from '@/lib/queries';
import type { Card } from '@/lib/api';

const PAGE_SIZE = 25;

export default function CardBrowserScreen() {
  const theme = useTheme();
  const router = useRouter();
  const { id } = useLocalSearchParams<{ id: string }>();
  const deckId = id ?? '';

  const [search, setSearch] = useState('');
  const [page, setPage] = useState(0);

  const params = useMemo(
    () => ({ q: search.trim(), limit: PAGE_SIZE, offset: page * PAGE_SIZE }),
    [page, search],
  );
  const cards = useCards(deckId, params);
  const setSuspended = useSetCardSuspended(deckId);

  const total = cards.data?.total ?? 0;
  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <Screen
      title="Cards"
      subtitle={total === 0 ? undefined : `${total} card${total === 1 ? '' : 's'} in this deck`}
      action={
        <Button label="Add" onPress={() => router.push(`/deck/${deckId}/add`)} />
      }
    >
      <View style={{ gap: theme.spacing.lg }}>
        <TextField
          label="Search"
          value={search}
          onChangeText={(text) => {
            setSearch(text);
            setPage(0);
          }}
          placeholder="Search front, back, and hints"
          autoCapitalize="none"
          autoCorrect={false}
        />

        {cards.isPending ? (
          <ScreenLoading />
        ) : cards.isError ? (
          <ScreenError message={cards.error.message} onRetry={() => void cards.refetch()} />
        ) : cards.data.cards.length === 0 ? (
          <EmptyState
            title={search.trim() === '' ? 'No cards yet' : 'Nothing matches that search'}
            body={
              search.trim() === ''
                ? "Add your first card and it'll appear here, ready for the next session."
                : 'Try a shorter phrase, or clear the search to see the whole deck.'
            }
            actionLabel={search.trim() === '' ? 'Add cards' : 'Clear search'}
            onAction={() => {
              if (search.trim() === '') router.push(`/deck/${deckId}/add`);
              else setSearch('');
            }}
          />
        ) : (
          <View style={{ gap: theme.spacing.sm }}>
            {cards.data.cards.map((card) => (
              <CardRow
                key={card.id}
                card={card}
                onEdit={() => router.push(`/card/${card.id}`)}
                onToggleSuspend={() =>
                  setSuspended.mutate({
                    cardId: card.id,
                    suspended: card.suspended_at === null,
                  })
                }
              />
            ))}
          </View>
        )}

        {pageCount > 1 ? (
          <View
            style={{
              flexDirection: 'row',
              alignItems: 'center',
              justifyContent: 'space-between',
              gap: theme.spacing.md,
            }}
          >
            <Button
              label="Previous"
              variant="secondary"
              disabled={page === 0}
              onPress={() => setPage((p) => Math.max(0, p - 1))}
            />
            <Text style={[theme.typography.caption, { color: theme.colors.textMuted }]}>
              Page {page + 1} of {pageCount}
            </Text>
            <Button
              label="Next"
              variant="secondary"
              disabled={page + 1 >= pageCount}
              onPress={() => setPage((p) => p + 1)}
            />
          </View>
        ) : null}
      </View>
    </Screen>
  );
}

function CardRow({
  card,
  onEdit,
  onToggleSuspend,
}: {
  card: Card;
  onEdit: () => void;
  onToggleSuspend: () => void;
}) {
  const theme = useTheme();
  const suspended = card.suspended_at !== null;
  const pending = card.id.startsWith('optimistic-');

  return (
    <Surface style={{ opacity: pending ? 0.6 : 1 }}>
      <View
        style={{
          flexDirection: 'row',
          alignItems: 'flex-start',
          justifyContent: 'space-between',
          gap: theme.spacing.md,
        }}
      >
        <View style={{ flex: 1, gap: 2 }}>
          <Text style={[theme.typography.heading, { color: theme.colors.text }]}>{card.front}</Text>
          <Text style={[theme.typography.body, { color: theme.colors.textMuted }]}>{card.back}</Text>
          {card.hint === '' ? null : (
            <Text style={[theme.typography.caption, { color: theme.colors.textFaint }]}>
              Hint: {card.hint}
            </Text>
          )}
          {card.tags.length === 0 ? null : (
            <Text style={[theme.typography.caption, { color: theme.colors.textFaint }]}>
              {card.tags.join(' · ')}
            </Text>
          )}
          {suspended ? (
            <Text style={[theme.typography.label, { color: theme.colors.danger }]}>Suspended</Text>
          ) : null}
        </View>

        {pending ? (
          <Text style={[theme.typography.caption, { color: theme.colors.textFaint }]}>Saving…</Text>
        ) : (
          <View style={{ gap: theme.spacing.xs }}>
            <Button label="Edit" variant="ghost" onPress={onEdit} />
            <Button
              label={suspended ? 'Unsuspend' : 'Suspend'}
              variant="ghost"
              onPress={onToggleSuspend}
            />
          </View>
        )}
      </View>
    </Surface>
  );
}
