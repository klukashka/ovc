import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, test } from 'vitest';

const styles = readFileSync(resolve(import.meta.dirname, '../src/styles.css'), 'utf8');

const paletteKeys = ['clay', 'slate', 'moss', 'plum', 'ochre', 'teal', 'rust', 'ink'] as const;
const stateKeys = ['info', 'success', 'warning', 'danger', 'live', 'accent'] as const;
const backgroundKeys = ['bg', 'surface'] as const;
const minimumPaletteDistance = 20;
const minimumStateDistance = 12;
const minimumContrast = 4.5;

type Theme = 'light' | 'dark';

interface Rgb {
  readonly red: number;
  readonly green: number;
  readonly blue: number;
}

interface Lab {
  readonly lightness: number;
  readonly a: number;
  readonly b: number;
}

function themeBlock(theme: Theme): string {
  const darkStart = styles.indexOf("[data-theme='dark'] {");
  if (darkStart < 0) {
    throw new Error('styles.css has no dark theme block');
  }
  if (theme === 'light') {
    return styles.slice(0, darkStart);
  }
  return styles.slice(darkStart, styles.indexOf('}', darkStart));
}

function colourOf(theme: Theme, key: string): Rgb {
  const match = new RegExp(`--color-${key}: #([0-9a-f]{6});`).exec(themeBlock(theme));
  const hex = match?.[1];
  if (hex === undefined) {
    throw new Error(`styles.css has no ${theme} --color-${key}`);
  }
  return {
    red: Number.parseInt(hex.slice(0, 2), 16) / 255,
    green: Number.parseInt(hex.slice(2, 4), 16) / 255,
    blue: Number.parseInt(hex.slice(4, 6), 16) / 255,
  };
}

function linear(channel: number): number {
  return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;
}

function relativeLuminance(colour: Rgb): number {
  return 0.2126 * linear(colour.red) + 0.7152 * linear(colour.green) + 0.0722 * linear(colour.blue);
}

function contrast(first: Rgb, second: Rgb): number {
  const [lighter, darker] = [relativeLuminance(first), relativeLuminance(second)].sort((x, y) => y - x);
  return ((lighter ?? 0) + 0.05) / ((darker ?? 0) + 0.05);
}

function labOf(colour: Rgb): Lab {
  const red = linear(colour.red);
  const green = linear(colour.green);
  const blue = linear(colour.blue);
  const x = (red * 0.4124 + green * 0.3576 + blue * 0.1805) / 0.95047;
  const y = red * 0.2126 + green * 0.7152 + blue * 0.0722;
  const z = (red * 0.0193 + green * 0.1192 + blue * 0.9505) / 1.08883;
  const pivot = (value: number): number => (value > 0.008856 ? Math.cbrt(value) : 7.787 * value + 16 / 116);
  return { lightness: 116 * pivot(y) - 16, a: 500 * (pivot(x) - pivot(y)), b: 200 * (pivot(y) - pivot(z)) };
}

function distance(first: Rgb, second: Rgb): number {
  const one = labOf(first);
  const other = labOf(second);
  return Math.hypot(one.lightness - other.lightness, one.a - other.a, one.b - other.b);
}

function closestPaletteDistance(theme: Theme): number {
  const distances = paletteKeys.flatMap((first, index) =>
    paletteKeys.slice(index + 1).map((second) => distance(colourOf(theme, first), colourOf(theme, second))),
  );
  return Math.min(...distances);
}

function closestStateDistance(theme: Theme): number {
  return Math.min(
    ...paletteKeys.flatMap((key) => stateKeys.map((state) => distance(colourOf(theme, key), colourOf(theme, state)))),
  );
}

function lowestContrast(theme: Theme): number {
  return Math.min(
    ...paletteKeys.flatMap((key) =>
      backgroundKeys.map((background) => contrast(colourOf(theme, key), colourOf(theme, background))),
    ),
  );
}

describe('the tutor and group palette', () => {
  test.each(['light', 'dark'] as const)(
    'every palette pair is at least 20 apart and every value at least 12 from a state colour in the %s theme',
    (theme) => {
      const paletteDistance = closestPaletteDistance(theme);
      const stateDistance = closestStateDistance(theme);

      expect(paletteDistance).toBeGreaterThanOrEqual(minimumPaletteDistance);
      expect(stateDistance).toBeGreaterThanOrEqual(minimumStateDistance);
    },
  );

  test.each(['light', 'dark'] as const)(
    'every palette value keeps 4.5:1 on bg and surface in the %s theme',
    (theme) => {
      const lowest = lowestContrast(theme);

      expect(lowest).toBeGreaterThanOrEqual(minimumContrast);
    },
  );
});
