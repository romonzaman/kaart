import { Text, View } from 'react-native';

import { Button } from './Button';
import { useTheme } from './theme';

export type EmptyStateProps = {
  title: string;
  /**
   * What to do next — not "no cards found". An empty list is a moment where the
   * user has no idea what the app expects of them, so say it.
   */
  body: string;
  actionLabel?: string;
  onAction?: () => void;
  testID?: string;
};

export function EmptyState({ title, body, actionLabel, onAction, testID }: EmptyStateProps) {
  const theme = useTheme();

  return (
    <View
      testID={testID}
      style={{
        paddingVertical: theme.spacing.xxxl,
        paddingHorizontal: theme.spacing.xl,
        gap: theme.spacing.md,
        alignItems: 'center',
        backgroundColor: theme.colors.surfaceAlt,
        borderRadius: theme.radius.lg,
        borderWidth: 1,
        borderColor: theme.colors.border,
      }}
    >
      <Text style={[theme.typography.title, { color: theme.colors.text, textAlign: 'center' }]}>
        {title}
      </Text>
      <Text
        style={[
          theme.typography.body,
          { color: theme.colors.textMuted, textAlign: 'center', maxWidth: 420 },
        ]}
      >
        {body}
      </Text>
      {actionLabel !== undefined && onAction !== undefined ? (
        <Button label={actionLabel} onPress={onAction} />
      ) : null}
    </View>
  );
}
