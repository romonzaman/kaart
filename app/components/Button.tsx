import { ActivityIndicator, Pressable, StyleSheet, Text, View } from 'react-native';
import type { StyleProp, ViewStyle } from 'react-native';

import { useTheme } from './theme';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';

export type ButtonProps = {
  label: string;
  onPress: () => void;
  variant?: ButtonVariant;
  disabled?: boolean;
  loading?: boolean;
  /** Small caption under the label — used for the rating buttons' intervals. */
  sublabel?: string;
  /** Overrides the variant's background. The rating buttons use this. */
  tone?: string;
  fullWidth?: boolean;
  style?: StyleProp<ViewStyle>;
  testID?: string;
  accessibilityLabel?: string;
};

export function Button({
  label,
  onPress,
  variant = 'primary',
  disabled = false,
  loading = false,
  sublabel,
  tone,
  fullWidth = false,
  style,
  testID,
  accessibilityLabel,
}: ButtonProps) {
  const theme = useTheme();
  const inactive = disabled || loading;

  const background =
    tone ??
    {
      primary: theme.colors.accent,
      secondary: theme.colors.surfaceAlt,
      ghost: 'transparent',
      danger: theme.colors.dangerSoft,
    }[variant];

  const foreground = tone
    ? '#ffffff'
    : {
        primary: theme.colors.accentText,
        secondary: theme.colors.text,
        ghost: theme.colors.accent,
        danger: theme.colors.danger,
      }[variant];

  const border = variant === 'secondary' ? theme.colors.border : 'transparent';

  return (
    <Pressable
      testID={testID}
      accessibilityRole="button"
      accessibilityLabel={accessibilityLabel ?? label}
      accessibilityState={{ disabled: inactive, busy: loading }}
      disabled={inactive}
      onPress={onPress}
      style={({ pressed }) => [
        styles.base,
        {
          backgroundColor: background,
          borderColor: border,
          borderRadius: theme.radius.md,
          paddingHorizontal: theme.spacing.lg,
          opacity: inactive ? 0.45 : pressed ? 0.8 : 1,
          alignSelf: fullWidth ? 'stretch' : 'flex-start',
        },
        style,
      ]}
    >
      {loading ? (
        <ActivityIndicator color={foreground} />
      ) : (
        <View style={styles.content}>
          <Text style={[theme.typography.heading, { color: foreground }]} numberOfLines={1}>
            {label}
          </Text>
          {sublabel === undefined ? null : (
            <Text
              style={[theme.typography.caption, { color: foreground, opacity: 0.85 }]}
              numberOfLines={1}
            >
              {sublabel}
            </Text>
          )}
        </View>
      )}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  base: {
    minHeight: 46,
    borderWidth: 1,
    alignItems: 'center',
    justifyContent: 'center',
  },
  content: {
    alignItems: 'center',
    justifyContent: 'center',
  },
});
