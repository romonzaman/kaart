import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';

import { ApiError, api } from '@/lib/api';
import type {
  Card,
  CardList,
  CreateCardInput,
  CreateDeckInput,
  Deck,
  DeckStats,
  ListCardsParams,
  UpdateCardInput,
  UpdateDeckInput,
} from '@/lib/api';

/**
 * Query keys in one place. Every invalidation in the app refers to these, so a
 * mutation cannot invalidate a key that no query actually uses.
 */
export const keys = {
  decks: ['decks'] as const,
  deck: (id: string) => ['decks', id] as const,
  deckStats: (id: string) => ['decks', id, 'stats'] as const,
  cards: (deckId: string, params: ListCardsParams) =>
    ['decks', deckId, 'cards', params.q ?? '', params.limit ?? 0, params.offset ?? 0] as const,
  cardsForDeck: (deckId: string) => ['decks', deckId, 'cards'] as const,
  card: (id: string) => ['cards', id] as const,
  queue: (deckId: string) => ['decks', deckId, 'queue'] as const,
};

export function useDecks(): UseQueryResult<Deck[], ApiError> {
  return useQuery({
    queryKey: keys.decks,
    queryFn: ({ signal }) => api.listDecks(signal).then((r) => r.decks),
  });
}

export function useDeck(deckId: string | undefined): UseQueryResult<Deck, ApiError> {
  return useQuery({
    queryKey: keys.deck(deckId ?? ''),
    queryFn: ({ signal }) => api.getDeck(deckId as string, signal),
    enabled: deckId !== undefined && deckId !== '',
  });
}

export function useDeckStats(deckId: string | undefined): UseQueryResult<DeckStats, ApiError> {
  return useQuery({
    queryKey: keys.deckStats(deckId ?? ''),
    queryFn: ({ signal }) => api.getStats(deckId as string, undefined, signal),
    enabled: deckId !== undefined && deckId !== '',
  });
}

export function useCards(
  deckId: string | undefined,
  params: ListCardsParams = {},
): UseQueryResult<CardList, ApiError> {
  return useQuery({
    queryKey: keys.cards(deckId ?? '', params),
    queryFn: ({ signal }) => api.listCards(deckId as string, params, signal),
    enabled: deckId !== undefined && deckId !== '',
    placeholderData: (previous) => previous,
  });
}

export function useCard(cardId: string | undefined): UseQueryResult<Card, ApiError> {
  return useQuery({
    queryKey: keys.card(cardId ?? ''),
    queryFn: ({ signal }) => api.getCard(cardId as string, signal),
    enabled: cardId !== undefined && cardId !== '',
  });
}

export function useCreateDeck(): UseMutationResult<Deck, ApiError, CreateDeckInput> {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateDeckInput) => api.createDeck(input),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: keys.decks });
    },
  });
}

export function useUpdateDeck(
  deckId: string,
): UseMutationResult<Deck, ApiError, UpdateDeckInput> {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (input: UpdateDeckInput) => api.updateDeck(deckId, input),
    onSuccess: (deck) => {
      client.setQueryData(keys.deck(deckId), deck);
      void client.invalidateQueries({ queryKey: keys.decks });
    },
  });
}

export function useDeleteDeck(): UseMutationResult<void, ApiError, string> {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (deckId: string) => api.deleteDeck(deckId),
    onSuccess: (_result, deckId) => {
      client.removeQueries({ queryKey: keys.deck(deckId) });
      void client.invalidateQueries({ queryKey: keys.decks });
    },
  });
}

/** The shape the optimistic rollback needs to restore. */
type CardsSnapshot = Array<[readonly unknown[], CardList | undefined]>;

/**
 * Card creation is optimistic.
 *
 * Adding cards is the highest-frequency action in the app — someone entering
 * twenty words in a sitting should never wait on a round trip between them. The
 * placeholder row carries a temporary id; on success it is replaced by the real
 * card, and on failure every touched list is rolled back to its prior contents.
 */
export function useCreateCard(
  deckId: string,
): UseMutationResult<Card, ApiError, CreateCardInput, CardsSnapshot> {
  const client = useQueryClient();

  return useMutation({
    mutationFn: (input: CreateCardInput) => api.createCard(deckId, input),

    onMutate: async (input): Promise<CardsSnapshot> => {
      await client.cancelQueries({ queryKey: keys.cardsForDeck(deckId) });

      const snapshot = client.getQueriesData<CardList>({
        queryKey: keys.cardsForDeck(deckId),
      }) as CardsSnapshot;

      const now = new Date().toISOString();
      const optimistic: Card = {
        id: `optimistic-${now}-${Math.random().toString(36).slice(2)}`,
        deck_id: deckId,
        front: input.front,
        back: input.back,
        hint: input.hint ?? '',
        tags: input.tags ?? [],
        created_at: now,
        updated_at: now,
        suspended_at: null,
      };

      for (const [key] of snapshot) {
        client.setQueryData<CardList>(key, (current) =>
          current === undefined
            ? current
            : { ...current, cards: [...current.cards, optimistic], total: current.total + 1 },
        );
      }

      return snapshot;
    },

    onError: (_error, _input, context) => {
      for (const [key, previous] of context ?? []) {
        client.setQueryData(key, previous);
      }
    },

    onSettled: () => {
      void client.invalidateQueries({ queryKey: keys.cardsForDeck(deckId) });
      void client.invalidateQueries({ queryKey: keys.deckStats(deckId) });
      void client.invalidateQueries({ queryKey: keys.queue(deckId) });
    },
  });
}

export function useUpdateCard(cardId: string): UseMutationResult<Card, ApiError, UpdateCardInput> {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (input: UpdateCardInput) => api.updateCard(cardId, input),
    onSuccess: (card) => {
      client.setQueryData(keys.card(cardId), card);
      void client.invalidateQueries({ queryKey: keys.cardsForDeck(card.deck_id) });
    },
  });
}

export function useDeleteCard(
  deckId: string,
): UseMutationResult<void, ApiError, string> {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (cardId: string) => api.deleteCard(cardId),
    onSuccess: (_result, cardId) => {
      client.removeQueries({ queryKey: keys.card(cardId) });
      void client.invalidateQueries({ queryKey: keys.cardsForDeck(deckId) });
      void client.invalidateQueries({ queryKey: keys.deckStats(deckId) });
    },
  });
}

export function useSetCardSuspended(
  deckId: string,
): UseMutationResult<Card, ApiError, { cardId: string; suspended: boolean }> {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ cardId, suspended }) =>
      suspended ? api.suspendCard(cardId) : api.unsuspendCard(cardId),
    onSuccess: (card) => {
      client.setQueryData(keys.card(card.id), card);
      void client.invalidateQueries({ queryKey: keys.cardsForDeck(deckId) });
      void client.invalidateQueries({ queryKey: keys.deckStats(deckId) });
    },
  });
}
