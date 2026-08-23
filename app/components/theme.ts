import { useColorScheme } from 'react-native';
import type { TextStyle } from 'react-native';

/**
 * The whole visual system lives here. Components read tokens from useTheme();
 * no component hardcodes a colour, a radius, or a font size.
 */

export type Palette = {
  background: string;
  surface: string;
  surfaceAlt: string;
  border: string;
  borderStrong: string;
  text: string;
  textMuted: string;
  textFaint: string;
  accent: string;
  accentText: string;
  accentSoft: string;
  danger: string;
  dangerSoft: string;
  /** Rating button colours, in Again/Hard/Good/Easy order. */
  again: string;
  hard: string;
  good: string;
  easy: string;
  /** Card-state accents used by counts and badges. */
  stateNew: string;
  stateLearning: string;
  stateReview: string;
};

const light: Palette = {
  background: '#f6f6f4',
  surface: '#ffffff',
  surfaceAlt: '#f0efec',
  border: '#e2e0da',
  borderStrong: '#c9c6bd',
  text: '#1b1a17',
  textMuted: '#63605a',
  textFaint: '#8f8b83',
  accent: '#1f6f5c',
  accentText: '#ffffff',
  accentSoft: '#e3f0ec',
  danger: '#9c2f2f',
  dangerSoft: '#f7e6e6',
  again: '#a33b3b',
  hard: '#9a6b1f',
  good: '#1f6f5c',
  easy: '#2f5f9c',
  stateNew: '#2f5f9c',
  stateLearning: '#9a6b1f',
  stateReview: '#1f6f5c',
};

const dark: Palette = {
  background: '#141513',
  surface: '#1d1f1c',
  surfaceAlt: '#262824',
  border: '#33352f',
  borderStrong: '#4a4d45',
  text: '#f0efea',
  textMuted: '#a8a59c',
  textFaint: '#7b7871',
  accent: '#4fb89a',
  accentText: '#0d1512',
  accentSoft: '#1c332c',
  danger: '#e07a7a',
  dangerSoft: '#33201f',
  again: '#e07a7a',
  hard: '#d9a95c',
  good: '#4fb89a',
  easy: '#7ba9de',
  stateNew: '#7ba9de',
  stateLearning: '#d9a95c',
  stateReview: '#4fb89a',
};

export const spacing = {
  xs: 4,
  sm: 8,
  md: 12,
  lg: 16,
  xl: 24,
  xxl: 32,
  xxxl: 48,
} as const;

export const radius = {
  sm: 6,
  md: 10,
  lg: 16,
  pill: 999,
} as const;

export const typography = {
  display: { fontSize: 30, lineHeight: 36, fontWeight: '700' },
  title: { fontSize: 22, lineHeight: 28, fontWeight: '700' },
  heading: { fontSize: 17, lineHeight: 23, fontWeight: '600' },
  body: { fontSize: 16, lineHeight: 23, fontWeight: '400' },
  label: { fontSize: 13, lineHeight: 18, fontWeight: '600' },
  caption: { fontSize: 13, lineHeight: 18, fontWeight: '400' },
  mono: { fontSize: 15, lineHeight: 21, fontWeight: '500' },
} satisfies Record<string, TextStyle>;

export type Theme = {
  colors: Palette;
  spacing: typeof spacing;
  radius: typeof radius;
  typography: typeof typography;
  dark: boolean;
};

/** The app's palette for the device's current appearance setting. */
export function useTheme(): Theme {
  const scheme = useColorScheme();
  const isDark = scheme === 'dark';
  return {
    colors: isDark ? dark : light,
    spacing,
    radius,
    typography,
    dark: isDark,
  };
}

/** Exported for tests and for screens that need a palette outside a component. */
export const palettes = { light, dark };
