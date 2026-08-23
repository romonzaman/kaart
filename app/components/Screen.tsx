import { ActivityIndicator, ScrollView, Text, View } from 'react-native';
import type { ReactNode } from 'react';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { Button } from './Button';
import { useTheme } from './theme';

export type ScreenProps = {
  children: ReactNode;
  /** Wraps the content in a ScrollView. Off for full-height screens like study. */
  scroll?: boolean;
  title?: string;
  subtitle?: string;
  /** Rendered to the right of the title. */
  action?: ReactNode;
  testID?: string;
};

/** The page frame: safe-area padding, background, a max width, optional header. */
export function Screen({ children, scroll = true, title, subtitle, action, testID }: ScreenProps) {
  const theme = useTheme();
  const insets = useSafeAreaInsets();

  const header =
    title === undefined ? null : (
      <View
        style={{
          flexDirection: 'row',
          alignItems: 'flex-start',
          justifyContent: 'space-between',
          gap: theme.spacing.md,
          marginBottom: theme.spacing.lg,
        }}
      >
        <View style={{ flex: 1, gap: theme.spacing.xs }}>
          <Text style={[theme.typography.display, { color: theme.colors.text }]}>{title}</Text>
          {subtitle === undefined ? null : (
            <Text style={[theme.typography.body, { color: theme.colors.textMuted }]}>
              {subtitle}
            </Text>
          )}
        </View>
        {action}
      </View>
    );

  const body = (
    <View style={{ width: '100%', maxWidth: 760, alignSelf: 'center', flex: scroll ? 0 : 1 }}>
      {header}
      {children}
    </View>
  );

  const padding = {
    paddingTop: insets.top + theme.spacing.xl,
    paddingBottom: insets.bottom + theme.spacing.xl,
    paddingLeft: insets.left + theme.spacing.lg,
    paddingRight: insets.right + theme.spacing.lg,
  };

  if (!scroll) {
    return (
      <View
        testID={testID}
        style={[{ flex: 1, backgroundColor: theme.colors.background }, padding]}
      >
        {body}
      </View>
    );
  }

  return (
    <ScrollView
      testID={testID}
      style={{ flex: 1, backgroundColor: theme.colors.background }}
      contentContainerStyle={padding}
      keyboardShouldPersistTaps="handled"
    >
      {body}
    </ScrollView>
  );
}

/** Centred spinner for a screen still loading its first data. */
export function ScreenLoading({ label = 'Loading…' }: { label?: string }) {
  const theme = useTheme();
  return (
    <View style={{ padding: theme.spacing.xxxl, alignItems: 'center', gap: theme.spacing.md }}>
      <ActivityIndicator color={theme.colors.accent} />
      <Text style={[theme.typography.caption, { color: theme.colors.textMuted }]}>{label}</Text>
    </View>
  );
}

/** Error state with a retry affordance. */
export function ScreenError({ message, onRetry }: { message: string; onRetry?: () => void }) {
  const theme = useTheme();
  return (
    <View
      style={{
        padding: theme.spacing.xl,
        gap: theme.spacing.md,
        alignItems: 'flex-start',
        backgroundColor: theme.colors.dangerSoft,
        borderRadius: theme.radius.lg,
      }}
    >
      <Text style={[theme.typography.heading, { color: theme.colors.danger }]}>
        Something went wrong
      </Text>
      <Text style={[theme.typography.body, { color: theme.colors.text }]}>{message}</Text>
      {onRetry === undefined ? null : (
        <Button label="Try again" variant="secondary" onPress={onRetry} />
      )}
    </View>
  );
}
