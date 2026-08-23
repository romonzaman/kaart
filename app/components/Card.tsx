import { Pressable, View } from 'react-native';
import type { ReactNode } from 'react';
import type { StyleProp, ViewStyle } from 'react-native';

import { useTheme } from './theme';

export type CardProps = {
  children: ReactNode;
  onPress?: () => void;
  style?: StyleProp<ViewStyle>;
  testID?: string;
  accessibilityLabel?: string;
};

/** A surface. Pressable when onPress is given, a plain View otherwise. */
export function Card({ children, onPress, style, testID, accessibilityLabel }: CardProps) {
  const theme = useTheme();

  const surface: StyleProp<ViewStyle> = [
    {
      backgroundColor: theme.colors.surface,
      borderColor: theme.colors.border,
      borderWidth: 1,
      borderRadius: theme.radius.lg,
      padding: theme.spacing.lg,
      gap: theme.spacing.xs,
    },
    style,
  ];

  if (onPress === undefined) {
    return (
      <View testID={testID} style={surface}>
        {children}
      </View>
    );
  }

  return (
    <Pressable
      testID={testID}
      accessibilityRole="button"
      accessibilityLabel={accessibilityLabel}
      onPress={onPress}
      style={({ pressed }) => [surface, { opacity: pressed ? 0.75 : 1 }]}
    >
      {children}
    </Pressable>
  );
}
