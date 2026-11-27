import type { components } from '../../lib/api/schema.ts';

export type PaletteColour = components['schemas']['MyTutor']['colour'];

export const paletteDotClass: Readonly<Record<PaletteColour, string>> = {
  clay: 'bg-clay',
  slate: 'bg-slate',
  moss: 'bg-moss',
  plum: 'bg-plum',
  ochre: 'bg-ochre',
  teal: 'bg-teal',
  rust: 'bg-rust',
  ink: 'bg-ink',
};

export const paletteEdgeClass: Readonly<Record<PaletteColour, string>> = {
  clay: 'border-l-clay',
  slate: 'border-l-slate',
  moss: 'border-l-moss',
  plum: 'border-l-plum',
  ochre: 'border-l-ochre',
  teal: 'border-l-teal',
  rust: 'border-l-rust',
  ink: 'border-l-ink',
};

export const paletteAvatarClass: Readonly<Record<PaletteColour, string>> = {
  clay: 'bg-[color-mix(in_srgb,var(--color-clay)_16%,var(--color-surface))] text-text',
  slate: 'bg-[color-mix(in_srgb,var(--color-slate)_16%,var(--color-surface))] text-text',
  moss: 'bg-[color-mix(in_srgb,var(--color-moss)_16%,var(--color-surface))] text-text',
  plum: 'bg-[color-mix(in_srgb,var(--color-plum)_16%,var(--color-surface))] text-text',
  ochre: 'bg-[color-mix(in_srgb,var(--color-ochre)_16%,var(--color-surface))] text-text',
  teal: 'bg-[color-mix(in_srgb,var(--color-teal)_16%,var(--color-surface))] text-text',
  rust: 'bg-[color-mix(in_srgb,var(--color-rust)_16%,var(--color-surface))] text-text',
  ink: 'bg-[color-mix(in_srgb,var(--color-ink)_16%,var(--color-surface))] text-text',
};
