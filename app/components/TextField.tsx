import { forwardRef } from 'react';
import { StyleSheet, Text, TextInput, View } from 'react-native';
import type { TextInputProps } from 'react-native';

import { useTheme } from './theme';

export type TextFieldProps = Omit<TextInputProps, 'style'> & {
  label: string;
  /** Shown under the field in the danger colour. */
  error?: string;
  /** Shown under the field in muted text when there is no error. */
  help?: string;
};

export const TextField = forwardRef<TextInput, TextFieldProps>(function TextField(
  { label, error, help, multiline, ...inputProps },
  ref,
) {
  const theme = useTheme();

  return (
    <View style={{ gap: theme.spacing.xs }}>
      <Text style={[theme.typography.label, { color: theme.colors.textMuted }]}>{label}</Text>

      <TextInput
        ref={ref}
        multiline={multiline}
        placeholderTextColor={theme.colors.textFaint}
        accessibilityLabel={label}
        style={[
          styles.input,
          theme.typography.body,
          {
            color: theme.colors.text,
            backgroundColor: theme.colors.surface,
            borderColor: error === undefined ? theme.colors.border : theme.colors.danger,
            borderRadius: theme.radius.md,
            paddingHorizontal: theme.spacing.md,
            paddingVertical: theme.spacing.md,
            minHeight: multiline === true ? 96 : 46,
            textAlignVertical: multiline === true ? 'top' : 'center',
          },
        ]}
        {...inputProps}
      />

      {error !== undefined ? (
        <Text style={[theme.typography.caption, { color: theme.colors.danger }]}>{error}</Text>
      ) : help !== undefined ? (
        <Text style={[theme.typography.caption, { color: theme.colors.textFaint }]}>{help}</Text>
      ) : null}
    </View>
  );
});

const styles = StyleSheet.create({
  input: {
    borderWidth: 1,
  },
});
