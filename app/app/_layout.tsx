import { useMemo } from 'react';
import { useColorScheme } from 'react-native';
import { Stack } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import { SafeAreaProvider } from 'react-native-safe-area-context';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import { ApiError } from '@/lib/api';
import { palettes } from '@/components';

function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 15_000,
        // A 404 or a validation failure will never succeed on retry; only a
        // dropped connection or a server fault is worth trying again.
        retry: (failureCount, error) =>
          error instanceof ApiError && error.retryable && failureCount < 2,
        refetchOnWindowFocus: false,
      },
      mutations: {
        retry: false,
      },
    },
  });
}

export default function RootLayout() {
  const scheme = useColorScheme();
  const client = useMemo(makeQueryClient, []);
  const colors = scheme === 'dark' ? palettes.dark : palettes.light;

  return (
    <QueryClientProvider client={client}>
      <SafeAreaProvider>
        <StatusBar style={scheme === 'dark' ? 'light' : 'dark'} />
        <Stack
          screenOptions={{
            headerStyle: { backgroundColor: colors.background },
            headerTintColor: colors.text,
            headerTitleStyle: { color: colors.text },
            headerShadowVisible: false,
            contentStyle: { backgroundColor: colors.background },
          }}
        >
          <Stack.Screen name="index" options={{ title: 'Kaart' }} />
          <Stack.Screen name="deck/new" options={{ title: 'New deck', presentation: 'modal' }} />
          <Stack.Screen name="deck/[id]/index" options={{ title: 'Deck' }} />
          <Stack.Screen name="deck/[id]/cards" options={{ title: 'Cards' }} />
          <Stack.Screen name="deck/[id]/add" options={{ title: 'Add cards' }} />
          <Stack.Screen
            name="deck/[id]/study"
            options={{ title: 'Study', headerBackVisible: false }}
          />
          <Stack.Screen name="card/[id]" options={{ title: 'Edit card' }} />
        </Stack>
      </SafeAreaProvider>
    </QueryClientProvider>
  );
}
