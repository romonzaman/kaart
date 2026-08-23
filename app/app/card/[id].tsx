import { useCallback, useEffect, useState } from 'react';
import { View } from 'react-native';
import { useLocalSearchParams, useRouter } from 'expo-router';

import {
  Button,
  Screen,
  ScreenError,
  ScreenLoading,
  TextField,
  useTheme,
} from '@/components';
import { useCard, useDeleteCard, useUpdateCard } from '@/lib/queries';
import { isTypingTarget, useKeyboardShortcuts } from '@/lib/keyboard';

export default function CardEditorScreen() {
  const theme = useTheme();
  const router = useRouter();
  const { id } = useLocalSearchParams<{ id: string }>();
  const cardId = id ?? '';

  const card = useCard(cardId);
  const updateCard = useUpdateCard(cardId);
  const deleteCard = useDeleteCard(card.data?.deck_id ?? '');

  const [front, setFront] = useState('');
  const [back, setBack] = useState('');
  const [hint, setHint] = useState('');
  const [tags, setTags] = useState('');
  const [error, setError] = useState<string | undefined>(undefined);
  const [confirmingDelete, setConfirmingDelete] = useState(false);

  // Seed the form once the card arrives.
  useEffect(() => {
    if (card.data === undefined) return;
    setFront(card.data.front);
    setBack(card.data.back);
    setHint(card.data.hint);
    setTags(card.data.tags.join(', '));
  }, [card.data]);

  const save = useCallback(() => {
    const trimmedFront = front.trim();
    const trimmedBack = back.trim();
    if (trimmedFront === '' || trimmedBack === '') {
      setError('A card needs both a front and a back.');
      return;
    }
    setError(undefined);

    updateCard.mutate(
      {
        front: trimmedFront,
        back: trimmedBack,
        hint: hint.trim(),
        tags: tags
          .split(',')
          .map((t) => t.trim())
          .filter((t) => t !== ''),
      },
      { onSuccess: () => router.back() },
    );
  }, [back, front, hint, router, tags, updateCard]);

  useKeyboardShortcuts(
    useCallback(
      (event: KeyboardEvent) => {
        if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
          event.preventDefault();
          save();
          return;
        }
        if (event.key === 'Escape' && !isTypingTarget(event)) {
          router.back();
        }
      },
      [router, save],
    ),
  );

  if (card.isPending) {
    return (
      <Screen>
        <ScreenLoading />
      </Screen>
    );
  }

  if (card.isError) {
    return (
      <Screen>
        <ScreenError message={card.error.message} onRetry={() => void card.refetch()} />
      </Screen>
    );
  }

  return (
    <Screen title="Edit card" subtitle="Editing content never changes the card's schedule.">
      <View style={{ gap: theme.spacing.lg }}>
        {updateCard.isError ? <ScreenError message={updateCard.error.message} /> : null}
        {deleteCard.isError ? <ScreenError message={deleteCard.error.message} /> : null}

        <TextField label="Front" value={front} onChangeText={setFront} error={error} autoFocus />
        <TextField label="Back" value={back} onChangeText={setBack} />
        <TextField label="Hint" value={hint} onChangeText={setHint} />
        <TextField
          label="Tags"
          value={tags}
          onChangeText={setTags}
          autoCapitalize="none"
          help="Comma separated."
        />

        <View style={{ flexDirection: 'row', gap: theme.spacing.md, flexWrap: 'wrap' }}>
          <Button label="Save" onPress={save} loading={updateCard.isPending} testID="save-card" />
          <Button label="Cancel" variant="ghost" onPress={() => router.back()} />
        </View>

        <View style={{ gap: theme.spacing.sm, marginTop: theme.spacing.xl }}>
          {confirmingDelete ? (
            <View style={{ flexDirection: 'row', gap: theme.spacing.md, flexWrap: 'wrap' }}>
              <Button
                label="Delete permanently"
                variant="danger"
                loading={deleteCard.isPending}
                onPress={() =>
                  deleteCard.mutate(cardId, {
                    onSuccess: () => router.back(),
                  })
                }
              />
              <Button
                label="Keep it"
                variant="ghost"
                onPress={() => setConfirmingDelete(false)}
              />
            </View>
          ) : (
            <Button
              label="Delete card"
              variant="danger"
              onPress={() => setConfirmingDelete(true)}
            />
          )}
        </View>
      </View>
    </Screen>
  );
}
