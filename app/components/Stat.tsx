import { Text, View } from 'react-native';

import { useTheme } from './theme';

export type StatProps = {
  label: string;
  value: number | string;
  /** Accent colour for the value — the deck screen colours new/learning/due. */
  tone?: string;
  testID?: string;
};

export function Stat({ label, value, tone, testID }: StatProps) {
  const theme = useTheme();

  return (
    <View testID={testID} style={{ gap: 2, minWidth: 72 }}>
      <Text style={[theme.typography.display, { color: tone ?? theme.colors.text }]}>{value}</Text>
      <Text style={[theme.typography.label, { color: theme.colors.textMuted }]}>{label}</Text>
    </View>
  );
}

/** A row of Stats that wraps rather than overflowing on a narrow phone. */
export function StatRow({ children }: { children: React.ReactNode }) {
  const theme = useTheme();
  return (
    <View
      style={{
        flexDirection: 'row',
        flexWrap: 'wrap',
        gap: theme.spacing.xl,
        rowGap: theme.spacing.lg,
      }}
    >
      {children}
    </View>
  );
}
