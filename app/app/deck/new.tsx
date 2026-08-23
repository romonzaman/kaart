import { useCallback, useState } from 'react';
import { View } from 'react-native';
import { useRouter } from 'expo-router';

import { Button, Screen, ScreenError, TextField, useTheme } from '@/components';
import { useCreateDeck } from '@/lib/queries';
import { useKeyboardShortcuts } from '@/lib/keyboard';

export default function NewDeckScreen() {
  const theme = useTheme();
  const router = useRouter();
  const createDeck = useCreateDeck();

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [newPerDay, setNewPerDay] = useState('20');
  const [error, setError] = useState<string | undefined>(undefined);

  const submit = useCallback(() => {
    const trimmed = name.trim();
    if (trimmed === '') {
      setError('Give the deck a name.');
      return;
    }
    const parsed = Number.parseInt(newPerDay, 10);
    if (Number.isNaN(parsed) || parsed < 0 || parsed > 1000) {
      setError('New cards per day must be a number between 0 and 1000.');
      return;
    }
    setError(undefined);

    createDeck.mutate(
      { name: trimmed, description: description.trim(), new_cards_per_day: parsed },
      {
        onSuccess: (deck) => router.replace(`/deck/${deck.id}`),
      },
    );
  }, [createDeck, description, name, newPerDay, router]);

  useKeyboardShortcuts(
    useCallback(
      (event: KeyboardEvent) => {
        if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
          event.preventDefault();
          submit();
        }
      },
      [submit],
    ),
  );

  return (
    <Screen title="New deck" subtitle="You can change any of this later.">
      <View style={{ gap: theme.spacing.lg }}>
        {createDeck.isError ? <ScreenError message={createDeck.error.message} /> : null}

        <TextField
          label="Name"
          value={name}
          onChangeText={setName}
          placeholder="Estonian A1"
          autoFocus
          returnKeyType="next"
          error={error}
        />

        <TextField
          label="Description"
          value={description}
          onChangeText={setDescription}
          placeholder="Beginner vocabulary, everyday words"
          multiline
        />

        <TextField
          label="New cards per day"
          value={newPerDay}
          onChangeText={setNewPerDay}
          keyboardType="number-pad"
          help="How many unseen cards enter your queue each day. Twenty is a sustainable default."
        />

        <View style={{ flexDirection: 'row', gap: theme.spacing.md }}>
          <Button
            label="Create deck"
            onPress={submit}
            loading={createDeck.isPending}
            testID="create-deck"
          />
          <Button label="Cancel" variant="ghost" onPress={() => router.back()} />
        </View>
      </View>
    </Screen>
  );
}
