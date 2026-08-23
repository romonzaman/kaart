import { useCallback, useRef, useState } from 'react';
import { Text, TextInput, View } from 'react-native';
import { useLocalSearchParams, useRouter } from 'expo-router';

import { Button, Screen, ScreenError, TextField, useTheme } from '@/components';
import { useCreateCard } from '@/lib/queries';
import { isTypingTarget, useKeyboardShortcuts } from '@/lib/keyboard';

/**
 * The card entry screen.
 *
 * "Save and add another" is the primary action, not "Save". The flow this
 * screen exists for is typing ten cards in a row, and the difference between
 * clearing the form in place and bouncing back to a list is the difference
 * between that being pleasant and being a chore.
 */
export default function AddCardsScreen() {
  const theme = useTheme();
  const router = useRouter();
  const { id } = useLocalSearchParams<{ id: string }>();
  const deckId = id ?? '';

  const createCard = useCreateCard(deckId);
  const frontRef = useRef<TextInput>(null);

  const [front, setFront] = useState('');
  const [back, setBack] = useState('');
  const [hint, setHint] = useState('');
  const [tags, setTags] = useState('');
  const [error, setError] = useState<string | undefined>(undefined);
  const [savedCount, setSavedCount] = useState(0);

  const save = useCallback(
    (thenClose: boolean) => {
      const trimmedFront = front.trim();
      const trimmedBack = back.trim();
      if (trimmedFront === '' || trimmedBack === '') {
        setError('A card needs both a front and a back.');
        return;
      }
      setError(undefined);

      const parsedTags = tags
        .split(',')
        .map((t) => t.trim())
        .filter((t) => t !== '');

      // Optimistic: the row is already in the list, so clear the form now
      // rather than waiting for the round trip.
      createCard.mutate({
        front: trimmedFront,
        back: trimmedBack,
        hint: hint.trim(),
        tags: parsedTags,
      });

      setSavedCount((n) => n + 1);
      setFront('');
      setBack('');
      setHint('');

      if (thenClose) {
        router.back();
      } else {
        frontRef.current?.focus();
      }
    },
    [back, createCard, front, hint, router, tags],
  );

  useKeyboardShortcuts(
    useCallback(
      (event: KeyboardEvent) => {
        if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
          event.preventDefault();
          save(false);
          return;
        }
        if (event.key === 'Escape' && !isTypingTarget(event)) {
          router.back();
        }
      },
      [router, save],
    ),
  );

  return (
    <Screen
      title="Add cards"
      subtitle={
        savedCount === 0
          ? 'Front, back, and an optional hint. Cmd/Ctrl+Enter saves and clears.'
          : `${savedCount} card${savedCount === 1 ? '' : 's'} added this sitting.`
      }
    >
      <View style={{ gap: theme.spacing.lg }}>
        {createCard.isError ? (
          <ScreenError
            message={`${createCard.error.message} The card was removed from the list.`}
          />
        ) : null}

        <TextField
          ref={frontRef}
          label="Front"
          value={front}
          onChangeText={setFront}
          placeholder="koer"
          autoFocus
          error={error}
        />

        <TextField label="Back" value={back} onChangeText={setBack} placeholder="dog" />

        <TextField
          label="Hint"
          value={hint}
          onChangeText={setHint}
          placeholder="KOH-er"
          help="Optional. Shown alongside the answer."
        />

        <TextField
          label="Tags"
          value={tags}
          onChangeText={setTags}
          placeholder="animals, a1"
          autoCapitalize="none"
          help="Comma separated. Kept between cards so a run of related cards is one keystroke each."
        />

        <View style={{ flexDirection: 'row', gap: theme.spacing.md, flexWrap: 'wrap' }}>
          <Button label="Save and add another" onPress={() => save(false)} testID="save-another" />
          <Button label="Save and close" variant="secondary" onPress={() => save(true)} />
          <Button label="Done" variant="ghost" onPress={() => router.back()} />
        </View>

        <Text style={[theme.typography.caption, { color: theme.colors.textFaint }]}>
          Cards appear in the deck immediately; they finish saving in the background.
        </Text>
      </View>
    </Screen>
  );
}
