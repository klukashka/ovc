import { readdirSync, readFileSync } from 'node:fs';
import { join, resolve } from 'node:path';

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

interface ColourPair {
  readonly foreground: string;
  readonly background: string;
}

const textSurfaces = ['bg', 'surface', 'field', 'sidebar', 'hover'] as const;
const controlBackgrounds = ['bg', 'surface', 'field', 'sidebar', 'hover'] as const;
const minimumBoundaryContrast = 3;

function pairs(foregrounds: readonly string[], backgrounds: readonly string[]): ColourPair[] {
  return foregrounds.flatMap((foreground) => backgrounds.map((background) => ({ foreground, background })));
}

function textPairs(theme: Theme): ColourPair[] {
  const mutedOnMine = theme === 'dark' ? pairs(['text-muted'], ['mine']) : [];
  return [
    ...pairs(['text', 'text-muted', 'accent-text'], textSurfaces),
    ...pairs(['text', 'accent-text'], ['mine']),
    ...mutedOnMine,
    ...pairs(['success', 'warning', 'info'], ['bg', 'surface', 'sidebar']),
    ...pairs(['danger'], ['bg', 'surface', 'field', 'sidebar', 'hover']),
    ...pairs(['live'], ['bg', 'surface']),
    ...pairs(['on-accent'], ['accent', 'accent-hover', 'danger']),
  ];
}

function boundaryPairs(): ColourPair[] {
  return [
    ...pairs(['border-strong'], controlBackgrounds),
    ...pairs(['icon'], ['bg', 'surface', 'field', 'sidebar', 'hover', 'mine']),
    ...pairs(['focus'], ['bg', 'surface', 'field', 'sidebar', 'hover', 'mine']),
    ...pairs(['accent'], ['bg', 'surface', 'field', 'mine']),
    ...pairs(['bg'], ['success']),
  ];
}

function failingPairs(theme: Theme, candidates: readonly ColourPair[], minimum: number): string[] {
  return candidates
    .filter(
      ({ foreground, background }) => contrast(colourOf(theme, foreground), colourOf(theme, background)) < minimum,
    )
    .map(({ foreground, background }) => `${foreground} on ${background}`);
}

const sourceRoot = resolve(import.meta.dirname, '../src');
const backgroundImageMarks = [
  'bg-linear-',
  'bg-radial-',
  'bg-conic-',
  'bg-gradient-',
  'bg-[url(',
  'bg-[image:',
  'bg-(image:',
  'gradient(',
  'url(',
];

function sourceFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) {
      return sourceFiles(path);
    }
    return /\.(ts|tsx)$/.test(entry.name) ? [path] : [];
  });
}

function backgroundImagesInSources(): string[] {
  return sourceFiles(sourceRoot).flatMap((path) => {
    const source = readFileSync(path, 'utf8');
    return backgroundImageMarks.filter((mark) => source.includes(mark)).map((mark) => `${path}: ${mark}`);
  });
}

function backgroundImagesInStyles(): string[] {
  const withoutImports = styles
    .split('\n')
    .filter((line) => !line.startsWith('@import'))
    .join('\n');
  const htmlRule = /html \{[^}]*\}/.exec(withoutImports)?.[0] ?? '';
  return [
    ...(withoutImports.includes('gradient(') ? ['gradient('] : []),
    ...(withoutImports.includes('url(') ? ['url('] : []),
    ...(withoutImports.includes('background-image') ? ['background-image'] : []),
    ...(htmlRule.includes('background-color') ? [] : ['no background-color on html']),
  ];
}

describe('the semantic colours', () => {
  test.each(['light', 'dark'] as const)(
    'every text colour keeps 4.5:1 on each surface it sits on in the %s theme',
    (theme) => {
      const failing = failingPairs(theme, textPairs(theme), minimumContrast);

      expect(failing).toEqual([]);
    },
  );

  test.each(['light', 'dark'] as const)(
    'every control boundary, icon and focus colour keeps 3:1 on each background a control sits on in the %s theme',
    (theme) => {
      const failing = failingPairs(theme, boundaryPairs(), minimumBoundaryContrast);

      expect(failing).toEqual([]);
    },
  );

  test('the page background is one flat colour', () => {
    const found = [...backgroundImagesInStyles(), ...backgroundImagesInSources()];

    expect(found).toEqual([]);
  });
});

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

test.each(['light', 'dark'] as const)(
  "the text colour on each relationship colour's 16 percent sRGB tint of surface reaches 4.5:1 in the %s theme",
  (theme) => {
    const surface = colourOf(theme, 'surface');
    const text = colourOf(theme, 'text');

    const ratios = paletteKeys.map((key) => {
      const colour = colourOf(theme, key);
      return contrast(text, {
        red: colour.red * 0.16 + surface.red * 0.84,
        green: colour.green * 0.16 + surface.green * 0.84,
        blue: colour.blue * 0.16 + surface.blue * 0.84,
      });
    });

    expect(Math.min(...ratios)).toBeGreaterThanOrEqual(minimumContrast);
  },
);
